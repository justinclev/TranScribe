// Package parser translates a docker-compose.yml file into a Blueprint.
// It detects the compose schema version and dispatches to the appropriate
// version-specific parser, so new versions can be added without touching
// the public Parse function.
package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/justinclev/transcribe/internal/models"
)

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// Parse reads the docker-compose file at filePath, detects its schema version,
// and returns a populated Blueprint ready to be hardened and generated.
func Parse(filePath string) (*models.Blueprint, error) {
	data, err := os.ReadFile(filepath.Clean(filePath))
	if err != nil {
		return nil, fmt.Errorf("parser: reading file: %w", err)
	}

	version, err := detectVersion(data)
	if err != nil {
		return nil, err
	}

	p, err := parserForVersion(version)
	if err != nil {
		return nil, err
	}

	// Derive a project name from the directory containing the compose file.
	name := filepath.Base(filepath.Dir(filepath.Clean(filePath)))

	return p.parse(data, name)
}

// ---------------------------------------------------------------------------
// Version detection & dispatch
// ---------------------------------------------------------------------------

// composeParser is the interface each version-specific parser must implement.
// To support a new compose schema, implement this interface and register it
// in parserForVersion.
type composeParser interface {
	parse(data []byte, name string) (*models.Blueprint, error)
}

// versionProbe is the minimal struct needed to read the top-level version key.
type versionProbe struct {
	Version string `yaml:"version"`
}

func detectVersion(data []byte) (string, error) {
	var probe versionProbe
	if err := yaml.Unmarshal(data, &probe); err != nil {
		return "", fmt.Errorf("parser: invalid YAML: %w", err)
	}
	// Compose files without an explicit version key default to the latest
	// (schema v3 behavior for Docker Compose v2 CLI).
	if probe.Version == "" {
		return "3", nil
	}
	return probe.Version, nil
}

func parserForVersion(version string) (composeParser, error) {
	switch {
	case strings.HasPrefix(version, "3"):
		return &v3Parser{}, nil
	default:
		return nil, fmt.Errorf("parser: unsupported compose version %q", version)
	}
}

// ---------------------------------------------------------------------------
// Schema v3 parser
// ---------------------------------------------------------------------------

type v3Parser struct{}

// composeFileV3 mirrors the docker-compose 3.x YAML schema.
type composeFileV3 struct {
	Version  string                      `yaml:"version"`
	Services map[string]composeServiceV3 `yaml:"services"`
}

type composeServiceV3 struct {
	Image       string     `yaml:"image"`
	Ports       []string   `yaml:"ports"`
	Environment envVarNode `yaml:"environment"`
}

func (p *v3Parser) parse(data []byte, name string) (*models.Blueprint, error) {
	var cf composeFileV3
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("parser: decoding compose v3: %w", err)
	}

	bp := &models.Blueprint{
		Name:   name,
		Region: "us-east-1", // sensible default; operators can override
		Network: models.NetworkConfig{
			VPCCidr: "10.0.0.0/16",
		},
		DBServiceAliases: make(map[string]models.DatabaseEngine),
	}

	for svcName, svc := range cf.Services {
		// Detect well-known DB images and promote them to a managed DB.
		if engine := detectEngine(svc.Image); engine != models.EngineNone {
			db := models.DatabaseConfig{
				Engine:      engine,
				IsPrivate:   true, // default private for SOC2 CC6.1
				ServiceName: svcName,
			}
			bp.Databases = append(bp.Databases, db)
			bp.DBServiceAliases[svcName] = engine

			// Set the primary Database field: prefer relational engines (RDS) over
			// caches (Redis/Memcached), since most services care most about RDS endpoint.
			if bp.Database.Engine == models.EngineNone || isRelational(engine) && !isRelational(bp.Database.Engine) {
				bp.Database = db
			}
			// DB containers become managed resources, not Services.
			continue
		}

		bp.Services = append(bp.Services, models.Service{
			Name:            svcName,
			Image:           svc.Image,
			Ports:           svc.Ports,
			EnvVars:         svc.Environment,
			CPU:             256,
			Memory:          512,
			MinCount:        1,
			MaxCount:        4,
			HealthCheckPath: "/health",
		})
	}

	return bp, nil
}

// ---------------------------------------------------------------------------
// Image → engine detection
// ---------------------------------------------------------------------------

// imageEngineMap maps common Docker Hub image name prefixes to their AWS
// managed database equivalent. Entries are checked via strings.HasPrefix on
// the image name (tag stripped), so "postgres:15-alpine" matches "postgres".
var imageEngineMap = []struct {
	prefix string
	engine models.DatabaseEngine
}{
	// Relational (RDS)
	{"postgres", models.EnginePostgres},
	{"postgis/postgis", models.EnginePostgres},
	{"bitnami/postgresql", models.EnginePostgres},
	{"mysql", models.EngineMySQL},
	{"bitnami/mysql", models.EngineMySQL},
	{"mariadb", models.EngineMariaDB},
	{"bitnami/mariadb", models.EngineMariaDB},
	// SQL Server
	{"mcr.microsoft.com/mssql/server", models.EngineSQLServer},
	{"microsoft/mssql-server-linux", models.EngineSQLServer},
	// Document (DocumentDB)
	{"mongo", models.EngineDocumentDB},
	{"bitnami/mongodb", models.EngineDocumentDB},
	// In-memory (ElastiCache)
	{"redis", models.EngineRedis},
	{"bitnami/redis", models.EngineRedis},
	{"keydb", models.EngineRedis},
	{"memcached", models.EngineMemcached},
	// Wide-column (Keyspaces)
	{"cassandra", models.EngineCassandra},
	{"bitnami/cassandra", models.EngineCassandra},
	// Graph (Neptune) — neo4j is the closest local equivalent
	{"neo4j", models.EngineNeptune},
}

// isRelational returns true for RDS-compatible engines (Postgres, MySQL, etc.)
// These take priority over cache/NoSQL engines when choosing the primary DB.
func isRelational(e models.DatabaseEngine) bool {
	switch e {
	case models.EnginePostgres, models.EngineMySQL, models.EngineMariaDB,
		models.EngineOracle, models.EngineSQLServer,
		models.EngineAuroraPostgres, models.EngineAuroraMySQL:
		return true
	}
	return false
}

// detectEngine strips the image tag and checks it against imageEngineMap.
// Returns EngineNone when no match is found (i.e. a regular workload).
func detectEngine(image string) models.DatabaseEngine {
	// Strip tag: "postgres:15-alpine" → "postgres"
	base := strings.SplitN(image, ":", 2)[0]
	base = strings.ToLower(strings.TrimSpace(base))
	for _, entry := range imageEngineMap {
		if strings.HasPrefix(base, entry.prefix) {
			return entry.engine
		}
	}
	return models.EngineNone
}

// ---------------------------------------------------------------------------
// Custom YAML type: environment (list or map)
// ---------------------------------------------------------------------------

// envVarNode implements yaml.Unmarshaler to handle the two legal forms of the
// docker-compose `environment` key:
//
//	Map form:  environment: { KEY: value }
//	List form: environment: [ "KEY=value" ]
type envVarNode map[string]string

func (e *envVarNode) UnmarshalYAML(value *yaml.Node) error {
	*e = make(envVarNode)
	switch value.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(value.Content); i += 2 {
			(*e)[value.Content[i].Value] = value.Content[i+1].Value
		}
	case yaml.SequenceNode:
		for _, item := range value.Content {
			parts := strings.SplitN(item.Value, "=", 2)
			if len(parts) == 2 {
				(*e)[parts[0]] = parts[1]
			} else {
				(*e)[parts[0]] = ""
			}
		}
	default:
		return fmt.Errorf("parser: unexpected environment node kind %v", value.Kind)
	}
	return nil
}
