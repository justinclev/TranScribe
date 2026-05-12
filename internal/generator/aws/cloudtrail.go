// Package aws — cloudtrail.tf template (LOG-01).
package aws

// cloudtrailTmpl renders cloudtrail.tf.
// Creates:
//   - S3 bucket (encrypted, versioned, access-logged) for trail delivery
//   - CloudWatch Logs log group for real-time event analysis
//   - IAM role allowing CloudTrail to write to CloudWatch
//   - aws_cloudtrail with multi-region, global events, log file validation,
//     S3 + CloudWatch delivery, and management-event logging
//
// Note: data.aws_caller_identity.current is declared in main.tf.
const cloudtrailTmpl = `# ── CloudTrail (LOG-01) ───────────────────────────────────────────────────────
# Multi-region trail writing to S3 (long-term) and CloudWatch Logs (real-time).

# ── CloudTrail S3 Bucket ──────────────────────────────────────────────────────

resource "aws_s3_bucket" "{{tfid .Name}}_trail" {
  bucket        = "${local.env}-{{.Name}}-cloudtrail-${data.aws_caller_identity.current.account_id}"
  force_destroy = false

  tags = {
    Name = "${local.env}-{{.Name}}-cloudtrail"
  }
}

resource "aws_s3_bucket_versioning" "{{tfid .Name}}_trail" {
  bucket = aws_s3_bucket.{{tfid .Name}}_trail.id

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "{{tfid .Name}}_trail" {
  bucket = aws_s3_bucket.{{tfid .Name}}_trail.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm     = "aws:kms"
      kms_master_key_id = aws_kms_key.{{tfid .Name}}.arn
    }
  }
}

resource "aws_s3_bucket_public_access_block" "{{tfid .Name}}_trail" {
  bucket                  = aws_s3_bucket.{{tfid .Name}}_trail.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_lifecycle_configuration" "{{tfid .Name}}_trail" {
  bucket = aws_s3_bucket.{{tfid .Name}}_trail.id

  rule {
    id     = "transition-to-glacier"
    status = "Enabled"

    transition {
      days          = 90
      storage_class = "GLACIER"
    }

    expiration {
      days = 2555 # 7 years (SOC2 / PCI retention)
    }
  }
}

resource "aws_s3_bucket_policy" "{{tfid .Name}}_trail" {
  bucket = aws_s3_bucket.{{tfid .Name}}_trail.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "AWSCloudTrailAclCheck"
        Effect = "Allow"
        Principal = {
          Service = "cloudtrail.amazonaws.com"
        }
        Action   = "s3:GetBucketAcl"
        Resource = aws_s3_bucket.{{tfid .Name}}_trail.arn
      },
      {
        Sid    = "AWSCloudTrailWrite"
        Effect = "Allow"
        Principal = {
          Service = "cloudtrail.amazonaws.com"
        }
        Action   = "s3:PutObject"
        Resource = "${aws_s3_bucket.{{tfid .Name}}_trail.arn}/AWSLogs/${data.aws_caller_identity.current.account_id}/*"
        Condition = {
          StringEquals = {
            "s3:x-amz-acl" = "bucket-owner-full-control"
          }
        }
      },
    ]
  })
}

# ── CloudWatch Logs delivery ──────────────────────────────────────────────────

resource "aws_cloudwatch_log_group" "{{tfid .Name}}_trail" {
  name              = "/aws/cloudtrail/${local.env}-{{.Name}}"
  retention_in_days = 365
  kms_key_id        = aws_kms_key.{{tfid .Name}}.arn

  tags = {
    Name = "${local.env}-{{.Name}}-trail-logs"
  }
}

resource "aws_iam_role" "{{tfid .Name}}_trail_cw" {
  name = "${local.env}-{{.Name}}-trail-cw-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "cloudtrail.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })

  tags = {
    Name = "${local.env}-{{.Name}}-trail-cw-role"
  }
}

resource "aws_iam_role_policy" "{{tfid .Name}}_trail_cw" {
  name = "${local.env}-{{.Name}}-trail-cw-policy"
  role = aws_iam_role.{{tfid .Name}}_trail_cw.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Action = [
        "logs:CreateLogStream",
        "logs:PutLogEvents",
      ]
      Resource = "${aws_cloudwatch_log_group.{{tfid .Name}}_trail.arn}:*"
    }]
  })
}

# ── CloudTrail ────────────────────────────────────────────────────────────────

resource "aws_cloudtrail" "{{tfid .Name}}" {
  name                          = "${local.env}-{{.Name}}-trail"
  s3_bucket_name                = aws_s3_bucket.{{tfid .Name}}_trail.bucket
  include_global_service_events = true
  is_multi_region_trail         = true
  enable_log_file_validation    = true
  kms_key_id                    = aws_kms_key.{{tfid .Name}}.arn

  cloud_watch_logs_group_arn = "${aws_cloudwatch_log_group.{{tfid .Name}}_trail.arn}:*"
  cloud_watch_logs_role_arn  = aws_iam_role.{{tfid .Name}}_trail_cw.arn

  event_selector {
    read_write_type           = "All"
    include_management_events = true

    data_resource {
      type   = "AWS::S3::Object"
      values = ["arn:aws:s3:::"]
    }
  }

  tags = {
    Name = "${local.env}-{{.Name}}-trail"
  }

  depends_on = [aws_s3_bucket_policy.{{tfid .Name}}_trail]
}
`
