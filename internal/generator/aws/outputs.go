// Package aws — outputs.tf template.
package aws

// outputsTmpl renders outputs.tf with useful references for other Terraform
// root modules, CI/CD pipelines, and operator runbooks.
const outputsTmpl = `# ── Outputs ───────────────────────────────────────────────────────────────────

output "vpc_id" {
  description = "ID of the application VPC"
  value       = aws_vpc.{{tfid .Name}}.id
}

output "private_subnet_ids" {
  description = "Private subnet IDs (AZ-a and AZ-b)"
  value       = [
    aws_subnet.{{tfid .Name}}_private_1.id,
    aws_subnet.{{tfid .Name}}_private_2.id,
  ]
}

output "public_subnet_ids" {
  description = "Public subnet IDs (AZ-a and AZ-b)"
  value       = [
    aws_subnet.{{tfid .Name}}_public_1.id,
    aws_subnet.{{tfid .Name}}_public_2.id,
  ]
}

output "kms_key_arn" {
  description = "ARN of the customer-managed KMS key used for encryption at rest"
  value       = aws_kms_key.{{tfid .Name}}.arn
}

output "ecs_cluster_name" {
  description = "Name of the ECS cluster"
  value       = aws_ecs_cluster.{{tfid .Name}}.name
}

output "ecs_cluster_arn" {
  description = "ARN of the ECS cluster"
  value       = aws_ecs_cluster.{{tfid .Name}}.arn
}
{{range .Services}}
output "ecr_repo_url_{{tfid .Name}}" {
  description = "ECR repository URL for the {{.Name}} service"
  value       = aws_ecr_repository.{{tfid .Name}}.repository_url
}
{{end}}
{{- if .Network.PublicLoadBalancer}}
output "alb_dns_name" {
  description = "DNS name of the Application Load Balancer"
  value       = aws_lb.{{tfid .Name}}.dns_name
}

output "alb_zone_id" {
  description = "Hosted zone ID of the ALB (for Route 53 alias records)"
  value       = aws_lb.{{tfid .Name}}.zone_id
}

output "alb_arn" {
  description = "ARN of the Application Load Balancer"
  value       = aws_lb.{{tfid .Name}}.arn
}

output "waf_web_acl_arn" {
  description = "ARN of the WAF v2 web ACL protecting the ALB"
  value       = aws_wafv2_web_acl.{{tfid .Name}}.arn
}
{{- end}}
{{- if .Database.Engine}}
output "db_secret_arn" {
  description = "ARN of the Secrets Manager secret holding the primary database password"
  value       = aws_secretsmanager_secret.{{tfid .Name}}_{{tfid .Database.ServiceName}}_password.arn
}
{{- end}}
{{- range .Databases}}
{{- if isRDS .Engine}}
output "rds_endpoint_{{tfid .ServiceName}}" {
  description = "RDS instance endpoint for {{.ServiceName}} (host:port)"
  value       = aws_db_instance.{{tfid $.Name}}_{{tfid .ServiceName}}.endpoint
}
{{- end}}
{{- if isAurora .Engine}}
output "aurora_cluster_endpoint_{{tfid .ServiceName}}" {
  description = "Aurora cluster writer endpoint for {{.ServiceName}}"
  value       = aws_rds_cluster.{{tfid $.Name}}_{{tfid .ServiceName}}.endpoint
}

output "aurora_reader_endpoint_{{tfid .ServiceName}}" {
  description = "Aurora cluster reader endpoint for {{.ServiceName}}"
  value       = aws_rds_cluster.{{tfid $.Name}}_{{tfid .ServiceName}}.reader_endpoint
}
{{- end}}
{{- if isDocDB .Engine}}
output "docdb_endpoint_{{tfid .ServiceName}}" {
  description = "DocumentDB cluster endpoint for {{.ServiceName}}"
  value       = aws_docdb_cluster.{{tfid $.Name}}_{{tfid .ServiceName}}.endpoint
}
{{- end}}
{{- if isRedis .Engine}}
output "redis_endpoint_{{tfid .ServiceName}}" {
  description = "ElastiCache Redis primary endpoint for {{.ServiceName}}"
  value       = aws_elasticache_replication_group.{{tfid $.Name}}_{{tfid .ServiceName}}.primary_endpoint_address
}
{{- end}}
{{- end}}

output "cloudtrail_s3_bucket" {
  description = "S3 bucket name receiving CloudTrail events"
  value       = aws_s3_bucket.{{tfid .Name}}_trail.bucket
}

output "terraform_workspace" {
  description = "Active Terraform workspace (environment)"
  value       = terraform.workspace
}
`
