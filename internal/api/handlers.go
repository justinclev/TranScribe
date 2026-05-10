// Package api — handler implementations.
// Each handler is a thin orchestration layer: it validates HTTP concerns,
// delegates all business logic to parser/hardener/generator, then serializes
// the response. Keep business logic out of this file.
package api

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/justinclev/transcribe/internal/generator"
	"github.com/justinclev/transcribe/internal/hardener"
	"github.com/justinclev/transcribe/internal/parser"
	"github.com/justinclev/transcribe/pkg/models"
)

// maxUploadBytes caps the multipart memory buffer at 8 MiB; anything larger
// is spilled to disk by the stdlib.  A docker-compose file will never be close
// to this limit.
const maxUploadBytes = 8 << 20 // 8 MiB

// apiError is the JSON envelope returned for all 4xx/5xx responses.
type apiError struct {
	Error string `json:"error"`
}

// writeError serializes msg as a JSON apiError and sets the given status code.
// It always sets Content-Type so callers do not need to.
func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiError{Error: msg})
}

// handleTranscribe accepts a docker-compose file upload, hardens it, generates
// Terraform HCL, and streams a zip archive back to the caller.
//
//	POST /api/v1/transcribe
//	Content-Type: multipart/form-data
//	Field name:   "file"
//
// On success the response is:
//
//	Content-Type:        application/zip
//	Content-Disposition: attachment; filename="transcribe-out.zip"
//	Body:                zip containing main.tf, vpc.tf, iam.tf
//
// Error responses use JSON: {"error":"<message>"}.
func handleTranscribe(w http.ResponseWriter, r *http.Request) {
	// ── 1. Parse the multipart form ────────────────────────────────────────
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		writeError(w, http.StatusBadRequest, "could not parse multipart form: "+err.Error())
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, `multipart field "file" is required`)
		return
	}
	defer file.Close()

	// ── 2. Persist the upload to a temp file so parser.Parse can read it ──
	tmpDir, err := os.MkdirTemp("", "transcribe-upload-*")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create temp directory")
		return
	}
	defer os.RemoveAll(tmpDir) // clean up upload + generated files on return

	composePath := filepath.Join(tmpDir, "docker-compose.yml")
	dst, err := os.Create(composePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create temp file")
		return
	}

	if _, err = io.Copy(dst, file); err != nil {
		dst.Close()
		writeError(w, http.StatusInternalServerError, "could not save uploaded file")
		return
	}
	dst.Close()

	// Optional sidecar: if the caller included a "config" field, persist it too.
	configPath := ""
	if configFile, _, cfgErr := r.FormFile("config"); cfgErr == nil {
		defer configFile.Close()
		configPath = filepath.Join(tmpDir, "transcribe.yml")
		cfgDst, cfgCreateErr := os.Create(configPath)
		if cfgCreateErr != nil {
			writeError(w, http.StatusInternalServerError, "could not create config temp file")
			return
		}
		if _, cfgCopyErr := io.Copy(cfgDst, configFile); cfgCopyErr != nil {
			cfgDst.Close()
			writeError(w, http.StatusInternalServerError, "could not save config file")
			return
		}
		cfgDst.Close()
	}

	// ── 3. Parse → (optionally apply sidecar config) → Harden → Generate ──
	bp, err := parser.Parse(composePath)
	if err != nil {
		// Treat parse failures as client errors: the uploaded YAML was invalid.
		writeError(w, http.StatusBadRequest, "invalid docker-compose file: "+err.Error())
		return
	}

	if configPath != "" {
		if parseErr := parser.ParseConfig(configPath, bp); parseErr != nil {
			writeError(w, http.StatusBadRequest, "invalid transcribe.yml: "+parseErr.Error())
			return
		}
	}

	if err := applyDefaults(bp, r); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	hardener.Harden(bp)

	outDir := filepath.Join(tmpDir, "out")
	if err := generator.Generate(bp, outDir); err != nil {
		writeError(w, http.StatusInternalServerError, "terraform generation failed: "+err.Error())
		return
	}

	// ── 4. Zip all generated files (recursively) ──────────────────────────
	if err := zipGeneratedFiles(w, outDir); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

// zipGeneratedFiles zips the contents of outDir and streams it to w.
func zipGeneratedFiles(w http.ResponseWriter, outDir string) error {
	entries, err := os.ReadDir(outDir)
	if err != nil || len(entries) == 0 {
		return fmt.Errorf("no files were generated")
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="transcribe-out.zip"`)

	zw := zip.NewWriter(w)
	defer zw.Close()

	if walkErr := filepath.WalkDir(outDir, func(path string, d os.DirEntry, walkFnErr error) error {
		if walkFnErr != nil {
			return walkFnErr
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(outDir, path)
		if relErr != nil {
			return relErr
		}
		in, openErr := os.Open(path)
		if openErr != nil {
			return fmt.Errorf("could not open %s: %w", rel, openErr)
		}
		defer in.Close()

		ze, createErr := zw.Create(rel)
		if createErr != nil {
			return fmt.Errorf("could not create zip entry: %w", createErr)
		}
		_, copyErr := io.Copy(ze, in)
		return copyErr
	}); walkErr != nil {
		return fmt.Errorf("could not write zip: %w", walkErr)
	}
	return nil
}

// applyDefaults applies provider and format defaults from the request to the blueprint.
func applyDefaults(bp *models.Blueprint, r *http.Request) error {
	// provider defaults to aws when the field is absent or empty.
	switch p := models.Provider(r.FormValue("provider")); p {
	case models.ProviderAWS, models.ProviderAzure, models.ProviderGCP:
		bp.Provider = p
	case "":
		bp.Provider = models.ProviderAWS
	default:
		return fmt.Errorf("unknown provider %s: must be aws, azure, or gcp", p)
	}

	// format defaults to terraform when the field is absent or empty.
	switch f := models.OutputFormat(r.FormValue("format")); f {
	case models.FormatTerraform, models.FormatPulumi, models.FormatCDK, models.FormatHelm:
		bp.OutputFormat = f
	case "":
		bp.OutputFormat = models.FormatTerraform
	default:
		return fmt.Errorf("unknown format %s: must be terraform, pulumi, cdk, or helm", f)
	}
	return nil
}
