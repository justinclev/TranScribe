// Package aws — rds.tf template.
// Dispatches on Database.Engine to emit the correct resource type.
package aws

const rdsTmpl = `{{- if ne .Database.Engine ""}}
# ── DB Subnet Group ───────────────────────────────────────────────────────────

resource "aws_db_subnet_group" "{{tfid .Name}}" {
  name       = "{{.Name}}-db-subnet-group"
  subnet_ids = [
    aws_subnet.{{tfid .Name}}_private_1.id,
    aws_subnet.{{tfid .Name}}_private_2.id,
  ]

  tags = {
    Name = "{{.Name}}-db-subnet-group"
  }
}
{{- if isRDS .Database.Engine}}

# ── RDS Instance ({{.Database.Engine}}) ──────────────────────────────────────

resource "aws_db_instance" "{{tfid .Name}}" {
  identifier        = "{{.Name}}-db"
  engine            = "{{rdsEngine .Database.Engine}}"
  engine_version    = "{{rdsEngineVersion .Database.Engine}}"
  instance_class    = "{{if .Database.InstanceClass}}{{.Database.InstanceClass}}{{else}}db.t3.medium{{end}}"
  allocated_storage = 20
  storage_type      = "gp3"
  storage_encrypted = true
  kms_key_id        = aws_kms_key.{{tfid .Name}}.arn

  db_name  = "{{tfid .Name}}"
  username = "admin"
  password = aws_secretsmanager_secret_version.{{tfid .Name}}_db_password.secret_string

  vpc_security_group_ids = [aws_security_group.{{tfid .Name}}_db.id]
  db_subnet_group_name   = aws_db_subnet_group.{{tfid .Name}}.name
  publicly_accessible    = false

  backup_retention_period = 7
  deletion_protection     = true
  skip_final_snapshot     = false
  final_snapshot_identifier = "{{.Name}}-db-final"

  enabled_cloudwatch_logs_exports = ["{{rdsLogExport .Database.Engine}}"]

  tags = {
    Name = "{{.Name}}-db"
  }
}

resource "aws_secretsmanager_secret" "{{tfid .Name}}_db_password" {
  name                    = "{{.Name}}/db/password"
  kms_key_id              = aws_kms_key.{{tfid .Name}}.arn
  recovery_window_in_days = 7

  tags = {
    Name = "{{.Name}}-db-password"
  }
}

resource "aws_secretsmanager_secret_version" "{{tfid .Name}}_db_password" {
  secret_id     = aws_secretsmanager_secret.{{tfid .Name}}_db_password.id
  secret_string = "REPLACE_ME"

  lifecycle {
    ignore_changes = [secret_string]
  }
}
{{- end}}
{{- if isAurora .Database.Engine}}

# ── Aurora Serverless v2 ({{.Database.Engine}}) ───────────────────────────────

resource "aws_rds_cluster" "{{tfid .Name}}" {
  cluster_identifier      = "{{.Name}}-cluster"
  engine                  = "{{auroraEngine .Database.Engine}}"
  engine_mode             = "provisioned"
  engine_version          = "{{auroraEngineVersion .Database.Engine}}"
  database_name           = "{{tfid .Name}}"
  master_username         = "admin"
  master_password         = aws_secretsmanager_secret_version.{{tfid .Name}}_db_password.secret_string
  storage_encrypted       = true
  kms_key_id              = aws_kms_key.{{tfid .Name}}.arn
  vpc_security_group_ids  = [aws_security_group.{{tfid .Name}}_db.id]
  db_subnet_group_name    = aws_db_subnet_group.{{tfid .Name}}.name
  skip_final_snapshot     = false
  final_snapshot_identifier = "{{.Name}}-cluster-final"
  backup_retention_period = 7
  deletion_protection     = true

  serverlessv2_scaling_configuration {
    min_capacity = 0.5
    max_capacity = 8
  }

  tags = {
    Name = "{{.Name}}-aurora-cluster"
  }
}

resource "aws_rds_cluster_instance" "{{tfid .Name}}" {
  count              = 2
  identifier         = "{{.Name}}-instance-${count.index}"
  cluster_identifier = aws_rds_cluster.{{tfid .Name}}.id
  instance_class     = "db.serverless"
  engine             = aws_rds_cluster.{{tfid .Name}}.engine
  engine_version     = aws_rds_cluster.{{tfid .Name}}.engine_version

  tags = {
    Name = "{{.Name}}-aurora-instance-${count.index}"
  }
}

resource "aws_secretsmanager_secret" "{{tfid .Name}}_db_password" {
  name                    = "{{.Name}}/db/password"
  kms_key_id              = aws_kms_key.{{tfid .Name}}.arn
  recovery_window_in_days = 7

  tags = {
    Name = "{{.Name}}-db-password"
  }
}

resource "aws_secretsmanager_secret_version" "{{tfid .Name}}_db_password" {
  secret_id     = aws_secretsmanager_secret.{{tfid .Name}}_db_password.id
  secret_string = "REPLACE_ME"

  lifecycle {
    ignore_changes = [secret_string]
  }
}
{{- end}}
{{- if isDocDB .Database.Engine}}

# ── Amazon DocumentDB ({{.Database.Engine}}) ──────────────────────────────────

resource "aws_docdb_cluster" "{{tfid .Name}}" {
  cluster_identifier      = "{{.Name}}-docdb"
  engine                  = "docdb"
  master_username         = "admin"
  master_password         = aws_secretsmanager_secret_version.{{tfid .Name}}_db_password.secret_string
  storage_encrypted       = true
  kms_key_id              = aws_kms_key.{{tfid .Name}}.arn
  vpc_security_group_ids  = [aws_security_group.{{tfid .Name}}_db.id]
  db_subnet_group_name    = aws_db_subnet_group.{{tfid .Name}}.name
  skip_final_snapshot     = false
  final_snapshot_identifier = "{{.Name}}-docdb-final"
  backup_retention_period = 7
  deletion_protection     = true

  tags = {
    Name = "{{.Name}}-docdb"
  }
}

resource "aws_docdb_cluster_instance" "{{tfid .Name}}" {
  count              = 2
  identifier         = "{{.Name}}-docdb-${count.index}"
  cluster_identifier = aws_docdb_cluster.{{tfid .Name}}.id
  instance_class     = "db.r6g.large"
}

resource "aws_secretsmanager_secret" "{{tfid .Name}}_db_password" {
  name                    = "{{.Name}}/db/password"
  kms_key_id              = aws_kms_key.{{tfid .Name}}.arn
  recovery_window_in_days = 7
  tags = { Name = "{{.Name}}-db-password" }
}

resource "aws_secretsmanager_secret_version" "{{tfid .Name}}_db_password" {
  secret_id     = aws_secretsmanager_secret.{{tfid .Name}}_db_password.id
  secret_string = "REPLACE_ME"
  lifecycle { ignore_changes = [secret_string] }
}
{{- end}}
{{- if isRedis .Database.Engine}}

# ── ElastiCache for Redis ─────────────────────────────────────────────────────

resource "aws_elasticache_subnet_group" "{{tfid .Name}}" {
  name       = "{{.Name}}-cache-subnet-group"
  subnet_ids = [
    aws_subnet.{{tfid .Name}}_private_1.id,
    aws_subnet.{{tfid .Name}}_private_2.id,
  ]
  tags = { Name = "{{.Name}}-cache-subnet-group" }
}

resource "aws_elasticache_replication_group" "{{tfid .Name}}" {
  replication_group_id = "{{.Name}}-redis"
  description          = "{{.Name}} Redis replication group"
  engine               = "redis"
  engine_version       = "7.0"
  node_type            = "cache.t3.micro"
  num_cache_clusters   = 2
  automatic_failover_enabled = true
  at_rest_encryption_enabled = true
  kms_key_id                 = aws_kms_key.{{tfid .Name}}.arn
  transit_encryption_enabled = true
  subnet_group_name          = aws_elasticache_subnet_group.{{tfid .Name}}.name
  security_group_ids         = [aws_security_group.{{tfid .Name}}_db.id]

  tags = { Name = "{{.Name}}-redis" }
}
{{- end}}
{{- if isMemcached .Database.Engine}}

# ── ElastiCache for Memcached ─────────────────────────────────────────────────

resource "aws_elasticache_subnet_group" "{{tfid .Name}}" {
  name       = "{{.Name}}-cache-subnet-group"
  subnet_ids = [
    aws_subnet.{{tfid .Name}}_private_1.id,
    aws_subnet.{{tfid .Name}}_private_2.id,
  ]
  tags = { Name = "{{.Name}}-cache-subnet-group" }
}

resource "aws_elasticache_cluster" "{{tfid .Name}}" {
  cluster_id           = "{{.Name}}-memcached"
  engine               = "memcached"
  engine_version       = "1.6.22"
  node_type            = "cache.t3.micro"
  num_cache_nodes      = 2
  parameter_group_name = "default.memcached1.6"
  subnet_group_name    = aws_elasticache_subnet_group.{{tfid .Name}}.name
  security_group_ids   = [aws_security_group.{{tfid .Name}}_db.id]

  tags = { Name = "{{.Name}}-memcached" }
}
{{- end}}
{{- if isDynamoDB .Database.Engine}}

# ── DynamoDB Table ────────────────────────────────────────────────────────────

resource "aws_dynamodb_table" "{{tfid .Name}}_data" {
  name         = "{{.Name}}-table"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "PK"
  range_key    = "SK"

  attribute {
    name = "PK"
    type = "S"
  }

  attribute {
    name = "SK"
    type = "S"
  }

  server_side_encryption {
    enabled     = true
    kms_key_arn = aws_kms_key.{{tfid .Name}}.arn
  }

  point_in_time_recovery {
    enabled = true
  }

  tags = { Name = "{{.Name}}-table" }
}
{{- end}}
{{- if isNeptune .Database.Engine}}

# ── Amazon Neptune ────────────────────────────────────────────────────────────

resource "aws_neptune_subnet_group" "{{tfid .Name}}" {
  name       = "{{.Name}}-neptune-subnet-group"
  subnet_ids = [
    aws_subnet.{{tfid .Name}}_private_1.id,
    aws_subnet.{{tfid .Name}}_private_2.id,
  ]
  tags = { Name = "{{.Name}}-neptune-subnet-group" }
}

resource "aws_neptune_cluster" "{{tfid .Name}}" {
  cluster_identifier                  = "{{.Name}}-neptune"
  engine                              = "neptune"
  vpc_security_group_ids              = [aws_security_group.{{tfid .Name}}_db.id]
  neptune_subnet_group_name           = aws_neptune_subnet_group.{{tfid .Name}}.name
  storage_encrypted                   = true
  iam_database_authentication_enabled = true
  skip_final_snapshot                 = false
  final_snapshot_identifier           = "{{.Name}}-neptune-final"
  backup_retention_period             = 7
  deletion_protection                 = true

  tags = { Name = "{{.Name}}-neptune" }
}

resource "aws_neptune_cluster_instance" "{{tfid .Name}}" {
  count              = 2
  identifier         = "{{.Name}}-neptune-${count.index}"
  cluster_identifier = aws_neptune_cluster.{{tfid .Name}}.id
  instance_class     = "db.r6g.large"
}
{{- end}}
{{- if isCassandra .Database.Engine}}

# ── Amazon Keyspaces (Cassandra-compatible) ───────────────────────────────────

resource "aws_keyspaces_keyspace" "{{tfid .Name}}" {
  name = "{{tfid .Name}}_keyspace"
  tags = { Name = "{{.Name}}-keyspace" }
}

resource "aws_keyspaces_table" "{{tfid .Name}}" {
  keyspace_name = aws_keyspaces_keyspace.{{tfid .Name}}.name
  table_name    = "{{tfid .Name}}_table"

  schema_definition {
    column {
      name = "pk"
      type = "text"
    }
    partition_key {
      name = "pk"
    }
  }

  encryption_specification {
    type               = "CUSTOMER_MANAGED_KMS_KEY"
    kms_key_identifier = aws_kms_key.{{tfid .Name}}.arn
  }

  point_in_time_recovery {
    status = "ENABLED"
  }

  tags = { Name = "{{.Name}}-keyspaces-table" }
}
{{- end}}
{{- if isTimestream .Database.Engine}}

# ── Amazon Timestream ─────────────────────────────────────────────────────────

resource "aws_timestreamwrite_database" "{{tfid .Name}}" {
  database_name = "{{.Name}}-timestream"
  kms_key_id    = aws_kms_key.{{tfid .Name}}.arn

  tags = { Name = "{{.Name}}-timestream" }
}

resource "aws_timestreamwrite_table" "{{tfid .Name}}" {
  database_name = aws_timestreamwrite_database.{{tfid .Name}}.database_name
  table_name    = "{{.Name}}-metrics"

  retention_properties {
    magnetic_store_retention_period_in_days = 365
    memory_store_retention_period_in_hours  = 24
  }

  magnetic_store_write_properties {
    enable_magnetic_store_writes = true
  }

  tags = { Name = "{{.Name}}-timestream-table" }
}
{{- end}}
{{- end}}
`
