package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

// minimalCompose is a valid docker-compose v3 YAML with one app service
// (with ports, to trigger NET-02) and a Postgres DB (to trigger NET-01).
const minimalCompose = `version: "3.8"
services:
  api:
    image: nginx:latest
    ports:
      - "80:80"
  db:
    image: postgres:15
`

// workerOnlyCompose has no ports — exercises the no-ALB path.
const workerOnlyCompose = `version: "3.8"
services:
  worker:
    image: my-org/worker:latest
`

// emptyServicesCompose produces a blueprint with zero app services.
const emptyServicesCompose = `version: "3.8"
services:
  db:
    image: postgres:15
`

// invalidYAML is not valid YAML, triggering a 400 from the handler.
const invalidYAML = `not: [valid
  yaml: {broken`

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// multipartRequest builds a POST /api/v1/transcribe request with the given
// content in the "file" multipart field.
func multipartRequest(t *testing.T, fieldName, filename, content string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile(fieldName, filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := io.WriteString(fw, content); err != nil {
		t.Fatalf("write content: %v", err)
	}
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transcribe", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

// validRequest is shorthand for a well-formed upload of minimalCompose.
func validRequest(t *testing.T) *http.Request {
	t.Helper()
	return multipartRequest(t, "file", "docker-compose.yml", minimalCompose)
}

// assertJSONError checks the body is valid JSON containing a non-empty "error" key.
func assertJSONError(t *testing.T, body []byte) {
	t.Helper()
	var e apiError
	if err := json.Unmarshal(body, &e); err != nil {
		t.Errorf("error body is not valid JSON: %v\nbody: %s", err, body)
		return
	}
	if e.Error == "" {
		t.Errorf("JSON error body has empty 'error' field, body: %s", body)
	}
}

// openZip parses the recorder body as a zip archive.
func openZip(t *testing.T, rec *httptest.ResponseRecorder) *zip.Reader {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatalf("response body is not a valid zip archive: %v", err)
	}
	return zr
}

// zipFiles returns the set of filenames present in the zip.
func zipFiles(zr *zip.Reader) map[string]bool {
	set := make(map[string]bool)
	for _, f := range zr.File {
		set[f.Name] = true
	}
	return set
}

// readZipEntry returns the content of the named entry in the zip.
func readZipEntry(t *testing.T, zr *zip.Reader, name string) string {
	t.Helper()
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("open zip entry %s: %v", name, err)
			}
			defer rc.Close()
			b, _ := io.ReadAll(rc)
			return string(b)
		}
	}
	t.Fatalf("zip entry %q not found", name)
	return ""
}

// ---------------------------------------------------------------------------
// Happy-path response shape
// ---------------------------------------------------------------------------

func TestHandleTranscribe_HappyPath_Status200(t *testing.T) {
	rec := httptest.NewRecorder()
	handleTranscribe(rec, validRequest(t))
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleTranscribe_HappyPath_ContentTypeZip(t *testing.T) {
	rec := httptest.NewRecorder()
	handleTranscribe(rec, validRequest(t))
	ct := rec.Header().Get("Content-Type")
	if ct != "application/zip" {
		t.Errorf("Content-Type = %q, want application/zip", ct)
	}
}

func TestHandleTranscribe_HappyPath_ContentDisposition(t *testing.T) {
	rec := httptest.NewRecorder()
	handleTranscribe(rec, validRequest(t))
	cd := rec.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "transcribe-out.zip") {
		t.Errorf("Content-Disposition = %q, want filename transcribe-out.zip", cd)
	}
	if !strings.Contains(cd, "attachment") {
		t.Errorf("Content-Disposition = %q, want attachment disposition", cd)
	}
}

func TestHandleTranscribe_HappyPath_ValidZipBody(t *testing.T) {
	rec := httptest.NewRecorder()
	handleTranscribe(rec, validRequest(t))
	openZip(t, rec) // panics (via t.Fatal) if body is not a valid zip
}

// ---------------------------------------------------------------------------
// Zip contents
// ---------------------------------------------------------------------------

func TestHandleTranscribe_Zip_ContainsAllThreeFiles(t *testing.T) {
	rec := httptest.NewRecorder()
	handleTranscribe(rec, validRequest(t))
	zr := openZip(t, rec)
	files := zipFiles(zr)
	for _, name := range []string{"main.tf", "vpc.tf", "iam.tf"} {
		if !files[name] {
			t.Errorf("zip missing %s; files: %v", name, files)
		}
	}
}

func TestHandleTranscribe_Zip_FilesHaveTranscribeHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	handleTranscribe(rec, validRequest(t))
	zr := openZip(t, rec)
	for _, f := range zr.File {
		content := readZipEntry(t, zr, f.Name)
		if !strings.Contains(content, "Generated by Transcribe - SOC2 Compliant Architecture") {
			t.Errorf("zip entry %s missing Transcribe header", f.Name)
		}
	}
}

func TestHandleTranscribe_Zip_MainTF_HasProvider(t *testing.T) {
	rec := httptest.NewRecorder()
	handleTranscribe(rec, validRequest(t))
	zr := openZip(t, rec)
	content := readZipEntry(t, zr, "main.tf")
	if !strings.Contains(content, `provider "aws"`) {
		t.Errorf("main.tf missing AWS provider block:\n%s", content)
	}
}

func TestHandleTranscribe_Zip_MainTF_HasTranscribeTag(t *testing.T) {
	rec := httptest.NewRecorder()
	handleTranscribe(rec, validRequest(t))
	zr := openZip(t, rec)
	content := readZipEntry(t, zr, "main.tf")
	if !strings.Contains(content, `Transcribe = "true"`) {
		t.Errorf("main.tf missing Transcribe default tag:\n%s", content)
	}
}

func TestHandleTranscribe_Zip_MainTF_HasDefaultRegion(t *testing.T) {
	rec := httptest.NewRecorder()
	handleTranscribe(rec, validRequest(t))
	zr := openZip(t, rec)
	content := readZipEntry(t, zr, "main.tf")
	// Parser defaults to us-east-1
	if !strings.Contains(content, "us-east-1") {
		t.Errorf("main.tf missing default region:\n%s", content)
	}
}

func TestHandleTranscribe_Zip_VPCTF_HasSubnets(t *testing.T) {
	rec := httptest.NewRecorder()
	handleTranscribe(rec, validRequest(t))
	zr := openZip(t, rec)
	content := readZipEntry(t, zr, "vpc.tf")
	for _, want := range []string{"public_1", "public_2", "private_1", "private_2"} {
		if !strings.Contains(content, want) {
			t.Errorf("vpc.tf missing subnet %q", want)
		}
	}
}

func TestHandleTranscribe_Zip_IAMTF_HasTaskRole(t *testing.T) {
	rec := httptest.NewRecorder()
	handleTranscribe(rec, validRequest(t))
	zr := openZip(t, rec)
	content := readZipEntry(t, zr, "iam.tf")
	// The api service should have a task role generated by the hardener
	if !strings.Contains(content, "task_role") {
		t.Errorf("iam.tf missing task role:\n%s", content)
	}
}

func TestHandleTranscribe_Zip_IAMTF_HasECSTrust(t *testing.T) {
	rec := httptest.NewRecorder()
	handleTranscribe(rec, validRequest(t))
	zr := openZip(t, rec)
	content := readZipEntry(t, zr, "iam.tf")
	if !strings.Contains(content, "ecs-tasks.amazonaws.com") {
		t.Errorf("iam.tf missing ECS trust relationship:\n%s", content)
	}
}

// ---------------------------------------------------------------------------
// Happy-path variants
// ---------------------------------------------------------------------------

func TestHandleTranscribe_WorkerOnlyCompose_NoALB_StillSucceeds(t *testing.T) {
	req := multipartRequest(t, "file", "docker-compose.yml", workerOnlyCompose)
	rec := httptest.NewRecorder()
	handleTranscribe(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	openZip(t, rec)
}

func TestHandleTranscribe_DBOnlyCompose_StillSucceeds(t *testing.T) {
	req := multipartRequest(t, "file", "docker-compose.yml", emptyServicesCompose)
	rec := httptest.NewRecorder()
	handleTranscribe(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Error paths — 400 Bad Request
// ---------------------------------------------------------------------------

func TestHandleTranscribe_NotMultipart_Returns400(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transcribe", strings.NewReader("hello"))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	handleTranscribe(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	assertJSONError(t, rec.Body.Bytes())
}

func TestHandleTranscribe_NoBody_Returns400(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transcribe", nil)
	rec := httptest.NewRecorder()
	handleTranscribe(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	assertJSONError(t, rec.Body.Bytes())
}

func TestHandleTranscribe_MissingFileField_Returns400(t *testing.T) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("other_field", "some value")
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transcribe", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	handleTranscribe(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	assertJSONError(t, rec.Body.Bytes())
}

func TestHandleTranscribe_InvalidYAML_Returns400(t *testing.T) {
	req := multipartRequest(t, "file", "docker-compose.yml", invalidYAML)
	rec := httptest.NewRecorder()
	handleTranscribe(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestHandleTranscribe_InvalidYAML_JSONErrorBody(t *testing.T) {
	req := multipartRequest(t, "file", "docker-compose.yml", invalidYAML)
	rec := httptest.NewRecorder()
	handleTranscribe(rec, req)
	assertJSONError(t, rec.Body.Bytes())
}

func TestHandleTranscribe_InvalidYAML_ErrorMessageDescriptive(t *testing.T) {
	req := multipartRequest(t, "file", "docker-compose.yml", invalidYAML)
	rec := httptest.NewRecorder()
	handleTranscribe(rec, req)
	var e apiError
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if !strings.Contains(e.Error, "invalid docker-compose file") {
		t.Errorf("error message = %q, want to contain 'invalid docker-compose file'", e.Error)
	}
}

func TestHandleTranscribe_ErrorResponse_ContentTypeIsJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transcribe", nil)
	rec := httptest.NewRecorder()
	handleTranscribe(rec, req)
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("error Content-Type = %q, want application/json", ct)
	}
}

func TestHandleTranscribe_WrongFieldName_Returns400(t *testing.T) {
	// Upload the file under "compose" instead of "file"
	req := multipartRequest(t, "compose", "docker-compose.yml", minimalCompose)
	rec := httptest.NewRecorder()
	handleTranscribe(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for wrong field name", rec.Code)
	}
}

// multipartRequestWithFields builds a multipart POST with the given file
// content in the "file" field plus any extra string form fields.
func multipartRequestWithFields(t *testing.T, content string, extra map[string]string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "docker-compose.yml")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := io.WriteString(fw, content); err != nil {
		t.Fatalf("write content: %v", err)
	}
	for k, v := range extra {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatalf("write field %s: %v", k, err)
		}
	}
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transcribe", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

// ---------------------------------------------------------------------------
// Provider field
// ---------------------------------------------------------------------------

func TestHandleTranscribe_Provider_AWS_Explicit_Returns200(t *testing.T) {
	req := multipartRequestWithFields(t, minimalCompose, map[string]string{"provider": "aws"})
	rec := httptest.NewRecorder()
	handleTranscribe(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleTranscribe_Provider_Azure_Returns200(t *testing.T) {
	req := multipartRequestWithFields(t, minimalCompose, map[string]string{"provider": "azure"})
	rec := httptest.NewRecorder()
	handleTranscribe(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleTranscribe_Provider_GCP_Returns200(t *testing.T) {
	req := multipartRequestWithFields(t, minimalCompose, map[string]string{"provider": "gcp"})
	rec := httptest.NewRecorder()
	handleTranscribe(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleTranscribe_Provider_Unknown_Returns400(t *testing.T) {
	req := multipartRequestWithFields(t, minimalCompose, map[string]string{"provider": "digitalocean"})
	rec := httptest.NewRecorder()
	handleTranscribe(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for unknown provider", rec.Code)
	}
	assertJSONError(t, rec.Body.Bytes())
}

func TestHandleTranscribe_Provider_Unknown_ErrorMessage(t *testing.T) {
	req := multipartRequestWithFields(t, minimalCompose, map[string]string{"provider": "digitalocean"})
	rec := httptest.NewRecorder()
	handleTranscribe(rec, req)
	var e apiError
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	if !strings.Contains(e.Error, "unknown provider") {
		t.Errorf("error = %q, want to contain 'unknown provider'", e.Error)
	}
}

// ---------------------------------------------------------------------------
// Format field
// ---------------------------------------------------------------------------

func TestHandleTranscribe_Format_Terraform_Explicit_Returns200(t *testing.T) {
	req := multipartRequestWithFields(t, minimalCompose, map[string]string{"format": "terraform"})
	rec := httptest.NewRecorder()
	handleTranscribe(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleTranscribe_Format_Pulumi_Returns200(t *testing.T) {
	req := multipartRequestWithFields(t, minimalCompose, map[string]string{"format": "pulumi"})
	rec := httptest.NewRecorder()
	handleTranscribe(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleTranscribe_Format_Pulumi_ZipContainsIndexTS(t *testing.T) {
	req := multipartRequestWithFields(t, minimalCompose, map[string]string{"format": "pulumi"})
	rec := httptest.NewRecorder()
	handleTranscribe(rec, req)
	zr := openZip(t, rec)
	files := zipFiles(zr)
	if !files["index.ts"] {
		t.Errorf("pulumi zip missing index.ts; files: %v", files)
	}
}

func TestHandleTranscribe_Format_CDK_Returns200(t *testing.T) {
	req := multipartRequestWithFields(t, minimalCompose, map[string]string{"format": "cdk"})
	rec := httptest.NewRecorder()
	handleTranscribe(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleTranscribe_Format_Helm_Returns200(t *testing.T) {
	req := multipartRequestWithFields(t, minimalCompose, map[string]string{"format": "helm"})
	rec := httptest.NewRecorder()
	handleTranscribe(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleTranscribe_Format_Helm_ZipContainsChartYAML(t *testing.T) {
	req := multipartRequestWithFields(t, minimalCompose, map[string]string{"format": "helm"})
	rec := httptest.NewRecorder()
	handleTranscribe(rec, req)
	zr := openZip(t, rec)
	files := zipFiles(zr)
	if !files["Chart.yaml"] {
		t.Errorf("helm zip missing Chart.yaml; files: %v", files)
	}
}

func TestHandleTranscribe_Format_Unknown_Returns400(t *testing.T) {
	req := multipartRequestWithFields(t, minimalCompose, map[string]string{"format": "cloudformation"})
	rec := httptest.NewRecorder()
	handleTranscribe(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for unknown format", rec.Code)
	}
	assertJSONError(t, rec.Body.Bytes())
}

func TestHandleTranscribe_Format_Unknown_ErrorMessage(t *testing.T) {
	req := multipartRequestWithFields(t, minimalCompose, map[string]string{"format": "cloudformation"})
	rec := httptest.NewRecorder()
	handleTranscribe(rec, req)
	var e apiError
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	if !strings.Contains(e.Error, "unknown format") {
		t.Errorf("error = %q, want to contain 'unknown format'", e.Error)
	}
}

func TestHandleTranscribe_CDK_NonAWS_Returns500(t *testing.T) {
	req := multipartRequestWithFields(t, minimalCompose, map[string]string{
		"format":   "cdk",
		"provider": "azure",
	})
	rec := httptest.NewRecorder()
	handleTranscribe(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 for CDK+azure", rec.Code)
	}
	assertJSONError(t, rec.Body.Bytes())
}

func TestRegisterRoutes_TranscribeEndpoint_Reachable(t *testing.T) {
	mux := http.NewServeMux()
	RegisterRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, validRequest(t))
	if rec.Code != http.StatusOK {
		t.Errorf("via mux: status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
}

func TestRegisterRoutes_RejectsNonPOST(t *testing.T) {
	mux := http.NewServeMux()
	RegisterRoutes(mux)
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		method := method
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/v1/transcribe", nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s /api/v1/transcribe: status = %d, want 405", method, rec.Code)
			}
		})
	}
}

func TestRegisterRoutes_UnknownPath_Returns404(t *testing.T) {
	mux := http.NewServeMux()
	RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/unknown", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown path: status = %d, want 404", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// writeError helper
// ---------------------------------------------------------------------------

func TestWriteError_SetsStatusCode(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusTeapot, "test error")
	if rec.Code != http.StatusTeapot {
		t.Errorf("status = %d, want 418", rec.Code)
	}
}

func TestWriteError_SetsJSONContentType(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusBadRequest, "test error")
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestWriteError_BodyIsValidJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusBadRequest, "something went wrong")
	assertJSONError(t, rec.Body.Bytes())
}

func TestWriteError_MessagePreserved(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusBadRequest, "my specific error message")
	var e apiError
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if e.Error != "my specific error message" {
		t.Errorf("error message = %q, want %q", e.Error, "my specific error message")
	}
}
