package parser

import (
	"errors"
	"os"
	"sort"
	"testing"

	"github.com/justinclev/transcribe/pkg/models"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func writeTempCompose(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "docker-compose-*.yml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}

func serviceByName(t *testing.T, bp *models.Blueprint, name string) models.Service {
	t.Helper()
	for _, s := range bp.Services {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("service %q not found in blueprint", name)
	return models.Service{}
}

// ---------------------------------------------------------------------------
// detectEngine (white-box — same package)
// ---------------------------------------------------------------------------

func TestDetectEngine_AllEngines(t *testing.T) {
	tests := []struct {
		image string
		want  models.DatabaseEngine
	}{
		// Relational (RDS)
		{"postgres", models.EnginePostgres},
		{"postgres:15", models.EnginePostgres},
		{"postgres:15-alpine", models.EnginePostgres},
		{"postgis/postgis:15-3.4-alpine", models.EnginePostgres},
		{"bitnami/postgresql:16", models.EnginePostgres},
		{"mysql", models.EngineMySQL},
		{"mysql:8.0", models.EngineMySQL},
		{"bitnami/mysql:8.0", models.EngineMySQL},
		{"mariadb", models.EngineMariaDB},
		{"mariadb:10.11", models.EngineMariaDB},
		{"bitnami/mariadb:10.11", models.EngineMariaDB},
		{"mcr.microsoft.com/mssql/server:2022-latest", models.EngineSQLServer},
		{"microsoft/mssql-server-linux", models.EngineSQLServer},
		// Document (DocumentDB)
		{"mongo", models.EngineDocumentDB},
		{"mongo:7", models.EngineDocumentDB},
		{"mongo:latest", models.EngineDocumentDB},
		{"bitnami/mongodb:7.0", models.EngineDocumentDB},
		// In-memory (ElastiCache)
		{"redis", models.EngineRedis},
		{"redis:7-alpine", models.EngineRedis},
		{"bitnami/redis:7.2", models.EngineRedis},
		{"keydb:latest", models.EngineRedis},
		{"memcached", models.EngineMemcached},
		{"memcached:1.6-alpine", models.EngineMemcached},
		// Wide-column (Keyspaces)
		{"cassandra", models.EngineCassandra},
		{"cassandra:4.1", models.EngineCassandra},
		{"bitnami/cassandra:4.1", models.EngineCassandra},
		// Graph (Neptune)
		{"neo4j", models.EngineNeptune},
		{"neo4j:5-community", models.EngineNeptune},
		// Not a DB
		{"nginx:latest", models.EngineNone},
		{"my-org/api-server:1.2.3", models.EngineNone},
		{"", models.EngineNone},
		{"node:20-alpine", models.EngineNone},
		{"python:3.12-slim", models.EngineNone},
		{"golang:1.22", models.EngineNone},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.image, func(t *testing.T) {
			if got := detectEngine(tc.image); got != tc.want {
				t.Errorf("detectEngine(%q) = %q, want %q", tc.image, got, tc.want)
			}
		})
	}
}

func TestDetectEngine_CaseInsensitive(t *testing.T) {
	if got := detectEngine("Postgres:15"); got != models.EnginePostgres {
		t.Errorf("expected EnginePostgres for mixed-case image, got %q", got)
	}
}

// A digest-addressed image should still be detected (colon splits before the @).
func TestDetectEngine_DigestImage(t *testing.T) {
	image := "postgres@sha256:abc123"
	if got := detectEngine(image); got != models.EnginePostgres {
		t.Errorf("detectEngine(%q) = %q, want EnginePostgres", image, got)
	}
}

// ---------------------------------------------------------------------------
// Environment variable parsing
// ---------------------------------------------------------------------------

func TestParse_EnvMapForm(t *testing.T) {
	compose := "version: \"3.8\"\nservices:\n  api:\n    image: nginx:latest\n    environment:\n      KEY_ONE: value_one\n      KEY_TWO: value_two\n      EMPTY_KEY:\n"
	path := writeTempCompose(t, compose)
	bp, err := Parse(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	svc := serviceByName(t, bp, "api")
	if svc.EnvVars["KEY_ONE"] != "value_one" {
		t.Errorf("KEY_ONE: got %q", svc.EnvVars["KEY_ONE"])
	}
	if svc.EnvVars["KEY_TWO"] != "value_two" {
		t.Errorf("KEY_TWO: got %q", svc.EnvVars["KEY_TWO"])
	}
	if _, ok := svc.EnvVars["EMPTY_KEY"]; !ok {
		t.Error("EMPTY_KEY should be present with empty value")
	}
}

func TestParse_EnvListForm(t *testing.T) {
	compose := "version: \"3.8\"\nservices:\n  api:\n    image: nginx:latest\n    environment:\n      - KEY_A=alpha\n      - KEY_B=beta=with=equals\n      - KEY_ONLY\n"
	path := writeTempCompose(t, compose)
	bp, err := Parse(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	svc := serviceByName(t, bp, "api")
	if svc.EnvVars["KEY_A"] != "alpha" {
		t.Errorf("KEY_A: got %q", svc.EnvVars["KEY_A"])
	}
	// Only the first '=' splits — the rest of the value is preserved.
	if svc.EnvVars["KEY_B"] != "beta=with=equals" {
		t.Errorf("KEY_B: got %q", svc.EnvVars["KEY_B"])
	}
	if v, ok := svc.EnvVars["KEY_ONLY"]; !ok || v != "" {
		t.Errorf("KEY_ONLY: present=%v value=%q", ok, v)
	}
}

func TestParse_NoEnvironmentKey(t *testing.T) {
	compose := "version: \"3.8\"\nservices:\n  api:\n    image: nginx:latest\n"
	path := writeTempCompose(t, compose)
	bp, err := Parse(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	svc := serviceByName(t, bp, "api")
	if len(svc.EnvVars) != 0 {
		t.Errorf("expected empty EnvVars, got %v", svc.EnvVars)
	}
}

// ---------------------------------------------------------------------------
// Ports
// ---------------------------------------------------------------------------

func TestParse_Ports_SingleMapping(t *testing.T) {
	compose := "version: \"3.8\"\nservices:\n  api:\n    image: nginx:latest\n    ports:\n      - \"80:80\"\n"
	path := writeTempCompose(t, compose)
	bp, err := Parse(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	svc := serviceByName(t, bp, "api")
	if len(svc.Ports) != 1 || svc.Ports[0] != "80:80" {
		t.Errorf("unexpected ports: %v", svc.Ports)
	}
}

func TestParse_Ports_MultipleMappings(t *testing.T) {
	compose := "version: \"3.8\"\nservices:\n  api:\n    image: nginx:latest\n    ports:\n      - \"80:80\"\n      - \"443:443\"\n      - \"8080:8080\"\n"
	path := writeTempCompose(t, compose)
	bp, err := Parse(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	svc := serviceByName(t, bp, "api")
	if len(svc.Ports) != 3 {
		t.Errorf("expected 3 ports, got %d: %v", len(svc.Ports), svc.Ports)
	}
}

func TestParse_Ports_HostOnly(t *testing.T) {
	compose := "version: \"3.8\"\nservices:\n  api:\n    image: nginx:latest\n    ports:\n      - \"9000\"\n"
	path := writeTempCompose(t, compose)
	bp, err := Parse(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	svc := serviceByName(t, bp, "api")
	if len(svc.Ports) != 1 || svc.Ports[0] != "9000" {
		t.Errorf("unexpected ports: %v", svc.Ports)
	}
}

func TestParse_NoPorts(t *testing.T) {
	compose := "version: \"3.8\"\nservices:\n  worker:\n    image: my-org/worker:latest\n"
	path := writeTempCompose(t, compose)
	bp, err := Parse(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	svc := serviceByName(t, bp, "worker")
	if len(svc.Ports) != 0 {
		t.Errorf("expected no ports, got %v", svc.Ports)
	}
}

// ---------------------------------------------------------------------------
// Database engine detection (via full Parse path)
// ---------------------------------------------------------------------------

func testDBEngine(t *testing.T, dbImage string, want models.DatabaseEngine) {
	t.Helper()
	compose := "version: \"3.8\"\nservices:\n  app:\n    image: nginx:latest\n  db:\n    image: " + dbImage + "\n"
	path := writeTempCompose(t, compose)
	bp, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if bp.Database.Engine != want {
		t.Errorf("image %q: engine=%q, want %q", dbImage, bp.Database.Engine, want)
	}
	if !bp.Database.IsPrivate {
		t.Error("database must default to IsPrivate=true")
	}
	for _, svc := range bp.Services {
		if svc.Image == dbImage {
			t.Errorf("DB image %q must be promoted to Blueprint.Database, not remain as a Service", dbImage)
		}
	}
}

func TestParse_DB_Postgres(t *testing.T) {
	testDBEngine(t, "postgres:15-alpine", models.EnginePostgres)
}
func TestParse_DB_MySQL(t *testing.T)     { testDBEngine(t, "mysql:8.0", models.EngineMySQL) }
func TestParse_DB_MariaDB(t *testing.T)   { testDBEngine(t, "mariadb:10.11", models.EngineMariaDB) }
func TestParse_DB_Mongo(t *testing.T)     { testDBEngine(t, "mongo:7", models.EngineDocumentDB) }
func TestParse_DB_Redis(t *testing.T)     { testDBEngine(t, "redis:7-alpine", models.EngineRedis) }
func TestParse_DB_Memcached(t *testing.T) { testDBEngine(t, "memcached:1.6", models.EngineMemcached) }
func TestParse_DB_Cassandra(t *testing.T) { testDBEngine(t, "cassandra:4.1", models.EngineCassandra) }
func TestParse_DB_Neo4j(t *testing.T)     { testDBEngine(t, "neo4j:5-community", models.EngineNeptune) }
func TestParse_DB_MSSQL(t *testing.T) {
	testDBEngine(t, "mcr.microsoft.com/mssql/server:2022-latest", models.EngineSQLServer)
}
func TestParse_DB_BitnamiPG(t *testing.T) {
	testDBEngine(t, "bitnami/postgresql:16", models.EnginePostgres)
}
func TestParse_DB_BitnamiMongo(t *testing.T) {
	testDBEngine(t, "bitnami/mongodb:7.0", models.EngineDocumentDB)
}
func TestParse_DB_KeyDB(t *testing.T) { testDBEngine(t, "keydb:latest", models.EngineRedis) }

// When multiple DB images appear, only the first one (map iteration order) wins.
func TestParse_MultipleDB_FirstWins(t *testing.T) {
	compose := "version: \"3.8\"\nservices:\n  app:\n    image: nginx:latest\n  db1:\n    image: postgres:15\n  cache:\n    image: redis:7\n"
	path := writeTempCompose(t, compose)
	bp, err := Parse(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bp.Database.Engine != models.EnginePostgres && bp.Database.Engine != models.EngineRedis {
		t.Errorf("unexpected engine %q", bp.Database.Engine)
	}
}

// ---------------------------------------------------------------------------
// Multiple services
// ---------------------------------------------------------------------------

func TestParse_MultipleAppServices(t *testing.T) {
	compose := "version: \"3.8\"\nservices:\n  frontend:\n    image: nginx:latest\n    ports:\n      - \"80:80\"\n  backend:\n    image: my-org/api:2.0\n    ports:\n      - \"8080:8080\"\n  worker:\n    image: my-org/worker:2.0\n"
	path := writeTempCompose(t, compose)
	bp, err := Parse(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bp.Services) != 3 {
		t.Errorf("expected 3 services, got %d", len(bp.Services))
	}
	names := make([]string, len(bp.Services))
	for i, s := range bp.Services {
		names[i] = s.Name
	}
	sort.Strings(names)
	for i, want := range []string{"backend", "frontend", "worker"} {
		if names[i] != want {
			t.Errorf("service[%d] = %q, want %q", i, names[i], want)
		}
	}
	if bp.Database.Engine != models.EngineNone {
		t.Errorf("expected no database, got %q", bp.Database.Engine)
	}
}

func TestParse_ServicesOnlyDB_NoAppServices(t *testing.T) {
	compose := "version: \"3.8\"\nservices:\n  db:\n    image: postgres:15\n"
	path := writeTempCompose(t, compose)
	bp, err := Parse(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bp.Services) != 0 {
		t.Errorf("DB-only compose should have 0 app services, got %d", len(bp.Services))
	}
	if bp.Database.Engine != models.EnginePostgres {
		t.Errorf("engine=%q, want EnginePostgres", bp.Database.Engine)
	}
}

// ---------------------------------------------------------------------------
// Version detection
// ---------------------------------------------------------------------------

func TestParse_NoVersionKey_DefaultsToV3(t *testing.T) {
	compose := "services:\n  api:\n    image: nginx:latest\n"
	path := writeTempCompose(t, compose)
	if _, err := Parse(path); err != nil {
		t.Fatalf("missing version key should not error, got: %v", err)
	}
}

func TestParse_Version3x_Variants(t *testing.T) {
	for _, ver := range []string{"3", "3.0", "3.8", "3.9"} {
		ver := ver
		t.Run(ver, func(t *testing.T) {
			compose := "version: \"" + ver + "\"\nservices:\n  api:\n    image: nginx:latest\n"
			path := writeTempCompose(t, compose)
			if _, err := Parse(path); err != nil {
				t.Fatalf("version %q should be supported, got: %v", ver, err)
			}
		})
	}
}

func TestParse_UnsupportedVersion_ReturnsError(t *testing.T) {
	compose := "version: \"2.4\"\nservices:\n  api:\n    image: nginx:latest\n"
	path := writeTempCompose(t, compose)
	_, err := Parse(path)
	if err == nil {
		t.Fatal("expected error for unsupported compose version 2.4")
	}
}

// ---------------------------------------------------------------------------
// Defaults
// ---------------------------------------------------------------------------

func TestParse_DefaultRegion(t *testing.T) {
	compose := "version: \"3.8\"\nservices:\n  api:\n    image: nginx:latest\n"
	path := writeTempCompose(t, compose)
	bp, err := Parse(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bp.Region == "" {
		t.Error("Region must not be empty")
	}
}

func TestParse_DefaultVPCCidr(t *testing.T) {
	compose := "version: \"3.8\"\nservices:\n  api:\n    image: nginx:latest\n"
	path := writeTempCompose(t, compose)
	bp, err := Parse(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bp.Network.VPCCidr != "10.0.0.0/16" {
		t.Errorf("VPCCidr=%q, want 10.0.0.0/16", bp.Network.VPCCidr)
	}
}

func TestParse_ProjectNameFromDirectory(t *testing.T) {
	compose := "version: \"3.8\"\nservices:\n  api:\n    image: nginx:latest\n"
	path := writeTempCompose(t, compose)
	bp, err := Parse(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bp.Name == "" {
		t.Error("Blueprint.Name must not be empty")
	}
}

// ---------------------------------------------------------------------------
// Error paths
// ---------------------------------------------------------------------------

func TestParse_FileNotFound(t *testing.T) {
	_, err := Parse("/nonexistent/path/docker-compose.yml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected os.ErrNotExist in error chain, got: %v", err)
	}
}

func TestParse_InvalidYAML_ReturnsError(t *testing.T) {
	path := writeTempCompose(t, "this: is: not: valid: yaml: ][")
	_, err := Parse(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestParse_EmptyFile_ReturnsBlueprint(t *testing.T) {
	path := writeTempCompose(t, "")
	bp, err := Parse(path)
	if err != nil {
		t.Fatalf("empty file should not error, got: %v", err)
	}
	if len(bp.Services) != 0 {
		t.Errorf("expected no services from empty file, got %d", len(bp.Services))
	}
}

// A scalar environment value (neither map nor sequence) must return an error
// that propagates from UnmarshalYAML through yaml.Unmarshal.
func TestParse_ScalarEnvironment_ReturnsError(t *testing.T) {
	compose := "version: \"3.8\"\nservices:\n  api:\n    image: nginx:latest\n    environment: scalar_value\n"
	path := writeTempCompose(t, compose)
	_, err := Parse(path)
	if err == nil {
		t.Fatal("expected error when environment is a scalar, got nil")
	}
}

// ---------------------------------------------------------------------------
// ParseConfig — sidecar transcribe.yml
// ---------------------------------------------------------------------------

func writeTempConfig(t *testing.T, dir, content string) string {
	t.Helper()
	path := dir + "/transcribe.yml"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func baseBlueprint(t *testing.T) *models.Blueprint {
	t.Helper()
	dir := t.TempDir()
	compose := "version: \"3.8\"\nservices:\n  api:\n    image: nginx:latest\n    ports:\n      - \"8080:8080\"\n  worker:\n    image: my/worker:1.0\n"
	composePath := dir + "/docker-compose.yml"
	if err := os.WriteFile(composePath, []byte(compose), 0o600); err != nil {
		t.Fatal(err)
	}
	bp, err := Parse(composePath)
	if err != nil {
		t.Fatalf("base compose parse failed: %v", err)
	}
	return bp
}

func TestParseConfig_EmptyPath_IsNoop(t *testing.T) {
	bp := baseBlueprint(t)
	origName := bp.Name
	if err := ParseConfig("", bp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bp.Name != origName {
		t.Error("ParseConfig with empty path should not modify the blueprint")
	}
}

func TestParseConfig_MissingFile_IsNoop(t *testing.T) {
	bp := baseBlueprint(t)
	origRegion := bp.Region
	if err := ParseConfig("/nonexistent/transcribe.yml", bp); err != nil {
		t.Fatalf("missing sidecar file should not error, got: %v", err)
	}
	if bp.Region != origRegion {
		t.Error("missing sidecar file should not modify the blueprint")
	}
}

func TestParseConfig_OverridesName(t *testing.T) {
	bp := baseBlueprint(t)
	dir := t.TempDir()
	cfgPath := writeTempConfig(t, dir, "name: my-app\n")
	if err := ParseConfig(cfgPath, bp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bp.Name != "my-app" {
		t.Errorf("Name=%q, want my-app", bp.Name)
	}
}

func TestParseConfig_OverridesRegion(t *testing.T) {
	bp := baseBlueprint(t)
	dir := t.TempDir()
	cfgPath := writeTempConfig(t, dir, "region: eu-west-1\n")
	if err := ParseConfig(cfgPath, bp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bp.Region != "eu-west-1" {
		t.Errorf("Region=%q, want eu-west-1", bp.Region)
	}
}

func TestParseConfig_OverridesVPCCidr(t *testing.T) {
	bp := baseBlueprint(t)
	dir := t.TempDir()
	cfgPath := writeTempConfig(t, dir, "vpc_cidr: 172.16.0.0/12\n")
	if err := ParseConfig(cfgPath, bp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bp.Network.VPCCidr != "172.16.0.0/12" {
		t.Errorf("VPCCidr=%q, want 172.16.0.0/12", bp.Network.VPCCidr)
	}
}

func TestParseConfig_OverridesDatabaseEngine(t *testing.T) {
	bp := baseBlueprint(t)
	dir := t.TempDir()
	cfgPath := writeTempConfig(t, dir, "database:\n  engine: aurora-postgres\n")
	if err := ParseConfig(cfgPath, bp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bp.Database.Engine != models.EngineAuroraPostgres {
		t.Errorf("Database.Engine=%q, want aurora-postgres", bp.Database.Engine)
	}
}

func TestParseConfig_OverridesDatabaseInstanceClass(t *testing.T) {
	bp := baseBlueprint(t)
	dir := t.TempDir()
	cfgPath := writeTempConfig(t, dir, "database:\n  instance_class: db.r6g.large\n")
	if err := ParseConfig(cfgPath, bp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bp.Database.InstanceClass != "db.r6g.large" {
		t.Errorf("Database.InstanceClass=%q, want db.r6g.large", bp.Database.InstanceClass)
	}
}

func TestParseConfig_OverridesServiceSizing(t *testing.T) {
	bp := baseBlueprint(t)
	dir := t.TempDir()
	cfgPath := writeTempConfig(t, dir, "services:\n  api:\n    cpu: 1024\n    memory: 2048\n    min_count: 2\n    max_count: 10\n")
	if err := ParseConfig(cfgPath, bp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	api := serviceByName(t, bp, "api")
	if api.CPU != 1024 {
		t.Errorf("api.CPU=%d, want 1024", api.CPU)
	}
	if api.Memory != 2048 {
		t.Errorf("api.Memory=%d, want 2048", api.Memory)
	}
	if api.MinCount != 2 {
		t.Errorf("api.MinCount=%d, want 2", api.MinCount)
	}
	if api.MaxCount != 10 {
		t.Errorf("api.MaxCount=%d, want 10", api.MaxCount)
	}
}

func TestParseConfig_UnmentionedService_Unchanged(t *testing.T) {
	bp := baseBlueprint(t)
	dir := t.TempDir()
	// Only override 'api'; 'worker' should keep defaults.
	cfgPath := writeTempConfig(t, dir, "services:\n  api:\n    cpu: 512\n")
	if err := ParseConfig(cfgPath, bp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	worker := serviceByName(t, bp, "worker")
	if worker.CPU != 256 {
		t.Errorf("worker.CPU=%d, want default 256", worker.CPU)
	}
}

func TestParseConfig_InvalidYAML_ReturnsError(t *testing.T) {
	bp := baseBlueprint(t)
	dir := t.TempDir()
	cfgPath := writeTempConfig(t, dir, "this: is: broken: yaml: ][")
	if err := ParseConfig(cfgPath, bp); err == nil {
		t.Fatal("expected error for invalid config YAML")
	}
}

func TestParseConfig_DefaultServiceSizing(t *testing.T) {
	bp := baseBlueprint(t)
	api := serviceByName(t, bp, "api")
	if api.CPU != 256 {
		t.Errorf("default CPU=%d, want 256", api.CPU)
	}
	if api.Memory != 512 {
		t.Errorf("default Memory=%d, want 512", api.Memory)
	}
	if api.MinCount != 1 {
		t.Errorf("default MinCount=%d, want 1", api.MinCount)
	}
	if api.MaxCount != 4 {
		t.Errorf("default MaxCount=%d, want 4", api.MaxCount)
	}
}
