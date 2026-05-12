// Package aws — rds.tf template.
// Ranges over Blueprint.Databases to emit the correct resource type for each
// detected database engine, supporting mixed workloads (e.g. Postgres + Redis).
package aws

const rdsTmpl = `{{- range .Databases}}
{{- if isRDS .Engine}}

# ── RDS Instance ({{.Engine}} / {{.ServiceName}}) ────────────────────────────

resource "aws_db_subnet_group" "{{tfid $.Name}}_{{tfid .ServiceName}}" {
  name       = "{{$.Name}}-{{.ServiceName}}-subnet-group"
  subnet_ids = [
    aws_subnet.{{tfid $.Name}}_private_1.id,
    aws_subnet.{{tfid $.Name}}_private_2.id,
  ]

  tags = {
    Name = "{{$.Name}}-{{.ServiceName}}-subnet-group"
  }
}

resource "aws_db_instance" "{{tfid $.Name}}_{{tfid .ServiceName}}" {
  identifier        = "{{$.Name}}-{{.ServiceName}}"
  engine            = "{{rdsEngine .Engine}}"
  engine_version    = "{{rdsEngineVersion .Engine}}"
  instance_class    = "{{if .InstanceClass}}{{.InstanceClass}}{{else}}db.t3.medium{{end}}"
  allocated_storage = 20
  storage_type      = "gp3"
  storage_encrypted = true
  kms_key_id        = aws_kms_key.{{tfid $.Name}}.arn

  db_name  = "{{tfid $.Name}}"
  username = "admin"
  password = aws_secretsmanager_secret_version.{{tfid $.Name}}_{{tfid .ServiceName}}_password.secret_string

  vpc_security_group_ids = [aws_security_group.{{tfid $.Name}}_db.id]
  db_subnet_group_name   = aws_db_subnet_group.{{tfid $.Name}}_{{tfid .ServiceName}}.name
  publicly_accessible    = false

  backup_retention_period = 7
  deletion_protection     = true
  skip_final_snapshot     = false
  final_snapshot_identifier = "{{$.Name}}-{{.ServiceName}}-final"

  enabled_cloudwatch_logs_exports = ["{{rdsLogExport .Engine}}"]

  tags = {
    Name = "{{$.Name}}-{{.ServiceName}}"
  }
}

resource "aws_secretsmanager_secret" "{{tfid $.Name}}_{{tfid .ServiceName}}_password" {
  name                    = "{{$.Name}}/{{.ServiceName}}/password"
  kms_key_id              = aws_kms_key.{{tfid $.Name}}.arn
  recovery_window_in_days = 7

  tags = {
    Name    = "{{$.Name}}-{{.ServiceName}}-password"
    Service = "{{.ServiceName}}"
  }
}

resource "aws_secretsmanager_secret_version" "{{tfid $.Name}}_{{tfid .ServiceName}}_password" {
  secret_id     = aws_secretsmanager_secret.{{tfid $.Name}}_{{tfid .ServiceName}}_password.id
  secret_string = random_password.{{tfid $.Name}}_{{tfid .ServiceName}}.result

  lifecycle {
    ignore_changes = [secret_string]
  }
}

resource "random_password" "{{tfid $.Name}}_{{tfid .ServiceName}}" {
  length           = 32
  special          = true
  override_special = "!#$%&*()-_=+[]{}<>:?"
}
{{- end}}
{{- if isAurora .Engine}}

# ── Aurora Serverless v2 ({{.Engine}} / {{.ServiceName}}) ─────────────────────

resource "aws_db_subnet_group" "{{tfid $.Name}}_{{tfid .ServiceName}}" {
  name       = "{{$.Name}}-{{.ServiceName}}-subnet-group"
  subnet_ids = [
    aws_subnet.{{tfid $.Name}}_private_1.id,
    aws_subnet.{{tfid $.Name}}_private_2.id,
  ]

  tags = {
    Name = "{{$.Name}}-{{.ServiceName}}-subnet-group"
  }
}

resource "aws_rds_cluster" "{{tfid $.Name}}_{{tfid .ServiceName}}" {
  cluster_identifier      = "{{$.Name}}-{{.ServiceName}}-cluster"
  engine                  = "{{auroraEngine .Engine}}"
  engine_mode             = "provisioned"
  engine_version          = "{{auroraEngineVersion .Engine}}"
  database_name           = "{{tfid $.Name}}"
  master_username         = "admin"
  master_password         = aws_secretsmanager_secret_version.{{tfid $.Name}}_{{tfid .ServiceName}}_password.secret_string
  storage_encrypted       = true
  kms_key_id              = aws_kms_key.{{tfid $.Name}}.arn
  vpc_security_group_ids  = [aws_security_group.{{tfid $.Name}}_db.id]
  db_subnet_group_name    = aws_db_subnet_group.{{tfid $.Name}}_{{tfid .ServiceName}}.name
  skip_final_snapshot     = false
  final_snapshot_identifier = "{{$.Name}}-{{.ServiceName}}-cluster-final"
  backup_retention_period = 7
  deletion_protection     = true

  serverlessv2_scaling_configuration {
    min_capacity = 0.5
    max_capacity = 8
  }

  tags = {
    Name = "{{$.Name}}-{{.ServiceName}}-cluster"
  }
}

resource "aws_rds_cluster_instance" "{{tfid $.Name}}_{{tfid .ServiceName}}" {
  count              = 2
  identifier         = "{{$.Name}}-{{.ServiceName}}-instance-${count.index}"
  cluster_identifier = aws_rds_cluster.{{tfid $.Name}}_{{tfid .ServiceName}}.id
  instance_class     = "db.serverless"
  engine             = aws_rds_cluster.{{tfid $.Name}}_{{tfid .ServiceName}}.engine
  engine_version     = aws_rds_cluster.{{tfid $.Name}}_{{tfid .ServiceName}}.engine_version

  tags = {
    Name = "{{$.Name}}-{{.ServiceName}}-aurora-instance-${count.index}"
  }
}

resource "aws_secretsmanager_secret" "{{tfid $.Name}}_{{tfid .ServiceName}}_password" {
  name                    = "{{$.Name}}/{{.ServiceName}}/password"
  kms_key_id              = aws_kms_key.{{tfid $.Name}}.arn
  recovery_window_in_days = 7

  tags = {
    Name    = "{{$.Name}}-{{.ServiceName}}-password"
    Service = "{{.ServiceName}}"
  }
}

resource "aws_secretsmanager_secret_version" "{{tfid $.Name}}_{{tfid .ServiceName}}_password" {
  secret_id     = aws_secretsmanager_secret.{{tfid $.Name}}_{{tfid .ServiceName}}_password.id
  secret_string = random_password.{{tfid $.Name}}_{{tfid .ServiceName}}.result

  lifecycle {
    ignore_changes = [secret_string]
  }
}

resource "random_password" "{{tfid $.Name}}_{{tfid .ServiceName}}" {
  length           = 32
  special          = true
  override_special = "!#$%&*()-_=+[]{}<>:?"
}
{{- end}}
{{- if isDocDB .Engine}}

# ── Amazon DocumentDB ({{.Engine}} / {{.ServiceName}}) ────────────────────────

resource "aws_db_subnet_group" "{{tfid $.Name}}_{{tfid .ServiceName}}" {
  name       = "{{$.Name}}-{{.ServiceName}}-subnet-group"
  subnet_ids = [
    aws_subnet.{{tfid $.Name}}_private_1.id,
    aws_subnet.{{tfid $.Name}}_private_2.id,
  ]
  tags = { Name = "{{$.Name}}-{{.ServiceName}}-subnet-group" }
}

resource "aws_docdb_cluster" "{{tfid $.Name}}_{{tfid .ServiceName}}" {
  cluster_identifier      = "{{$.Name}}-{{.ServiceName}}-docdb"
  engine                  = "docdb"
  master_username         = "admin"
  master_password         = aws_secretsmanager_secret_version.{{tfid $.Name}}_{{tfid .ServiceName}}_password.secret_string
  storage_encrypted       = true
  kms_key_id              = aws_kms_key.{{tfid $.Name}}.arn
  vpc_security_group_ids  = [aws_security_group.{{tfid $.Name}}_db.id]
  db_subnet_group_name    = aws_db_subnet_group.{{tfid $.Name}}_{{tfid .ServiceName}}.name
  skip_final_snapshot     = false
  final_snapshot_identifier = "{{$.Name}}-{{.ServiceName}}-docdb-final"
  backup_retention_period = 7
  deletion_protection     = true

  tags = {
    Name = "{{$.Name}}-{{.ServiceName}}-docdb"
  }
}

resource "aws_docdb_cluster_instance" "{{tfid $.Name}}_{{tfid .ServiceName}}" {
  count              = 2
  identifier         = "{{$.Name}}-{{.ServiceName}}-docdb-${count.index}"
  cluster_identifier = aws_docdb_cluster.{{tfid $.Name}}_{{tfid .ServiceName}}.id
  instance_class     = "db.r6g.large"
}

resource "aws_secretsmanager_secret" "{{tfid $.Name}}_{{tfid .ServiceName}}_password" {
  name                    = "{{$.Name}}/{{.ServiceName}}/password"
  kms_key_id              = aws_kms_key.{{tfid $.Name}}.arn
  recovery_window_in_days = 7
  tags = { Name = "{{$.Name}}-{{.ServiceName}}-password" }
}

resource "aws_secretsmanager_secret_version" "{{tfid $.Name}}_{{tfid .ServiceName}}_password" {
  secret_id     = aws_secretsmanager_secret.{{tfid $.Name}}_{{tfid .ServiceName}}_password.id
  secret_string = random_password.{{tfid $.Name}}_{{tfid .ServiceName}}.result
  lifecycle { ignore_changes = [secret_string] }
}

resource "random_password" "{{tfid $.Name}}_{{tfid .ServiceName}}" {
  length           = 32
  special          = true
  override_special = "!#$%&*()-_=+[]{}<>:?"
}
{{- end}}
{{- if isRedis .Engine}}

# ── ElastiCache for Redis ({{.ServiceName}}) ──────────────────────────────────

resource "aws_elasticache_subnet_group" "{{tfid $.Name}}_{{tfid .ServiceName}}" {
  name       = "{{$.Name}}-{{.ServiceName}}-cache-subnet-group"
  subnet_ids = [
    aws_subnet.{{tfid $.Name}}_private_1.id,
    aws_subnet.{{tfid $.Name}}_private_2.id,
  ]
  tags = { Name = "{{$.Name}}-{{.ServiceName}}-cache-subnet-group" }
}

resource "aws_elasticache_replication_group" "{{tfid $.Name}}_{{tfid .ServiceName}}" {
  replication_group_id = "{{$.Name}}-{{.ServiceName}}"
  description          = "{{$.Name}} Redis replication group for {{.ServiceName}}"
  engine               = "redis"
  engine_version       = "7.0"
  node_type            = "cache.t3.micro"
  num_cache_clusters   = 2
  automatic_failover_enabled = true
  at_rest_encryption_enabled = true
  kms_key_id                 = aws_kms_key.{{tfid $.Name}}.arn
  transit_encryption_enabled = true
  subnet_group_name          = aws_elasticache_subnet_group.{{tfid $.Name}}_{{tfid .ServiceName}}.name
  security_group_ids         = [aws_security_group.{{tfid $.Name}}_db.id]

  tags = { Name = "{{$.Name}}-{{.ServiceName}}-redis" }
}
{{- end}}
{{- if isMemcached .Engine}}

# ── ElastiCache for Memcached ({{.ServiceName}}) ──────────────────────────────

resource "aws_elasticache_subnet_group" "{{tfid $.Name}}_{{tfid .ServiceName}}" {
  name       = "{{$.Name}}-{{.ServiceName}}-cache-subnet-group"
  subnet_ids = [
    aws_subnet.{{tfid $.Name}}_private_1.id,
    aws_subnet.{{tfid $.Name}}_private_2.id,
  ]
  tags = { Name = "{{$.Name}}-{{.ServiceName}}-cache-subnet-group" }
}

resource "aws_elasticache_cluster" "{{tfid $.Name}}_{{tfid .ServiceName}}" {
  cluster_id           = "{{$.Name}}-{{.ServiceName}}-memcached"
  engine               = "memcached"
  engine_version       = "1.6.22"
  node_type            = "cache.t3.micro"
  num_cache_nodes      = 2
  parameter_group_name = "default.memcached1.6"
  subnet_group_name    = aws_elasticache_subnet_group.{{tfid $.Name}}_{{tfid .ServiceName}}.name
  security_group_ids   = [aws_security_group.{{tfid $.Name}}_db.id]

  tags = { Name = "{{$.Name}}-{{.ServiceName}}-memcached" }
}
{{- end}}
{{- if isDynamoDB .Engine}}

# ── DynamoDB Table ({{.ServiceName}}) ─────────────────────────────────────────

resource "aws_dynamodb_table" "{{tfid $.Name}}_{{tfid .ServiceName}}_data" {
  name         = "{{$.Name}}-{{.ServiceName}}-table"
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
    kms_key_arn = aws_kms_key.{{tfid $.Name}}.arn
  }

  point_in_time_recovery {
    enabled = true
  }

  tags = { Name = "{{$.Name}}-{{.ServiceName}}-table" }
}
{{- end}}
{{- if isNeptune .Engine}}

# ── Amazon Neptune ({{.ServiceName}}) ─────────────────────────────────────────

resource "aws_neptune_subnet_group" "{{tfid $.Name}}_{{tfid .ServiceName}}" {
  name       = "{{$.Name}}-{{.ServiceName}}-neptune-subnet-group"
  subnet_ids = [
    aws_subnet.{{tfid $.Name}}_private_1.id,
    aws_subnet.{{tfid $.Name}}_private_2.id,
  ]
  tags = { Name = "{{$.Name}}-{{.ServiceName}}-neptune-subnet-group" }
}

resource "aws_neptune_cluster" "{{tfid $.Name}}_{{tfid .ServiceName}}" {
  cluster_identifier                  = "{{$.Name}}-{{.ServiceName}}-neptune"
  engine                              = "neptune"
  vpc_security_group_ids              = [aws_security_group.{{tfid $.Name}}_db.id]
  neptune_subnet_group_name           = aws_neptune_subnet_group.{{tfid $.Name}}_{{tfid .ServiceName}}.name
  storage_encrypted                   = true
  iam_database_authentication_enabled = true
  skip_final_snapshot                 = false
  final_snapshot_identifier           = "{{$.Name}}-{{.ServiceName}}-neptune-final"
  backup_retention_period             = 7
  deletion_protection                 = true

  tags = { Name = "{{$.Name}}-{{.ServiceName}}-neptune" }
}

resource "aws_neptune_cluster_instance" "{{tfid $.Name}}_{{tfid .ServiceName}}" {
  count              = 2
  identifier         = "{{$.Name}}-{{.ServiceName}}-neptune-${count.index}"
  cluster_identifier = aws_neptune_cluster.{{tfid $.Name}}_{{tfid .ServiceName}}.id
  instance_class     = "db.r6g.large"
}
{{- end}}
{{- if isCassandra .Engine}}

# ── Amazon Keyspaces (Cassandra-compatible / {{.ServiceName}}) ────────────────

resource "aws_keyspaces_keyspace" "{{tfid $.Name}}_{{tfid .ServiceName}}" {
  name = "{{tfid $.Name}}_{{tfid .ServiceName}}_keyspace"
  tags = { Name = "{{$.Name}}-{{.ServiceName}}-keyspace" }
}

resource "aws_keyspaces_table" "{{tfid $.Name}}_{{tfid .ServiceName}}" {
  keyspace_name = aws_keyspaces_keyspace.{{tfid $.Name}}_{{tfid .ServiceName}}.name
  table_name    = "{{tfid $.Name}}_{{tfid .ServiceName}}_table"

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
    kms_key_identifier = aws_kms_key.{{tfid $.Name}}.arn
  }

  point_in_time_recovery {
    status = "ENABLED"
  }

  tags = { Name = "{{$.Name}}-{{.ServiceName}}-keyspaces-table" }
}
{{- end}}
{{- if isTimestream .Engine}}

# ── Amazon Timestream ({{.ServiceName}}) ──────────────────────────────────────

resource "aws_timestreamwrite_database" "{{tfid $.Name}}_{{tfid .ServiceName}}" {
  database_name = "{{$.Name}}-{{.ServiceName}}-timestream"
  kms_key_id    = aws_kms_key.{{tfid $.Name}}.arn

  tags = { Name = "{{$.Name}}-{{.ServiceName}}-timestream" }
}

resource "aws_timestreamwrite_table" "{{tfid $.Name}}_{{tfid .ServiceName}}" {
  database_name = aws_timestreamwrite_database.{{tfid $.Name}}_{{tfid .ServiceName}}.database_name
  table_name    = "{{$.Name}}-{{.ServiceName}}-metrics"

  retention_properties {
    magnetic_store_retention_period_in_days = 365
    memory_store_retention_period_in_hours  = 24
  }

  magnetic_store_write_properties {
    enable_magnetic_store_writes = true
  }

  tags = { Name = "{{$.Name}}-{{.ServiceName}}-timestream-table" }
}
{{- end}}
{{- end}}
`

