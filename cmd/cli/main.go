package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/justinclev/transcribe/internal/generator"
	"github.com/justinclev/transcribe/internal/hardener"
	"github.com/justinclev/transcribe/internal/parser"
)

func main() {
	filePath := flag.String("file", "", "Path to the docker-compose.yml file to transcribe (required)")
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

	// Step 2 — Apply SOC2 hardening rules.
	hardener.Harden(bp)

	// Step 3 — Generate Terraform HCL into ./out.
	const outputDir = "out"
	genErr := generator.Generate(bp, outputDir)
	if genErr != nil {
		fmt.Fprintf(os.Stderr, "error: generator: %v\n", genErr)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Terraform files written to ./%s/\n", outputDir)

	// Step 4 — Pretty-print the hardened Blueprint as JSON for verification.
	out, err := json.MarshalIndent(bp, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error marshaling blueprint: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(out))
}
