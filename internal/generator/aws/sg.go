// Package aws — security_groups.tf template.
package aws

// sgTmpl renders security_groups.tf.
// Creates: ALB SG (80/443 inbound), ECS SG (from ALB only), DB SG (from ECS only).
const sgTmpl = `# ── ALB Security Group ───────────────────────────────────────────────────────

resource "aws_security_group" "{{tfid .Name}}_alb" {
  name        = "{{.Name}}-alb-sg"
  description = "Internet-facing ALB: allow 80 and 443 inbound"
  vpc_id      = aws_vpc.{{tfid .Name}}.id

  ingress {
    description = "HTTP"
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    description = "HTTPS"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "{{.Name}}-alb-sg"
  }
}

# ── ECS Security Group ────────────────────────────────────────────────────────

resource "aws_security_group" "{{tfid .Name}}_ecs" {
  name        = "{{.Name}}-ecs-sg"
  description = "ECS tasks: allow inbound from ALB on ephemeral ports"
  vpc_id      = aws_vpc.{{tfid .Name}}.id

  ingress {
    description     = "From ALB"
    from_port       = 1024
    to_port         = 65535
    protocol        = "tcp"
    security_groups = [aws_security_group.{{tfid .Name}}_alb.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "{{.Name}}-ecs-sg"
  }
}
{{- if ne .Database.Engine ""}}

# ── RDS / Cache Security Group ────────────────────────────────────────────────

resource "aws_security_group" "{{tfid .Name}}_db" {
  name        = "{{.Name}}-db-sg"
  description = "Database: allow inbound from ECS tasks only"
  vpc_id      = aws_vpc.{{tfid .Name}}.id

  ingress {
    description     = "From ECS"
    from_port       = {{dbPort .Database.Engine}}
    to_port         = {{dbPort .Database.Engine}}
    protocol        = "tcp"
    security_groups = [aws_security_group.{{tfid .Name}}_ecs.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "{{.Name}}-db-sg"
  }
}
{{- end}}
`
