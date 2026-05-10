package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/justinclev/transcribe/internal/generator"
	"github.com/justinclev/transcribe/internal/hardener"
	"github.com/justinclev/transcribe/internal/parser"
	"github.com/justinclev/transcribe/pkg/models"
)

func main() {
	filePath := flag.String("file", "", "Path to the docker-compose.yml file to transcribe (required)")
	configPath := flag.String("config", "", "Path to an optional transcribe.yml sidecar config file")
	provider := flag.String("provider", "aws", "Target cloud provider: aws, azure, or gcp")
	format := flag.String("format", "terraform", "Output format: terraform, pulumi, cdk, or helm")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: transcribe -file <path/to/docker-compose.yml> [-config <path/to/transcribe.yml>] [-provider aws|azure|gcp] [-format terraform|pulumi|cdk|helm]\n\n")
		fmt.Fprintf(os.Stderr, "Transcribe converts a docker-compose file into hardened, SOC2-compliant infrastructure code.\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *filePath == "" {
		fmt.Fprintln(os.Stderr, "error: -file flag is required")
		flag.Usage()
		os.Exit(1)
	}

	p := models.Provider(*provider)
	switch p {
	case models.ProviderAWS, models.ProviderAzure, models.ProviderGCP:
		// valid
	default:
		fmt.Fprintf(os.Stderr, "error: unknown provider %q: must be aws, azure, or gcp\n", *provider)
		os.Exit(1)
	}

	f := models.OutputFormat(*format)
	switch f {
	case models.FormatTerraform, models.FormatPulumi, models.FormatCDK, models.FormatHelm:
		// valid
	default:
		fmt.Fprintf(os.Stderr, "error: unknown format %q: must be terraform, pulumi, cdk, or helm\n", *format)
		os.Exit(1)
	}

	// Step 1 — Parse the docker-compose file into a Blueprint.
	bp, err := parser.Parse(*filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Step 1b — Apply optional sidecar config overrides.
	if err = parser.ParseConfig(*configPath, bp); err != nil {
		fmt.Fprintf(os.Stderr, "error: transcribe.yml: %v\n", err)
		os.Exit(1)
	}

	bp.Provider = p
	bp.OutputFormat = f

	// Step 2 — Apply SOC2 hardening rules.
	hardener.Harden(bp)

	// Step 3 — Generate Terraform HCL into ./out.
	const outputDir = "out"
	genErr := generator.Generate(bp, outputDir)
	if genErr != nil {
		fmt.Fprintf(os.Stderr, "error: generator: %v\n", genErr)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Files written to ./%s/\n", outputDir)

	// Step 4 — Pretty-print the hardened Blueprint as JSON for verification.
	out, err := json.MarshalIndent(bp, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error marshaling blueprint: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(out))
}
