// Package azure generates SOC2-compliant Azure Terraform HCL from a Blueprint.
//
// Output files:
//
//	main.tf     — azurerm provider + resource group
//	network.tf  — Virtual Network, subnets (2 public + 2 private), NAT Gateway
//	identity.tf — User-assigned Managed Identity per service
package azure

import (
	"github.com/justinclev/transcribe/internal/generator/render"
	"github.com/justinclev/transcribe/internal/models"
)

// Generate writes all Azure Terraform files into outputDir.
func Generate(bp *models.Blueprint, outputDir string) error {
	return render.WriteFiles(outputDir, []struct{ Name, Tmpl string }{
		{"main.tf", mainTmpl},
		{"network.tf", networkTmpl},
		{"identity.tf", identityTmpl},
	}, bp, nil)
}

const mainTmpl = `terraform {
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 3.0"
    }
  }
}

provider "azurerm" {
  features {}
}

resource "azurerm_resource_group" "{{tfid .Name}}" {
  name     = "{{.Name}}-rg"
  location = "{{.Region}}"

  tags = {
    Transcribe = "true"
  }
}
`

const networkTmpl = `locals {
  vnet_cidr = "{{.Network.VPCCidr}}"
}

# ── Virtual Network ───────────────────────────────────────────────────────────

resource "azurerm_virtual_network" "{{tfid .Name}}" {
  name                = "{{.Name}}-vnet"
  address_space       = [local.vnet_cidr]
  location            = azurerm_resource_group.{{tfid .Name}}.location
  resource_group_name = azurerm_resource_group.{{tfid .Name}}.name

  tags = {
    Name       = "{{.Name}}-vnet"
    Transcribe = "true"
  }
}

# ── Public Subnets ────────────────────────────────────────────────────────────

resource "azurerm_subnet" "{{tfid .Name}}_public_1" {
  name                 = "{{.Name}}-public-1"
  resource_group_name  = azurerm_resource_group.{{tfid .Name}}.name
  virtual_network_name = azurerm_virtual_network.{{tfid .Name}}.name
  address_prefixes     = [cidrsubnet(local.vnet_cidr, 8, 0)]
}

resource "azurerm_subnet" "{{tfid .Name}}_public_2" {
  name                 = "{{.Name}}-public-2"
  resource_group_name  = azurerm_resource_group.{{tfid .Name}}.name
  virtual_network_name = azurerm_virtual_network.{{tfid .Name}}.name
  address_prefixes     = [cidrsubnet(local.vnet_cidr, 8, 1)]
}

# ── Private Subnets ───────────────────────────────────────────────────────────

resource "azurerm_subnet" "{{tfid .Name}}_private_1" {
  name                 = "{{.Name}}-private-1"
  resource_group_name  = azurerm_resource_group.{{tfid .Name}}.name
  virtual_network_name = azurerm_virtual_network.{{tfid .Name}}.name
  address_prefixes     = [cidrsubnet(local.vnet_cidr, 8, 10)]
}

resource "azurerm_subnet" "{{tfid .Name}}_private_2" {
  name                 = "{{.Name}}-private-2"
  resource_group_name  = azurerm_resource_group.{{tfid .Name}}.name
  virtual_network_name = azurerm_virtual_network.{{tfid .Name}}.name
  address_prefixes     = [cidrsubnet(local.vnet_cidr, 8, 11)]
}

# ── NAT Gateway (private-subnet egress) ──────────────────────────────────────

resource "azurerm_public_ip" "{{tfid .Name}}_nat" {
  name                = "{{.Name}}-nat-pip"
  location            = azurerm_resource_group.{{tfid .Name}}.location
  resource_group_name = azurerm_resource_group.{{tfid .Name}}.name
  allocation_method   = "Static"
  sku                 = "Standard"

  tags = {
    Name       = "{{.Name}}-nat-pip"
    Transcribe = "true"
  }
}

resource "azurerm_nat_gateway" "{{tfid .Name}}" {
  name                = "{{.Name}}-nat"
  location            = azurerm_resource_group.{{tfid .Name}}.location
  resource_group_name = azurerm_resource_group.{{tfid .Name}}.name

  tags = {
    Name       = "{{.Name}}-nat"
    Transcribe = "true"
  }
}

resource "azurerm_nat_gateway_public_ip_association" "{{tfid .Name}}" {
  nat_gateway_id       = azurerm_nat_gateway.{{tfid .Name}}.id
  public_ip_address_id = azurerm_public_ip.{{tfid .Name}}_nat.id
}

resource "azurerm_subnet_nat_gateway_association" "{{tfid .Name}}_private_1" {
  subnet_id      = azurerm_subnet.{{tfid .Name}}_private_1.id
  nat_gateway_id = azurerm_nat_gateway.{{tfid .Name}}.id
}

resource "azurerm_subnet_nat_gateway_association" "{{tfid .Name}}_private_2" {
  subnet_id      = azurerm_subnet.{{tfid .Name}}_private_2.id
  nat_gateway_id = azurerm_nat_gateway.{{tfid .Name}}.id
}
`

const identityTmpl = `{{- range .Services}}
# ── {{.Name}} ─────────────────────────────────────────────────────────────────

resource "azurerm_user_assigned_identity" "{{tfid .IAMRoleName}}" {
  name                = "{{.IAMRoleName}}"
  location            = azurerm_resource_group.{{tfid $.Name}}.location
  resource_group_name = azurerm_resource_group.{{tfid $.Name}}.name

  tags = {
    Name       = "{{.IAMRoleName}}"
    Transcribe = "true"
  }
}
{{end}}`
