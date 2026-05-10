// Package generator renders a hardened Blueprint into SOC2-compliant IaC output.
// Call Generate after running the hardener; the target cloud provider and output
// format are read from bp.Provider and bp.OutputFormat (both default to aws/terraform).
// The output directory is created automatically.
package generator

import (
	"fmt"

	awsgen "github.com/justinclev/transcribe/internal/generator/aws"
	azuregen "github.com/justinclev/transcribe/internal/generator/azure"
	cdkgen "github.com/justinclev/transcribe/internal/generator/cdk"
	gcpgen "github.com/justinclev/transcribe/internal/generator/gcp"
	helmgen "github.com/justinclev/transcribe/internal/generator/helm"
	pulumigen "github.com/justinclev/transcribe/internal/generator/pulumi"
	"github.com/justinclev/transcribe/pkg/models"
)

// Generate writes IaC output for bp into outputDir.
// Both bp.Provider and bp.OutputFormat default to aws/terraform when empty.
// Returns an error for unsupported provider+format combinations.
func Generate(bp *models.Blueprint, outputDir string) error {
	if bp.Provider == "" {
		bp.Provider = models.ProviderAWS
	}
	if bp.OutputFormat == "" {
		bp.OutputFormat = models.FormatTerraform
	}

	// Validate configuration before generation.
	if err := validateBlueprint(bp); err != nil {
		return err
	}

	// Helm is cloud-agnostic — it does not require a specific provider.
	if bp.OutputFormat == models.FormatHelm {
		return helmgen.Generate(bp, outputDir)
	}

	// CDK is currently AWS-only.
	if bp.OutputFormat == models.FormatCDK {
		if bp.Provider != models.ProviderAWS {
			return fmt.Errorf("cdk output is only supported with the aws provider (got %q)", bp.Provider)
		}
		return cdkgen.Generate(bp, outputDir)
	}

	// Pulumi handles all providers internally.
	if bp.OutputFormat == models.FormatPulumi {
		return pulumigen.Generate(bp, outputDir)
	}

	// Terraform is dispatched per-provider.
	tfRoutes := map[models.Provider]func(*models.Blueprint, string) error{
		models.ProviderAWS:   awsgen.Generate,
		models.ProviderAzure: azuregen.Generate,
		models.ProviderGCP:   gcpgen.Generate,
	}

	fn, ok := tfRoutes[bp.Provider]
	if !ok {
		return fmt.Errorf("unsupported combination: provider=%q format=%q", bp.Provider, bp.OutputFormat)
	}
	return fn(bp, outputDir)
}
