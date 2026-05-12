// Package aws — alb.tf template.
package aws

// albTmpl renders alb.tf.
// Creates (only when Network.PublicLoadBalancer is true):
//   - aws_lb (internet-facing ALB)
//   - HTTP→HTTPS redirect listener
//   - HTTPS listener (TLS 1.3 policy, ACM cert)
//   - aws_acm_certificate (DNS validation)
//   - one aws_lb_target_group + aws_lb_listener_rule per ported service
const albTmpl = `{{- if .Network.PublicLoadBalancer}}
# ── Application Load Balancer ─────────────────────────────────────────────────

resource "aws_lb" "{{tfid .Name}}" {
  name               = "${local.env}-alb"
  internal           = false
  load_balancer_type = "application"
  security_groups    = [aws_security_group.{{tfid .Name}}_alb.id]
  subnets            = [
    aws_subnet.{{tfid .Name}}_public_1.id,
    aws_subnet.{{tfid .Name}}_public_2.id,
  ]

  enable_deletion_protection = true

  access_logs {
    bucket  = aws_s3_bucket.{{tfid .Name}}_alb_logs.bucket
    prefix  = "alb"
    enabled = true
  }

  tags = {
    Name = "{{.Name}}-alb"
  }
}

# ── ALB Access Logs S3 Bucket ─────────────────────────────────────────────────

resource "aws_s3_bucket" "{{tfid .Name}}_alb_logs" {
  bucket        = "${local.env}-{{.Name}}-alb-access-logs-${data.aws_caller_identity.current.account_id}"
  force_destroy = false

  tags = {
    Name = "${local.env}-{{.Name}}-alb-access-logs"
  }
}

resource "aws_s3_bucket_versioning" "{{tfid .Name}}_alb_logs" {
  bucket = aws_s3_bucket.{{tfid .Name}}_alb_logs.id

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "{{tfid .Name}}_alb_logs" {
  bucket = aws_s3_bucket.{{tfid .Name}}_alb_logs.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm     = "aws:kms"
      kms_master_key_id = aws_kms_key.{{tfid .Name}}.arn
    }
  }
}

resource "aws_s3_bucket_public_access_block" "{{tfid .Name}}_alb_logs" {
  bucket                  = aws_s3_bucket.{{tfid .Name}}_alb_logs.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

# ── HTTP → HTTPS redirect listener ───────────────────────────────────────────

resource "aws_lb_listener" "{{tfid .Name}}_http" {
  load_balancer_arn = aws_lb.{{tfid .Name}}.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type = "redirect"

    redirect {
      port        = "443"
      protocol    = "HTTPS"
      status_code = "HTTP_301"
    }
  }
}

# ── HTTPS listener ────────────────────────────────────────────────────────────

resource "aws_lb_listener" "{{tfid .Name}}_https" {
  load_balancer_arn = aws_lb.{{tfid .Name}}.arn
  port              = 443
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-2021-06"
  certificate_arn   = aws_acm_certificate.{{tfid .Name}}.arn

  default_action {
    type = "fixed-response"

    fixed_response {
      content_type = "text/plain"
      message_body = "Not Found"
      status_code  = "404"
    }
  }
{{- if .Network.Domain}}

  depends_on = [aws_acm_certificate_validation.{{tfid .Name}}]
{{- end}}
}

# ── ACM Certificate ───────────────────────────────────────────────────────────

resource "aws_acm_certificate" "{{tfid .Name}}" {
  domain_name       = "{{if .Network.Domain}}{{.Network.Domain}}{{else}}${var.domain}{{end}}"
  validation_method = "DNS"

  lifecycle {
    create_before_destroy = true
  }

  tags = {
    Name = "${local.env}-{{.Name}}-cert"
  }
}
{{- if .Network.Domain}}

# ── Route 53 DNS Certificate Validation ──────────────────────────────────────

data "aws_route53_zone" "{{tfid .Name}}" {
  name         = "{{.Network.Domain}}"
  private_zone = false
}

resource "aws_route53_record" "{{tfid .Name}}_cert_validation" {
  for_each = {
    for dvo in aws_acm_certificate.{{tfid .Name}}.domain_validation_options : dvo.domain_name => {
      name   = dvo.resource_record_name
      record = dvo.resource_record_value
      type   = dvo.resource_record_type
    }
  }

  allow_overwrite = true
  name            = each.value.name
  records         = [each.value.record]
  ttl             = 60
  type            = each.value.type
  zone_id         = data.aws_route53_zone.{{tfid .Name}}.zone_id
}

resource "aws_acm_certificate_validation" "{{tfid .Name}}" {
  certificate_arn         = aws_acm_certificate.{{tfid .Name}}.arn
  validation_record_fqdns = [for record in aws_route53_record.{{tfid .Name}}_cert_validation : record.fqdn]
}
{{- end}}
{{range $i, $svc := .Services}}{{if $svc.Ports}}
# ── Target Group: {{$svc.Name}} ──────────────────────────────────────────────

resource "aws_lb_target_group" "{{tfid $svc.Name}}" {
  name        = "${local.env}-{{$svc.Name}}-tg"
  port        = {{firstPort $svc.Ports}}
  protocol    = "HTTP"
  vpc_id      = aws_vpc.{{tfid $.Name}}.id
  target_type = "ip"

  health_check {
    enabled             = true
    healthy_threshold   = 2
    unhealthy_threshold = 3
    interval            = 30
    path                = "{{$svc.HealthCheckPath}}"
    protocol            = "HTTP"
    timeout             = 5
    matcher             = "200-399"
  }

  tags = {
    Name = "{{$.Name}}-{{$svc.Name}}-tg"
  }
}

resource "aws_lb_listener_rule" "{{tfid $svc.Name}}" {
  listener_arn = aws_lb_listener.{{tfid $.Name}}_https.arn
  priority     = {{if isFrontend $svc.Name}}999{{else}}{{add100 $i}}{{end}}

  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.{{tfid $svc.Name}}.arn
  }

  condition {
    path_pattern {
      values = {{if isFrontend $svc.Name}}["/*"]{{else}}["/{{$svc.Name}}/*", "/{{$svc.Name}}"]{{end}}
    }
  }
}
{{end}}{{end}}
{{- end}}
`
