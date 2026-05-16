package cdk

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
		Network:  models.NetworkConfig{VPCCidr: "10.0.0.0/16"},
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
	for _, name := range []string{"cdk.json", "package.json", "bin/app.ts", "lib/stack.ts"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("missing file: %s", name)
		}
	}
}

func TestGenerate_CdkJson_HasTranscribeContext(t *testing.T) {
	dir := t.TempDir()
	if err := Generate(minimalBP(), dir); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "cdk.json"))
	if !strings.Contains(string(b), `"Transcribe": "true"`) {
		t.Error("cdk.json should have Transcribe context tag")
	}
}

func TestGenerate_PackageJson_HasAppName(t *testing.T) {
	dir := t.TempDir()
	if err := Generate(minimalBP(), dir); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "package.json"))
	if !strings.Contains(string(b), "test-app") {
		t.Error("package.json should contain app name")
	}
}

func TestGenerate_StackTS_HasServicePerBlueprint(t *testing.T) {
	dir := t.TempDir()
	bp := minimalBP()
	bp.Services = []models.Service{
		{Name: "api", Ports: []string{"80:8080"}, CPU: 256, Memory: 512},
		{Name: "worker", CPU: 256, Memory: 512},
	}
	if err := Generate(bp, dir); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "lib/stack.ts"))
	content := string(b)
	if !strings.Contains(content, "api") {
		t.Error("stack.ts should reference api service")
	}
	if !strings.Contains(content, "worker") {
		t.Error("stack.ts should reference worker service")
	}
}

// ---------------------------------------------------------------------------
// cdkFuncMap — firstPort
// ---------------------------------------------------------------------------

func TestCdkFuncMap_FirstPort(t *testing.T) {
	fm := cdkFuncMap()
	fn := fm["firstPort"].(func([]string) string)
	if got := fn(nil); got != "80" {
		t.Errorf("firstPort(nil) = %s, want 80", got)
	}
	if got := fn([]string{"443:8443"}); got != "8443" {
		t.Errorf("firstPort([443:8443]) = %s, want 8443", got)
	}
	if got := fn([]string{"9090"}); got != "9090" {
		t.Errorf("firstPort([9090]) = %s, want 9090", got)
	}
}
