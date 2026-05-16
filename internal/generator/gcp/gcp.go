// Package gcp generates SOC2-compliant GCP Terraform HCL from a Blueprint.
//
// Output files:
//
//	main.tf    — google provider block
//	network.tf — VPC network, subnets (2 public + 2 private), Cloud NAT
//	iam.tf     — Service Account + IAM bindings per service
package gcp

import (
	"github.com/justinclev/transcribe/internal/generator/render"
	"github.com/justinclev/transcribe/internal/models"
)

// Generate writes all GCP Terraform files into outputDir.
func Generate(bp *models.Blueprint, outputDir string) error {
	return render.WriteFiles(outputDir, []struct{ Name, Tmpl string }{
		{"main.tf", mainTmpl},
		{"network.tf", networkTmpl},
		{"iam.tf", iamTmpl},
	}, bp, nil)
}

const mainTmpl = `terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

variable "project_id" {
  description = "GCP project ID to deploy resources into."
  type        = string
}

provider "google" {
  project = var.project_id
  region  = "{{.Region}}"
}
`

const networkTmpl = `# ── VPC ──────────────────────────────────────────────────────────────────────

resource "google_compute_network" "{{tfid .Name}}" {
  name                    = "{{.Name}}-vpc"
  auto_create_subnetworks = false
}

# ── Public Subnets ────────────────────────────────────────────────────────────

resource "google_compute_subnetwork" "{{tfid .Name}}_public_1" {
  name          = "{{.Name}}-public-1"
  ip_cidr_range = cidrsubnet("{{.Network.VPCCidr}}", 8, 0)
  region        = "{{.Region}}"
  network       = google_compute_network.{{tfid .Name}}.id
}

resource "google_compute_subnetwork" "{{tfid .Name}}_public_2" {
  name          = "{{.Name}}-public-2"
  ip_cidr_range = cidrsubnet("{{.Network.VPCCidr}}", 8, 1)
  region        = "{{.Region}}"
  network       = google_compute_network.{{tfid .Name}}.id
}

# ── Private Subnets ───────────────────────────────────────────────────────────

resource "google_compute_subnetwork" "{{tfid .Name}}_private_1" {
  name                     = "{{.Name}}-private-1"
  ip_cidr_range            = cidrsubnet("{{.Network.VPCCidr}}", 8, 10)
  region                   = "{{.Region}}"
  network                  = google_compute_network.{{tfid .Name}}.id
  private_ip_google_access = true
}

resource "google_compute_subnetwork" "{{tfid .Name}}_private_2" {
  name                     = "{{.Name}}-private-2"
  ip_cidr_range            = cidrsubnet("{{.Network.VPCCidr}}", 8, 11)
  region                   = "{{.Region}}"
  network                  = google_compute_network.{{tfid .Name}}.id
  private_ip_google_access = true
}

# ── Cloud NAT (private-subnet egress) ────────────────────────────────────────

resource "google_compute_router" "{{tfid .Name}}" {
  name    = "{{.Name}}-router"
  region  = "{{.Region}}"
  network = google_compute_network.{{tfid .Name}}.id
}

resource "google_compute_router_nat" "{{tfid .Name}}" {
  name                               = "{{.Name}}-nat"
  router                             = google_compute_router.{{tfid .Name}}.name
  region                             = "{{.Region}}"
  nat_ip_allocate_option             = "AUTO_ONLY"
  source_subnetwork_ip_ranges_to_nat = "LIST_OF_SUBNETWORKS"

  subnetwork {
    name                    = google_compute_subnetwork.{{tfid .Name}}_private_1.id
    source_ip_ranges_to_nat = ["ALL_IP_RANGES"]
  }

  subnetwork {
    name                    = google_compute_subnetwork.{{tfid .Name}}_private_2.id
    source_ip_ranges_to_nat = ["ALL_IP_RANGES"]
  }
}
`

const iamTmpl = `{{- range .Services}}
# ── {{.Name}} ─────────────────────────────────────────────────────────────────

resource "google_service_account" "{{tfid .IAMRoleName}}" {
  account_id   = "{{gcpSAID .IAMRoleName}}"
  display_name = "{{.IAMRoleName}}"
}

resource "google_project_iam_member" "{{tfid .IAMRoleName}}_logging" {
  project = var.project_id
  role    = "roles/logging.logWriter"
  member  = "serviceAccount:${google_service_account.{{tfid .IAMRoleName}}.email}"
}

resource "google_project_iam_member" "{{tfid .IAMRoleName}}_registry" {
  project = var.project_id
  role    = "roles/artifactregistry.reader"
  member  = "serviceAccount:${google_service_account.{{tfid .IAMRoleName}}.email}"
}
{{end}}`
