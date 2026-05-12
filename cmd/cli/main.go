package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/justinclev/transcribe/internal/generator"
	"github.com/justinclev/transcribe/internal/hardener"
	"github.com/justinclev/transcribe/internal/parser"
)

func main() {
	filePath := flag.String("file", "", "Path to the docker-compose.yml file to transcribe (required)")
	configPath := flag.String("config", "", "Path to transcribe.yml sidecar config (default: transcribe.yml in the same directory as -file)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: transcribe -file <path/to/docker-compose.yml>\n\n")
		fmt.Fprintf(os.Stderr, "Transcribe converts a docker-compose file into hardened, SOC2-compliant AWS Terraform code.\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *filePath == "" {
		fmt.Fprintln(os.Stderr, "error: -file flag is required")
		flag.Usage()
		os.Exit(1)
	}

	// Step 1 — Parse the docker-compose file into a Blueprint.
	bp, err := parser.Parse(*filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Step 2 — Apply the transcribe.yml sidecar config (if present).
	// Auto-detect transcribe.yml in the same directory as the compose file
	// unless the caller explicitly provided a -config path.
	cfg := *configPath
	if cfg == "" {
		cfg = filepath.Join(filepath.Dir(filepath.Clean(*filePath)), "transcribe.yml")
	}
	if err := parser.ParseConfig(cfg, bp); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Step 3 — Apply SOC2 hardening rules.
	hardener.Harden(bp)

	// Step 4 — Generate Terraform HCL into ./out.
	const outputDir = "out"
	if err := generator.Generate(bp, outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "error: generator: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Terraform files written to ./%s/\n", outputDir)

	// Step 5 — Pretty-print the hardened Blueprint as JSON for verification.
	out, err := json.MarshalIndent(bp, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error marshalling blueprint: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(out))
}
