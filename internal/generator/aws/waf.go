// Package aws — waf.tf template (SEC-01).
package aws

// wafTmpl renders waf.tf.
// Creates (only when Network.PublicLoadBalancer is true):
//   - aws_wafv2_web_acl with AWS-managed OWASP core rule set
//   - aws_wafv2_web_acl_association binding the ACL to the ALB
//   - aws_wafv2_web_acl_logging_configuration sending logs to CloudWatch
const wafTmpl = `{{- if .Network.PublicLoadBalancer}}
# ── WAF v2 Web ACL (SEC-01) ───────────────────────────────────────────────────
# OWASP Core Rule Set + AWS-managed additional protections on the ALB.

resource "aws_wafv2_web_acl" "{{tfid .Name}}" {
  name        = "${local.env}-{{.Name}}-waf"
  description = "SOC2 SEC-01 — OWASP core protections for {{.Name}} ALB"
  scope       = "REGIONAL"

  default_action {
    allow {}
  }

  # AWS-managed OWASP Core Rule Set
  rule {
    name     = "AWSManagedRulesCommonRuleSet"
    priority = 10

    override_action {
      none {}
    }

    statement {
      managed_rule_group_statement {
        name        = "AWSManagedRulesCommonRuleSet"
        vendor_name = "AWS"
      }
    }

    visibility_config {
      cloudwatch_metrics_enabled = true
      metric_name                = "${local.env}-{{.Name}}-common"
      sampled_requests_enabled   = true
    }
  }

  # AWS-managed Known Bad Inputs rule set
  rule {
    name     = "AWSManagedRulesKnownBadInputsRuleSet"
    priority = 20

    override_action {
      none {}
    }

    statement {
      managed_rule_group_statement {
        name        = "AWSManagedRulesKnownBadInputsRuleSet"
        vendor_name = "AWS"
      }
    }

    visibility_config {
      cloudwatch_metrics_enabled = true
      metric_name                = "${local.env}-{{.Name}}-bad-inputs"
      sampled_requests_enabled   = true
    }
  }

  # AWS-managed SQL injection rule set
  rule {
    name     = "AWSManagedRulesSQLiRuleSet"
    priority = 30

    override_action {
      none {}
    }

    statement {
      managed_rule_group_statement {
        name        = "AWSManagedRulesSQLiRuleSet"
        vendor_name = "AWS"
      }
    }

    visibility_config {
      cloudwatch_metrics_enabled = true
      metric_name                = "${local.env}-{{.Name}}-sqli"
        sampled_requests_enabled   = true
    }
  }

  visibility_config {
    cloudwatch_metrics_enabled = true
    metric_name                = "${local.env}-{{.Name}}-waf"
    sampled_requests_enabled   = true
  }

  tags = {
    Name = "${local.env}-{{.Name}}-waf"
  }
}

resource "aws_wafv2_web_acl_association" "{{tfid .Name}}" {
  resource_arn = aws_lb.{{tfid .Name}}.arn
  web_acl_arn  = aws_wafv2_web_acl.{{tfid .Name}}.arn
}

resource "aws_cloudwatch_log_group" "{{tfid .Name}}_waf" {
  # WAF log group names must start with "aws-waf-logs-"
  name              = "aws-waf-logs-${local.env}-{{.Name}}"
  retention_in_days = 365
  kms_key_id        = aws_kms_key.{{tfid .Name}}.arn

  tags = {
    Name = "aws-waf-logs-${local.env}-{{.Name}}"
  }
}

resource "aws_wafv2_web_acl_logging_configuration" "{{tfid .Name}}" {
  log_destination_configs = [aws_cloudwatch_log_group.{{tfid .Name}}_waf.arn]
  resource_arn            = aws_wafv2_web_acl.{{tfid .Name}}.arn
}
{{- end}}
`
