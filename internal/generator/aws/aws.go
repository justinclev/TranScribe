// Package aws generates SOC2-compliant AWS Terraform HCL from a Blueprint.
//
// Output files:
//
//	main.tf              — AWS provider + Terraform block
//	vpc.tf               — VPC, subnets, IGW, NAT GW, route tables
//	security_groups.tf   — ALB, ECS, and DB security groups
//	kms.tf               — Customer-managed KMS key (encryption at rest)
//	ecr.tf               — ECR repository per service
//	logs.tf              — CloudWatch log group per service + VPC flow logs log group
//	flow_logs.tf         — VPC Flow Logs to CloudWatch (SOC2 CC6/CC7)
//	iam.tf               — ECS task IAM role + inline policy per service
//	ecs.tf               — ECS cluster, task execution role, task definitions, services
//	alb.tf               — ALB, HTTPS listener, ACM cert, target groups (when PublicLoadBalancer)
//	rds.tf               — Managed database (engine-specific resource)
//	secrets.tf           — Secrets Manager secrets for service env vars
//	backend.tf           — S3 + DynamoDB Terraform remote state
package aws

import (
	"strconv"
	"strings"
	"text/template"

	"github.com/justinclev/transcribe/internal/generator/render"
	"github.com/justinclev/transcribe/pkg/models"
)

// Generate writes all AWS Terraform files into outputDir.
func Generate(bp *models.Blueprint, outputDir string) error {
	return render.WriteFiles(outputDir, []struct{ Name, Tmpl string }{
		{"main.tf", mainTmpl},
		{"vpc.tf", vpcTmpl},
		{"security_groups.tf", sgTmpl},
		{"kms.tf", kmsTmpl},
		{"ecr.tf", ecrTmpl},
		{"logs.tf", logsTmpl},
		{"flow_logs.tf", flowLogsTmpl},
		{"iam.tf", iamTmpl},
		{"ecs.tf", ecsTmpl},
		{"alb.tf", albTmpl},
		{"rds.tf", rdsTmpl},
		{"secrets.tf", secretsTmpl},
		{"backend.tf", backendTmpl},
	}, bp, funcMap())
}

// funcMap returns the AWS-specific template functions.
func funcMap() template.FuncMap {
	return template.FuncMap{
		"dbPort":              dbPort,
		"isRDS":               isRDS,
		"isAurora":            isAurora,
		"isDocDB":             isDocDB,
		"isRedis":             isRedis,
		"isMemcached":         isMemcached,
		"isDynamoDB":          isDynamoDB,
		"isNeptune":           isNeptune,
		"isCassandra":         isCassandra,
		"isTimestream":        isTimestream,
		"rdsEngine":           rdsEngine,
		"rdsEngineVersion":    rdsEngineVersion,
		"rdsLogExport":        rdsLogExport,
		"auroraEngine":        auroraEngine,
		"auroraEngineVersion": auroraEngineVersion,
		"firstPort":           firstPort,
		"add100":              add100,
		"list":                list,
	}
}

// ---------------------------------------------------------------------------
// Database engine helpers
// ---------------------------------------------------------------------------

// dbPort returns the default port for a given database engine.
func dbPort(e models.DatabaseEngine) string {
	switch e {
	case models.EnginePostgres, models.EngineAuroraPostgres:
		return "5432"
	case models.EngineMySQL, models.EngineAuroraMySQL, models.EngineMariaDB:
		return "3306"
	case models.EngineSQLServer:
		return "1433"
	case models.EngineOracle:
		return "1521"
	case models.EngineDocumentDB:
		return "27017"
	case models.EngineRedis:
		return "6379"
	case models.EngineMemcached:
		return "11211"
	case models.EngineNeptune:
		return "8182"
	default:
		return "5432"
	}
}

func isRDS(e models.DatabaseEngine) bool {
	switch e {
	case models.EnginePostgres, models.EngineMySQL, models.EngineMariaDB,
		models.EngineOracle, models.EngineSQLServer:
		return true
	}
	return false
}

func isAurora(e models.DatabaseEngine) bool {
	return e == models.EngineAuroraPostgres || e == models.EngineAuroraMySQL
}

func isDocDB(e models.DatabaseEngine) bool      { return e == models.EngineDocumentDB }
func isRedis(e models.DatabaseEngine) bool      { return e == models.EngineRedis }
func isMemcached(e models.DatabaseEngine) bool  { return e == models.EngineMemcached }
func isDynamoDB(e models.DatabaseEngine) bool   { return e == models.EngineDynamoDB }
func isNeptune(e models.DatabaseEngine) bool    { return e == models.EngineNeptune }
func isCassandra(e models.DatabaseEngine) bool  { return e == models.EngineCassandra }
func isTimestream(e models.DatabaseEngine) bool { return e == models.EngineTimestream }

func rdsEngine(e models.DatabaseEngine) string {
	switch e {
	case models.EngineMySQL:
		return "mysql"
	case models.EngineMariaDB:
		return "mariadb"
	case models.EngineOracle:
		return "oracle-se2"
	case models.EngineSQLServer:
		return "sqlserver-ex"
	default:
		return "postgres"
	}
}

func rdsEngineVersion(e models.DatabaseEngine) string {
	switch e {
	case models.EngineMySQL:
		return "8.0"
	case models.EngineMariaDB:
		return "10.11"
	case models.EngineOracle:
		return "19"
	case models.EngineSQLServer:
		return "15.00"
	default:
		return "16.1"
	}
}

func rdsLogExport(e models.DatabaseEngine) string {
	switch e {
	case models.EngineMySQL, models.EngineMariaDB, models.EngineAuroraMySQL:
		return "error"
	case models.EngineSQLServer:
		return "error"
	default:
		return "postgresql"
	}
}

func auroraEngine(e models.DatabaseEngine) string {
	if e == models.EngineAuroraMySQL {
		return "aurora-mysql"
	}
	return "aurora-postgresql"
}

func auroraEngineVersion(e models.DatabaseEngine) string {
	if e == models.EngineAuroraMySQL {
		return "8.0.mysql_aurora.3.05.0"
	}
	return "16.1"
}

// ---------------------------------------------------------------------------
// ALB / ECS helpers
// ---------------------------------------------------------------------------

// firstPort extracts the container port from the first entry of a ports slice.
// Each entry may be "hostPort:containerPort" or just "port".
func firstPort(ports []string) string {
	if len(ports) == 0 {
		return "80"
	}
	p := ports[0]
	if idx := strings.Index(p, ":"); idx >= 0 {
		return p[idx+1:]
	}
	return p
}

// add100 returns i+100 as a string, used for ALB listener rule priorities.
func add100(i int) string { return strconv.Itoa(i + 100) }

// list wraps a single string in a slice, used inside ECS template to pass
// a one-element slice to firstPort.
func list(s string) []string { return []string{s} }

// ---------------------------------------------------------------------------
// main.tf template
// ---------------------------------------------------------------------------

// mainTmpl renders main.tf: the AWS provider pinned to bp.Region.
const mainTmpl = `terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = "{{.Region}}"

  default_tags {
    tags = {
      Transcribe = "true"
    }
  }
}
`

// ---------------------------------------------------------------------------
// vpc.tf template
// ---------------------------------------------------------------------------

const vpcTmpl = `locals {
  vpc_cidr = "{{.Network.VPCCidr}}"
}

data "aws_availability_zones" "available" {
  state = "available"
}

# ── VPC ──────────────────────────────────────────────────────────────────────

resource "aws_vpc" "{{tfid .Name}}" {
  cidr_block           = local.vpc_cidr
  enable_dns_hostnames = true
  enable_dns_support   = true

  tags = {
    Name = "{{.Name}}-vpc"
  }
}

# ── Internet Gateway ──────────────────────────────────────────────────────────

resource "aws_internet_gateway" "{{tfid .Name}}" {
  vpc_id = aws_vpc.{{tfid .Name}}.id

  tags = {
    Name = "{{.Name}}-igw"
  }
}

# ── Public Subnets ────────────────────────────────────────────────────────────

resource "aws_subnet" "{{tfid .Name}}_public_1" {
  vpc_id                  = aws_vpc.{{tfid .Name}}.id
  cidr_block              = cidrsubnet(local.vpc_cidr, 8, 0)
  availability_zone       = data.aws_availability_zones.available.names[0]
  map_public_ip_on_launch = true

  tags = {
    Name = "{{.Name}}-public-1"
  }
}

resource "aws_subnet" "{{tfid .Name}}_public_2" {
  vpc_id                  = aws_vpc.{{tfid .Name}}.id
  cidr_block              = cidrsubnet(local.vpc_cidr, 8, 1)
  availability_zone       = data.aws_availability_zones.available.names[1]
  map_public_ip_on_launch = true

  tags = {
    Name = "{{.Name}}-public-2"
  }
}

# ── Private Subnets ───────────────────────────────────────────────────────────

resource "aws_subnet" "{{tfid .Name}}_private_1" {
  vpc_id            = aws_vpc.{{tfid .Name}}.id
  cidr_block        = cidrsubnet(local.vpc_cidr, 8, 10)
  availability_zone = data.aws_availability_zones.available.names[0]

  tags = {
    Name = "{{.Name}}-private-1"
  }
}

resource "aws_subnet" "{{tfid .Name}}_private_2" {
  vpc_id            = aws_vpc.{{tfid .Name}}.id
  cidr_block        = cidrsubnet(local.vpc_cidr, 8, 11)
  availability_zone = data.aws_availability_zones.available.names[1]

  tags = {
    Name = "{{.Name}}-private-2"
  }
}

# ── NAT Gateway (private-subnet egress) ──────────────────────────────────────

resource "aws_eip" "{{tfid .Name}}_nat" {
  domain = "vpc"

  tags = {
    Name = "{{.Name}}-nat-eip"
  }
}

resource "aws_nat_gateway" "{{tfid .Name}}" {
  allocation_id = aws_eip.{{tfid .Name}}_nat.id
  subnet_id     = aws_subnet.{{tfid .Name}}_public_1.id
  depends_on    = [aws_internet_gateway.{{tfid .Name}}]

  tags = {
    Name = "{{.Name}}-nat"
  }
}

# ── Route Tables ──────────────────────────────────────────────────────────────

resource "aws_route_table" "{{tfid .Name}}_public" {
  vpc_id = aws_vpc.{{tfid .Name}}.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.{{tfid .Name}}.id
  }

  tags = {
    Name = "{{.Name}}-public-rt"
  }
}

resource "aws_route_table_association" "{{tfid .Name}}_public_1" {
  subnet_id      = aws_subnet.{{tfid .Name}}_public_1.id
  route_table_id = aws_route_table.{{tfid .Name}}_public.id
}

resource "aws_route_table_association" "{{tfid .Name}}_public_2" {
  subnet_id      = aws_subnet.{{tfid .Name}}_public_2.id
  route_table_id = aws_route_table.{{tfid .Name}}_public.id
}

resource "aws_route_table" "{{tfid .Name}}_private" {
  vpc_id = aws_vpc.{{tfid .Name}}.id

  route {
    cidr_block     = "0.0.0.0/0"
    nat_gateway_id = aws_nat_gateway.{{tfid .Name}}.id
  }

  tags = {
    Name = "{{.Name}}-private-rt"
  }
}

resource "aws_route_table_association" "{{tfid .Name}}_private_1" {
  subnet_id      = aws_subnet.{{tfid .Name}}_private_1.id
  route_table_id = aws_route_table.{{tfid .Name}}_private.id
}

resource "aws_route_table_association" "{{tfid .Name}}_private_2" {
  subnet_id      = aws_subnet.{{tfid .Name}}_private_2.id
  route_table_id = aws_route_table.{{tfid .Name}}_private.id
}
`

// ---------------------------------------------------------------------------
// iam.tf template
// ---------------------------------------------------------------------------

// iamTmpl renders iam.tf: one aws_iam_role + aws_iam_role_policy per service.
const iamTmpl = `{{- range .Services}}
# ── {{.Name}} ─────────────────────────────────────────────────────────────────

resource "aws_iam_role" "{{tfid .IAMRoleName}}" {
  name = "{{.IAMRoleName}}"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect    = "Allow"
        Principal = { Service = "ecs-tasks.amazonaws.com" }
        Action    = "sts:AssumeRole"
      },
    ]
  })

  tags = {
    Name = "{{.IAMRoleName}}"
  }
}

resource "aws_iam_role_policy" "{{tfid .IAMRoleName}}" {
  name = "{{.IAMRoleName}}-policy"
  role = aws_iam_role.{{tfid .IAMRoleName}}.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "CloudWatchLogs"
        Effect = "Allow"
        Action = [
          "logs:CreateLogStream",
          "logs:PutLogEvents",
        ]
        Resource = "*"
      },
      {
        Sid    = "ECRPull"
        Effect = "Allow"
        Action = [
          "ecr:GetAuthorizationToken",
          "ecr:BatchCheckLayerAvailability",
          "ecr:GetDownloadUrlForLayer",
          "ecr:BatchGetImage",
        ]
        Resource = "*"
      },
    ]
  })
}
{{end}}`
