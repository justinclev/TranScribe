package azure

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justinclev/transcribe/pkg/models"
)

func minimalBP() *models.Blueprint {
	return &models.Blueprint{
		Name:     "test-app",
		Provider: models.ProviderAzure,
		Region:   "eastus",
		Network:  models.NetworkConfig{VPCCidr: "10.0.0.0/16"},
		Services: []models.Service{
			{Name: "api", CPU: 256, Memory: 512},
		},
	}
}

func TestGenerate_CreatesExpectedFiles(t *testing.T) {
	dir := t.TempDir()
	if err := Generate(minimalBP(), dir); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for _, name := range []string{"main.tf", "network.tf", "identity.tf"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("missing file: %s", name)
		}
	}
}

func TestGenerate_MainTF_HasProvider(t *testing.T) {
	dir := t.TempDir()
	if err := Generate(minimalBP(), dir); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "main.tf"))
	if !strings.Contains(string(b), "azurerm") {
		t.Error("main.tf should contain azurerm provider")
	}
	if !strings.Contains(string(b), "eastus") {
		t.Error("main.tf should contain region")
	}
}

func TestGenerate_MainTF_HasTranscribeTag(t *testing.T) {
	dir := t.TempDir()
	if err := Generate(minimalBP(), dir); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "main.tf"))
	if !strings.Contains(string(b), `Transcribe = "true"`) {
		t.Error("main.tf should have Transcribe tag")
	}
}

func TestGenerate_NetworkTF_HasVNet(t *testing.T) {
	dir := t.TempDir()
	if err := Generate(minimalBP(), dir); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "network.tf"))
	if !strings.Contains(string(b), "azurerm_virtual_network") {
		t.Error("network.tf should contain virtual network")
	}
}

func TestGenerate_IdentityTF_HasManagedIdentityPerService(t *testing.T) {
	dir := t.TempDir()
	bp := minimalBP()
	bp.Services = []models.Service{
		{Name: "api", CPU: 256, Memory: 512},
		{Name: "worker", CPU: 256, Memory: 512},
	}
	if err := Generate(bp, dir); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "identity.tf"))
	content := string(b)
	if !strings.Contains(content, "api") {
		t.Error("identity.tf missing api identity")
	}
	if !strings.Contains(content, "worker") {
		t.Error("identity.tf missing worker identity")
	}
}
