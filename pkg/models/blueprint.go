package models

// ComplianceControl records a SOC2 or NIST control that has been applied to
// this resource by the hardener. It is carried through to the generator so
// that control IDs can be emitted as Terraform resource tags.
type ComplianceControl struct {
	ControlID   string `json:"control_id"  yaml:"control_id"`
	Description string `json:"description" yaml:"description"`
}

// Service represents a single container workload sourced from a docker-compose
// service entry. It is the primary compute unit inside a Blueprint.
type Service struct {
	Name    string            `json:"name"    yaml:"name"`
	Image   string            `json:"image"   yaml:"image"`
	Ports   []string          `json:"ports"   yaml:"ports"` // e.g. ["8080:8080"]
	EnvVars map[string]string `json:"env_vars" yaml:"env_vars"`

	// Set by the hardener.
	IAMRoleName        string              `json:"iam_role_name"       yaml:"iam_role_name"`
	ComplianceControls []ComplianceControl `json:"compliance_controls" yaml:"compliance_controls"`
}

// NetworkConfig describes the target VPC topology and load-balancing strategy.
type NetworkConfig struct {
	VPCCidr            string `json:"vpc_cidr"            yaml:"vpc_cidr"`              // e.g. "10.0.0.0/16"
	PublicLoadBalancer bool   `json:"public_load_balancer" yaml:"public_load_balancer"` // true = internet-facing ALB
}

// DatabaseEngine enumerates supported managed database engines and their AWS
// service mapping. The generator switches on this value to emit the correct
// Terraform resource type.
type DatabaseEngine string

const (
	// Relational — maps to aws_db_instance (RDS).
	EnginePostgres  DatabaseEngine = "postgres"  // RDS PostgreSQL
	EngineMySQL     DatabaseEngine = "mysql"     // RDS MySQL
	EngineMariaDB   DatabaseEngine = "mariadb"   // RDS MariaDB
	EngineOracle    DatabaseEngine = "oracle"    // RDS Oracle (SE2/EE)
	EngineSQLServer DatabaseEngine = "sqlserver" // RDS SQL Server

	// Aurora — maps to aws_rds_cluster (Aurora Serverless v2 or provisioned).
	EngineAuroraPostgres DatabaseEngine = "aurora-postgres" // Aurora PostgreSQL-compatible
	EngineAuroraMySQL    DatabaseEngine = "aurora-mysql"    // Aurora MySQL-compatible

	// Document — maps to aws_docdb_cluster.
	EngineDocumentDB DatabaseEngine = "mongo" // Amazon DocumentDB (MongoDB-compatible)

	// In-memory — maps to aws_elasticache_replication_group.
	EngineRedis     DatabaseEngine = "redis"     // ElastiCache for Redis
	EngineMemcached DatabaseEngine = "memcached" // ElastiCache for Memcached

	// Key-value — maps to aws_dynamodb_table.
	EngineDynamoDB DatabaseEngine = "dynamodb" // Amazon DynamoDB

	// Graph — maps to aws_neptune_cluster.
	EngineNeptune DatabaseEngine = "neptune" // Amazon Neptune (openCypher / Gremlin / SPARQL)

	// Wide-column — maps to aws_keyspaces_table.
	EngineCassandra DatabaseEngine = "cassandra" // Amazon Keyspaces (Cassandra-compatible)

	// Time-series — maps to Amazon Timestream (aws_timestreamwrite_database).
	EngineTimestream DatabaseEngine = "timestream" // Amazon Timestream

	// None — no managed database; any DB runs as a Service container.
	EngineNone DatabaseEngine = ""
)

// DatabaseConfig describes an optional managed database to provision
// alongside the services.
type DatabaseConfig struct {
	Engine    DatabaseEngine `json:"engine"    yaml:"engine"`      // see DatabaseEngine constants
	IsPrivate bool           `json:"is_private" yaml:"is_private"` // true = no public endpoint
}

// Blueprint is the central intermediary data model. A docker-compose file is
// parsed and normalised into a Blueprint, which the hardener then enforces
// SOC2 controls on before the generator renders it into Terraform HCL.
type Blueprint struct {
	Name     string         `json:"name"     yaml:"name"`
	Region   string         `json:"region"   yaml:"region"` // AWS region, e.g. "us-east-1"
	Services []Service      `json:"services" yaml:"services"`
	Network  NetworkConfig  `json:"network"  yaml:"network"`
	Database DatabaseConfig `json:"database" yaml:"database"`

	// Set by the hardener.
	IsHardened         bool                `json:"is_hardened"         yaml:"is_hardened"`
	ComplianceControls []ComplianceControl `json:"compliance_controls" yaml:"compliance_controls"`
}
