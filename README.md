# TranScribe

TranScribe converts a `docker-compose.yml` file into hardened, SOC2-compliant infrastructure-as-code. Point it at your compose file and receive ready-to-apply Terraform, Pulumi, AWS CDK, or Helm output — with encryption, private networking, IAM least-privilege, and compliance tags applied automatically.

## How it works

```
docker-compose.yml  ──►  Parser  ──►  Hardener  ──►  Generator  ──►  IaC output
  (+ transcribe.yml)       │              │                │
                     Blueprint       SOC2 controls    Terraform / Pulumi / CDK / Helm
```

1. **Parser** — reads services, ports, and database images from your compose file. Known database images (`postgres`, `redis`, `mongo`, etc.) are automatically promoted to managed cloud services (RDS, ElastiCache, DocumentDB, …).
2. **Hardener** — enforces three SOC2 controls automatically:
   - `NET-01` — databases are forced into private subnets with no public endpoint
   - `NET-02` — an internet-facing load balancer is provisioned whenever any service exposes ports
   - `IAM-01` — a unique task IAM role is generated per service
3. **Generator** — renders the hardened Blueprint into the requested IaC format and provider.

---

## Supported targets

| Provider | `--provider` | Terraform | Pulumi | CDK | Helm |
| -------- | ------------ | :-------: | :----: | :-: | :--: |
| AWS      | `aws`        |     ✓     |   ✓    |  ✓  |  ✓   |
| Azure    | `azure`      |     ✓     |   ✓    |  —  |  ✓   |
| GCP      | `gcp`        |     ✓     |   ✓    |  —  |  ✓   |

CDK output is AWS-only. Helm output is cloud-agnostic.

### AWS Terraform — generated files

| File                 | Contents                                                                                                                                            |
| -------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| `main.tf`            | Provider config, Terraform backend, data sources                                                                                                    |
| `vpc.tf`             | VPC, public/private subnets, NAT gateway, Internet gateway                                                                                          |
| `iam.tf`             | Per-service ECS task roles                                                                                                                          |
| `ecs.tf`             | ECS cluster (Container Insights), task definitions, services, Application Auto Scaling (CPU + memory target tracking, when `max_count > min_count`) |
| `alb.tf`             | Application Load Balancer, listeners, target groups                                                                                                 |
| `security_groups.tf` | ALB, ECS, and DB security groups                                                                                                                    |
| `rds.tf`             | RDS / Aurora / ElastiCache / DynamoDB / Neptune / Keyspaces / Timestream                                                                            |
| `ecr.tf`             | ECR repositories per service                                                                                                                        |
| `logs.tf`            | CloudWatch log groups per service                                                                                                                   |
| `kms.tf`             | Customer-managed KMS key                                                                                                                            |
| `flow_logs.tf`       | VPC flow logs to CloudWatch                                                                                                                         |
| `secrets.tf`         | Secrets Manager secrets for each env var                                                                                                            |
| `backend.tf`         | S3 + DynamoDB Terraform remote state backend                                                                                                        |

---

## Supported database engines

TranScribe detects the following images automatically and maps them to the appropriate managed service:

| Compose image                                       | Cloud service         | Engine key  |
| --------------------------------------------------- | --------------------- | ----------- |
| `postgres`, `bitnami/postgresql`, `postgis/postgis` | RDS PostgreSQL        | `postgres`  |
| `mysql`, `bitnami/mysql`                            | RDS MySQL             | `mysql`     |
| `mariadb`, `bitnami/mariadb`                        | RDS MariaDB           | `mariadb`   |
| `mcr.microsoft.com/mssql/server`                    | RDS SQL Server        | `sqlserver` |
| `mongo`, `bitnami/mongodb`                          | DocumentDB            | `mongo`     |
| `redis`, `bitnami/redis`, `keydb`                   | ElastiCache Redis     | `redis`     |
| `memcached`                                         | ElastiCache Memcached | `memcached` |
| `cassandra`, `bitnami/cassandra`                    | Amazon Keyspaces      | `cassandra` |
| `neo4j`                                             | Amazon Neptune        | `neptune`   |

Use the `transcribe.yml` sidecar to override the detected engine or to specify engines like `aurora-postgres`, `aurora-mysql`, `dynamodb`, and `timestream` that have no common compose equivalent.

---

## Quickstart

### CLI

```bash
# Install
go install github.com/justinclev/transcribe/cmd/cli@latest

# Generate AWS Terraform (default)
transcribe -file path/to/docker-compose.yml

# With a transcribe.yml sidecar config
transcribe -file path/to/docker-compose.yml -config path/to/transcribe.yml

# Generate GCP Terraform
transcribe -file path/to/docker-compose.yml -provider gcp

# Generate AWS Pulumi
transcribe -file path/to/docker-compose.yml -format pulumi

# Generate a Helm chart
transcribe -file path/to/docker-compose.yml -format helm

# Output is written to ./out/
```

### API server

```bash
# Run locally
go run cmd/server/main.go          # listens on :8080

# Or with Docker Compose
docker compose up
```

#### `POST /api/v1/transcribe`

Upload your compose file as multipart form data.

```bash
# Minimal — AWS Terraform (defaults)
curl -X POST http://localhost:8080/api/v1/transcribe \
  -F "file=@docker-compose.yml" \
  -o out.zip

# With provider + format
curl -X POST http://localhost:8080/api/v1/transcribe \
  -F "file=@docker-compose.yml" \
  -F "provider=aws" \
  -F "format=terraform" \
  -o out.zip

# With a transcribe.yml sidecar
curl -X POST http://localhost:8080/api/v1/transcribe \
  -F "file=@docker-compose.yml" \
  -F "config=@transcribe.yml" \
  -F "provider=aws" \
  -F "format=terraform" \
  -o out.zip
```

**Form fields:**

| Field      | Required | Values                                  | Default     |
| ---------- | -------- | --------------------------------------- | ----------- |
| `file`     | ✓        | `docker-compose.yml`                    | —           |
| `config`   | —        | `transcribe.yml` sidecar                | —           |
| `provider` | —        | `aws` · `azure` · `gcp`                 | `aws`       |
| `format`   | —        | `terraform` · `pulumi` · `cdk` · `helm` | `terraform` |

The response is a `application/zip` archive containing all generated files.

**Health check:** `GET /healthz` → `{"status":"ok"}`

---

## `transcribe.yml` — sidecar config

Place a `transcribe.yml` file alongside your `docker-compose.yml` (or upload it via the `config` field) to override the defaults the parser infers automatically. Every field is optional — only the values you specify are applied.

```yaml
# transcribe.yml

# Override the project name (default: parent directory name)
name: my-app

# Target cloud region
# AWS default: us-east-1  |  Azure: eastus  |  GCP: us-central1
region: eu-west-1

# VPC CIDR block (default: 10.0.0.0/16)
vpc_cidr: 10.1.0.0/16

database:
  # Override the engine detected from the compose image name.
  # Useful when your image name is custom (e.g. "my-org/app-db:latest")
  # or when you want an engine that has no compose equivalent.
  #
  # Valid values: postgres | mysql | mariadb | sqlserver | oracle |
  #               aurora-postgres | aurora-mysql | mongo | redis |
  #               memcached | dynamodb | neptune | cassandra | timestream
  engine: aurora-postgres

  # RDS / ElastiCache instance size (default: db.t3.medium)
  instance_class: db.r6g.large

# Per-service sizing overrides.
# Services not listed here keep the defaults (cpu: 256, memory: 512,
# min_count: 1, max_count: 4).
services:
  api:
    cpu: 1024 # Fargate CPU units: 256 | 512 | 1024 | 2048 | 4096
    memory: 2048 # Fargate memory in MiB (must be compatible with cpu)
    min_count: 2 # minimum running task count (desired_count)
    max_count: 10 # maximum task count for autoscaling

  worker:
    cpu: 512
    memory: 1024
    min_count: 1
    max_count: 5
```

### Fargate CPU / memory compatibility

| `cpu` | Valid `memory` values (MiB)       |
| ----- | --------------------------------- |
| 256   | 512, 1024, 2048                   |
| 512   | 1024 – 4096 (in 1024 increments)  |
| 1024  | 2048 – 8192 (in 1024 increments)  |
| 2048  | 4096 – 16384 (in 1024 increments) |
| 4096  | 8192 – 30720 (in 1024 increments) |

---

## Development

### Prerequisites

- Go 1.22+
- Docker (optional, for containerised workflows)

### Run tests

```bash
go test ./...

# With race detector
go test -race ./...
```

### Run the server locally

```bash
go run cmd/server/main.go
# PORT env var overrides the default :8080
PORT=9090 go run cmd/server/main.go
```

### Docker Compose (recommended for local dev)

```bash
# Start API server with live reload
docker compose up

# Run the test suite once and exit
docker compose run --rm test

# Run the CLI against a local compose file
FILE=path/to/docker-compose.yml docker compose run --rm cli
```

### Project layout

```
cmd/
  cli/        — standalone CLI binary
  server/     — HTTP API server binary
internal/
  parser/     — docker-compose → Blueprint, transcribe.yml sidecar parser
  hardener/   — SOC2 control enforcement (NET-01, NET-02, IAM-01)
  generator/  — IaC rendering engine
    render/   — shared template helpers (RenderFile, WriteFiles, base FuncMap)
    aws/      — AWS Terraform templates (13 output files)
    azure/    — Azure Terraform templates
    gcp/      — GCP Terraform templates
    cdk/      — AWS CDK (TypeScript)
    helm/     — Kubernetes Helm chart
    pulumi/   — Pulumi TypeScript (AWS, Azure, GCP)
  api/        — HTTP handlers and route registration
pkg/
  models/     — shared data types (Blueprint, Service, DatabaseConfig, …)
```
