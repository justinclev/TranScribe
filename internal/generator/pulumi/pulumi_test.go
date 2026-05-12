package pulumi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justinclev/transcribe/pkg/models"
)

func awsBP() *models.Blueprint {
	return &models.Blueprint{
		Name:     "test-app",
		Provider: models.ProviderAWS,
		Region:   "us-east-1",
		Network:  models.NetworkConfig{VPCCidr: "10.0.0.0/16"},
		Services: []models.Service{
			{Name: "api", Ports: []string{"80:8080"}, CPU: 256, Memory: 512},
		},
	}
}

func azureBP() *models.Blueprint {
	return &models.Blueprint{
		Name:     "test-app",
		Provider: models.ProviderAzure,
		Region:   "eastus",
		Services: []models.Service{
			{Name: "api", CPU: 256, Memory: 512},
		},
	}
}

func gcpBP() *models.Blueprint {
	return &models.Blueprint{
		Name:     "test-app",
		Provider: models.ProviderGCP,
		Region:   "us-central1",
		Services: []models.Service{
			{Name: "api", CPU: 256, Memory: 512},
		},
	}
}

func TestGenerate_AWS_CreatesExpectedFiles(t *testing.T) {
	dir := t.TempDir()
	if err := Generate(awsBP(), dir); err != nil {
		t.Fatalf("Generate (AWS): %v", err)
	}
	for _, name := range []string{"Pulumi.yaml", "index.ts", "package.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("missing file: %s", name)
		}
	}
}

func TestGenerate_Azure_CreatesExpectedFiles(t *testing.T) {
	dir := t.TempDir()
	if err := Generate(azureBP(), dir); err != nil {
		t.Fatalf("Generate (Azure): %v", err)
	}
	for _, name := range []string{"Pulumi.yaml", "index.ts", "package.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("missing file: %s", name)
		}
	}
}

func TestGenerate_GCP_CreatesExpectedFiles(t *testing.T) {
	dir := t.TempDir()
	if err := Generate(gcpBP(), dir); err != nil {
		t.Fatalf("Generate (GCP): %v", err)
	}
	for _, name := range []string{"Pulumi.yaml", "index.ts", "package.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("missing file: %s", name)
		}
	}
}

func TestGenerate_UnsupportedProvider_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	bp := awsBP()
	bp.Provider = "kubernetes" // not supported
	if err := Generate(bp, dir); err == nil {
		t.Error("expected error for unsupported provider, got nil")
	}
}

func TestGenerate_AWS_PulumiYaml_HasProjectName(t *testing.T) {
	dir := t.TempDir()
	if err := Generate(awsBP(), dir); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "Pulumi.yaml"))
	if !strings.Contains(string(b), "test-app") {
		t.Error("Pulumi.yaml should contain project name")
	}
}

func TestGenerate_AWS_IndexTS_HasServiceReference(t *testing.T) {
	dir := t.TempDir()
	bp := awsBP()
	bp.Services = []models.Service{
		{Name: "api", Ports: []string{"80:8080"}, CPU: 256, Memory: 512},
		{Name: "worker", CPU: 256, Memory: 512},
	}
	if err := Generate(bp, dir); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "index.ts"))
	content := string(b)
	if !strings.Contains(content, "api") {
		t.Error("index.ts should reference api service")
	}
	if !strings.Contains(content, "worker") {
		t.Error("index.ts should reference worker service")
	}
}
