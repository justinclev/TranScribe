// Package aws — endpoints.tf template (NET-03).
package aws

// endpointsTmpl renders endpoints.tf.
// Creates Interface VPC Endpoints for:
//   - ECR API + DKR (container image pull without NAT)
//   - Secrets Manager (secret reads without NAT)
//   - CloudWatch Logs (log shipping without NAT)
//   - SSM (parameter store + session manager, no NAT)
//
// Also creates a Gateway endpoint for S3 (free, required for ECR image layers).
// An endpoint security group permits HTTPS from within the VPC.
const endpointsTmpl = `# ── VPC Endpoints (NET-03) ────────────────────────────────────────────────────
# Interface endpoints keep traffic for ECR, Secrets Manager, and CloudWatch
# inside the AWS network — no NAT gateway traversal for these control-plane
# calls. Required for SOC2 CC6.6 (network access control).

resource "aws_security_group" "{{tfid .Name}}_endpoints" {
  name        = "${local.env}-{{.Name}}-vpce-sg"
  description = "Allow HTTPS from VPC to Interface Endpoints"
  vpc_id      = aws_vpc.{{tfid .Name}}.id

  ingress {
    description = "HTTPS from VPC"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = [aws_vpc.{{tfid .Name}}.cidr_block]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "${local.env}-{{.Name}}-vpce-sg"
  }
}

# ── S3 Gateway Endpoint (free; required for ECR image layer pulls) ─────────────

resource "aws_vpc_endpoint" "{{tfid .Name}}_s3" {
  vpc_id            = aws_vpc.{{tfid .Name}}.id
  service_name      = "com.amazonaws.{{.Region}}.s3"
  vpc_endpoint_type = "Gateway"
  route_table_ids   = [
    aws_route_table.{{tfid .Name}}_private.id,
  ]

  tags = {
    Name = "${local.env}-{{.Name}}-s3-endpoint"
  }
}

# ── ECR API endpoint ──────────────────────────────────────────────────────────

resource "aws_vpc_endpoint" "{{tfid .Name}}_ecr_api" {
  vpc_id              = aws_vpc.{{tfid .Name}}.id
  service_name        = "com.amazonaws.{{.Region}}.ecr.api"
  vpc_endpoint_type   = "Interface"
  subnet_ids          = [
    aws_subnet.{{tfid .Name}}_private_1.id,
    aws_subnet.{{tfid .Name}}_private_2.id,
  ]
  security_group_ids  = [aws_security_group.{{tfid .Name}}_endpoints.id]
  private_dns_enabled = true

  tags = {
    Name = "${local.env}-{{.Name}}-ecr-api-endpoint"
  }
}

# ── ECR DKR endpoint (image layer downloads) ──────────────────────────────────

resource "aws_vpc_endpoint" "{{tfid .Name}}_ecr_dkr" {
  vpc_id              = aws_vpc.{{tfid .Name}}.id
  service_name        = "com.amazonaws.{{.Region}}.ecr.dkr"
  vpc_endpoint_type   = "Interface"
  subnet_ids          = [
    aws_subnet.{{tfid .Name}}_private_1.id,
    aws_subnet.{{tfid .Name}}_private_2.id,
  ]
  security_group_ids  = [aws_security_group.{{tfid .Name}}_endpoints.id]
  private_dns_enabled = true

  tags = {
    Name = "${local.env}-{{.Name}}-ecr-dkr-endpoint"
  }
}

# ── Secrets Manager endpoint ──────────────────────────────────────────────────

resource "aws_vpc_endpoint" "{{tfid .Name}}_secretsmanager" {
  vpc_id              = aws_vpc.{{tfid .Name}}.id
  service_name        = "com.amazonaws.{{.Region}}.secretsmanager"
  vpc_endpoint_type   = "Interface"
  subnet_ids          = [
    aws_subnet.{{tfid .Name}}_private_1.id,
    aws_subnet.{{tfid .Name}}_private_2.id,
  ]
  security_group_ids  = [aws_security_group.{{tfid .Name}}_endpoints.id]
  private_dns_enabled = true

  tags = {
    Name = "${local.env}-{{.Name}}-secretsmanager-endpoint"
  }
}

# ── CloudWatch Logs endpoint ──────────────────────────────────────────────────

resource "aws_vpc_endpoint" "{{tfid .Name}}_logs" {
  vpc_id              = aws_vpc.{{tfid .Name}}.id
  service_name        = "com.amazonaws.{{.Region}}.logs"
  vpc_endpoint_type   = "Interface"
  subnet_ids          = [
    aws_subnet.{{tfid .Name}}_private_1.id,
    aws_subnet.{{tfid .Name}}_private_2.id,
  ]
  security_group_ids  = [aws_security_group.{{tfid .Name}}_endpoints.id]
  private_dns_enabled = true

  tags = {
    Name = "${local.env}-{{.Name}}-logs-endpoint"
  }
}

# ── SSM endpoint (Systems Manager / Session Manager) ─────────────────────────

resource "aws_vpc_endpoint" "{{tfid .Name}}_ssm" {
  vpc_id              = aws_vpc.{{tfid .Name}}.id
  service_name        = "com.amazonaws.{{.Region}}.ssm"
  vpc_endpoint_type   = "Interface"
  subnet_ids          = [
    aws_subnet.{{tfid .Name}}_private_1.id,
    aws_subnet.{{tfid .Name}}_private_2.id,
  ]
  security_group_ids  = [aws_security_group.{{tfid .Name}}_endpoints.id]
  private_dns_enabled = true

  tags = {
    Name = "${local.env}-{{.Name}}-ssm-endpoint"
  }
}
`
