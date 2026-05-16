package helm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justinclev/transcribe/internal/models"
)

func minimalBP() *models.Blueprint {
	return &models.Blueprint{
		Name:     "test-app",
		Provider: models.ProviderAWS,
		Region:   "us-east-1",
		Services: []models.Service{
			{Name: "api", Ports: []string{"80:8080"}, CPU: 256, Memory: 512},
		},
	}
}

func TestGenerate_CreatesExpectedFiles(t *testing.T) {
	dir := t.TempDir()
	if err := Generate(minimalBP(), dir); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for _, name := range []string{
		"Chart.yaml",
		"values.yaml",
		"templates/deployment.yaml",
		"templates/service.yaml",
	} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("missing file: %s", name)
		}
	}
}

func TestGenerate_ChartYaml_HasAppName(t *testing.T) {
	dir := t.TempDir()
	if err := Generate(minimalBP(), dir); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "Chart.yaml"))
	if !strings.Contains(string(b), "test-app") {
		t.Error("Chart.yaml should contain app name")
	}
}

func TestGenerate_DeploymentYaml_HasServiceContainer(t *testing.T) {
	dir := t.TempDir()
	bp := minimalBP()
	bp.Services = []models.Service{
		{Name: "api", Ports: []string{"80:8080"}, CPU: 256, Memory: 512},
		{Name: "worker", CPU: 256, Memory: 512},
	}
	if err := Generate(bp, dir); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "templates/deployment.yaml"))
	content := string(b)
	// Each service produces a Deployment document (separated by ---)
	count := strings.Count(content, "kind: Deployment")
	if count != 2 {
		t.Errorf("expected 2 Deployment documents for 2 services, got %d:\n%s", count, content)
	}
}

func TestGenerate_ValuesYaml_HasReplicaCount(t *testing.T) {
	dir := t.TempDir()
	if err := Generate(minimalBP(), dir); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "values.yaml"))
	if !strings.Contains(string(b), "replicaCount") {
		t.Error("values.yaml should contain replicaCount")
	}
}
