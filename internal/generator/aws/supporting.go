// Package aws — ECR, CloudWatch, KMS, VPC Flow Logs, Secrets Manager, backend templates.
package aws

const ecrTmpl = `{{- range .Services}}
# ── ECR Repository: {{.Name}} ─────────────────────────────────────────────────

resource "aws_ecr_repository" "{{tfid .Name}}" {
  name                 = "${local.env}-{{$.Name}}-{{.Name}}"
  image_tag_mutability = "IMMUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }

  encryption_configuration {
    encryption_type = "KMS"
    kms_key         = aws_kms_key.{{tfid $.Name}}.arn
  }

  tags = {
    Name = "{{$.Name}}-{{.Name}}"
  }
}

resource "aws_ecr_lifecycle_policy" "{{tfid .Name}}" {
  repository = aws_ecr_repository.{{tfid .Name}}.name

  policy = jsonencode({
    rules = [{
      rulePriority = 1
      description  = "Keep last 10 tagged images"
      selection = {
        tagStatus   = "tagged"
        tagPrefixList = ["v"]
        countType   = "imageCountMoreThan"
        countNumber = 10
      }
      action = { type = "expire" }
    }]
  })
}
{{end}}`

const logsTmpl = `{{- range .Services}}
# ── CloudWatch Log Group: {{.Name}} ───────────────────────────────────────────

resource "aws_cloudwatch_log_group" "{{tfid .Name}}" {
  name              = "/ecs/${local.env}-{{$.Name}}/{{.Name}}"
  retention_in_days = 90
  kms_key_id        = aws_kms_key.{{tfid $.Name}}.arn

  tags = {
    Name = "${local.env}-{{$.Name}}-{{.Name}}-logs"
  }
}
{{end}}
# ── VPC Flow Logs Log Group ───────────────────────────────────────────────────

resource "aws_cloudwatch_log_group" "{{tfid .Name}}_flow_logs" {
  name              = "/vpc/${local.env}-{{.Name}}/flow-logs"
  retention_in_days = 90
  kms_key_id        = aws_kms_key.{{tfid .Name}}.arn

  tags = {
    Name = "${local.env}-{{.Name}}-vpc-flow-logs"
  }
}
`

const kmsTmpl = `# ── Customer-Managed KMS Key ─────────────────────────────────────────────────
# Used for: ECS secrets, ECR images, CloudWatch logs, RDS, S3 ALB access logs.

resource "aws_kms_key" "{{tfid .Name}}" {
  description             = "${local.env}-{{.Name}} — SOC2 encryption at rest"
  deletion_window_in_days = 30
  enable_key_rotation     = true
  multi_region            = false

  tags = {
    Name = "${local.env}-{{.Name}}-kms"
  }
}

resource "aws_kms_alias" "{{tfid .Name}}" {
  name          = "alias/${local.env}-{{.Name}}"
  target_key_id = aws_kms_key.{{tfid .Name}}.key_id
}
`

const flowLogsTmpl = `# ── VPC Flow Logs (SOC2 CC6.6 / CC7.2) ──────────────────────────────────────

resource "aws_iam_role" "{{tfid .Name}}_flow_logs" {
  name = "${local.env}-{{.Name}}-vpc-flow-logs-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "vpc-flow-logs.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })

  tags = {
    Name = "${local.env}-{{.Name}}-vpc-flow-logs-role"
  }
}

resource "aws_iam_role_policy" "{{tfid .Name}}_flow_logs" {
  name = "${local.env}-{{.Name}}-vpc-flow-logs-policy"
  role = aws_iam_role.{{tfid .Name}}_flow_logs.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Action = [
        "logs:CreateLogGroup",
        "logs:CreateLogStream",
        "logs:PutLogEvents",
        "logs:DescribeLogGroups",
        "logs:DescribeLogStreams",
      ]
      Resource = "*"
    }]
  })
}

resource "aws_flow_log" "{{tfid .Name}}" {
  vpc_id          = aws_vpc.{{tfid .Name}}.id
  traffic_type    = "ALL"
  iam_role_arn    = aws_iam_role.{{tfid .Name}}_flow_logs.arn
  log_destination = aws_cloudwatch_log_group.{{tfid .Name}}_flow_logs.arn

  tags = {
    Name = "{{.Name}}-vpc-flow-logs"
  }
}
`

//nolint:gosec // this is a terraform template placeholder, not an actual credential
const secretsTmpl = `{{- range .Services}}{{if .EnvVars}}
# ── Secrets Manager: {{.Name}} ────────────────────────────────────────────────
{{$svcName := .Name}}{{$overrides := .SecretARNOverrides}}{{range $k, $v := .EnvVars}}{{if isSensitive $k}}{{if not (isOverridden $k $overrides)}}
resource "aws_secretsmanager_secret" "{{tfid $.Name}}_{{tfid $k}}" {
  name                    = "${local.env}-{{$.Name}}/{{$svcName}}/{{$k}}"
  kms_key_id              = aws_kms_key.{{tfid $.Name}}.arn
  recovery_window_in_days = 7

  tags = {
    Name    = "${local.env}-{{$.Name}}-{{$k}}"
    Service = "{{$svcName}}"
  }
}

resource "aws_secretsmanager_secret_version" "{{tfid $.Name}}_{{tfid $k}}" {
  secret_id     = aws_secretsmanager_secret.{{tfid $.Name}}_{{tfid $k}}.id
  secret_string = random_password.{{tfid $.Name}}_{{tfid $k}}.result

  lifecycle {
    ignore_changes = [secret_string]
  }
}

resource "random_password" "{{tfid $.Name}}_{{tfid $k}}" {
  length           = 32
  special          = true
  override_special = "!#$%&*()-_=+[]{}<>:?"
}
{{end}}{{end}}{{end}}{{end}}{{end}}`

const backendTmpl = `# ── Terraform Remote State ────────────────────────────────────────────────────
# Uncomment and fill in bucket/table names after running:
#   terraform apply -target=aws_s3_bucket.{{tfid .Name}}_tfstate
#   terraform apply -target=aws_dynamodb_table.{{tfid .Name}}_tflock

# terraform {
#   backend "s3" {
#     bucket         = "${local.env}-{{.Name}}-terraform-state-${data.aws_caller_identity.current.account_id}"
#     key            = "terraform.tfstate"
#     region         = "{{.Region}}"
#     encrypt        = true
#     kms_key_id     = "alias/${local.env}-{{.Name}}"
#     dynamodb_table = "${local.env}-{{.Name}}-terraform-lock"
#   }
# }

resource "aws_s3_bucket" "{{tfid .Name}}_tfstate" {
  bucket        = "${local.env}-{{.Name}}-terraform-state-${data.aws_caller_identity.current.account_id}"
  force_destroy = false

  tags = {
    Name = "${local.env}-{{.Name}}-terraform-state"
  }
}

resource "aws_s3_bucket_versioning" "{{tfid .Name}}_tfstate" {
  bucket = aws_s3_bucket.{{tfid .Name}}_tfstate.id

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "{{tfid .Name}}_tfstate" {
  bucket = aws_s3_bucket.{{tfid .Name}}_tfstate.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm     = "aws:kms"
      kms_master_key_id = aws_kms_key.{{tfid .Name}}.arn
    }
  }
}

resource "aws_s3_bucket_public_access_block" "{{tfid .Name}}_tfstate" {
  bucket                  = aws_s3_bucket.{{tfid .Name}}_tfstate.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_dynamodb_table" "{{tfid .Name}}_tflock" {
  name         = "{{.Name}}-terraform-lock"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "LockID"

  attribute {
    name = "LockID"
    type = "S"
  }

  server_side_encryption {
    enabled     = true
    kms_key_arn = aws_kms_key.{{tfid .Name}}.arn
  }

  tags = {
    Name = "{{.Name}}-terraform-lock"
  }
}
`
