// Package hardener applies SOC2 / NIST compliance rules to a Blueprint.
// Every rule is auto-remediated in place and recorded as a ComplianceControl
// so the generator can emit the audit trail as Terraform resource tags.
// Call Harden before passing the Blueprint to the generator.
package hardener

import (
	"fmt"
	"net"
	"strings"

	"github.com/justinclev/transcribe/internal/models"
)

// Harden applies all three hardening rules to bp in place, then marks it as
// hardened. Subsequent calls are no-ops (idempotent).
func Harden(bp *models.Blueprint) {
	if bp.IsHardened {
		return
	}

	enforceZeroTrustNetworking(bp) // Rule 1 — NET-01: Database isolation
	enforceAutomaticPerimeter(bp)  // Rule 2 — NET-02: Internet-facing ALB
	enforceIAMLeastPrivilege(bp)   // Rule 3 — IAM-01: Unique task roles
	rewriteDBHostRefs(bp)          // Rule 4 — NET-03: Rewrite DB container-name env vars to Terraform refs
	wireDBSecrets(bp)              // Rule 5 — SEC-02: Map explicit secrets list to Secrets Manager ARNs

	bp.IsHardened = true
}

// ---------------------------------------------------------------------------
// Rule 1 — Zero-Trust Networking (NET-01)
// ---------------------------------------------------------------------------

// enforceZeroTrustNetworking ensures the VPC CIDR is valid and forces any
// managed database into a private subnet with no public endpoint.
func enforceZeroTrustNetworking(bp *models.Blueprint) {
	// Validate/default the VPC CIDR block.
	if _, _, err := net.ParseCIDR(bp.Network.VPCCidr); err != nil {
		bp.Network.VPCCidr = "10.0.0.0/16"
	}

	if bp.Database.Engine == models.EngineNone && len(bp.Databases) == 0 {
		return
	}

	bp.Database.IsPrivate = true
	for i := range bp.Databases {
		bp.Databases[i].IsPrivate = true
	}

	bp.ComplianceControls = append(bp.ComplianceControls, models.ComplianceControl{
		ControlID:   "NET-01",
		Description: "Database isolation enforced in private subnets",
	})
}

// ---------------------------------------------------------------------------
// Rule 2 — Automatic Perimeter (NET-02)
// ---------------------------------------------------------------------------

// enforceAutomaticPerimeter scans services and provisions an internet-facing
// ALB whenever at least one service exposes ports.
func enforceAutomaticPerimeter(bp *models.Blueprint) {
	for _, svc := range bp.Services {
		if len(svc.Ports) > 0 {
			bp.Network.PublicLoadBalancer = true
			bp.ComplianceControls = append(bp.ComplianceControls, models.ComplianceControl{
				ControlID:   "NET-02",
				Description: "Internet-facing ALB provisioned for public services",
			})
			return
		}
	}
}

// ---------------------------------------------------------------------------
// Rule 3 — IAM Least Privilege (IAM-01)
// ---------------------------------------------------------------------------

// enforceIAMLeastPrivilege generates a unique ECS task role name for every
// service and records the control against that service.
func enforceIAMLeastPrivilege(bp *models.Blueprint) {
	for i := range bp.Services {
		svc := &bp.Services[i]

		svc.IAMRoleName = fmt.Sprintf("%s-%s-task-role", bp.Name, svc.Name)

		svc.ComplianceControls = append(svc.ComplianceControls, models.ComplianceControl{
			ControlID:   "IAM-01",
			Description: "Unique task-specific IAM role generated",
		})
	}
}

// ---------------------------------------------------------------------------
// Rule 4 — Env-var DB host rewriting (NET-03)
// ---------------------------------------------------------------------------

// rewriteDBHostRefs scans all service env vars and replaces any VALUE that
// equals a compose service name found in DBServiceAliases with the correct
// Terraform interpolation expression for the managed database resource.
// This ensures ECS task definitions reference actual infrastructure endpoints
// rather than Docker container names (which don't exist in ECS).
func rewriteDBHostRefs(bp *models.Blueprint) {
	if len(bp.DBServiceAliases) == 0 {
		return
	}
	endpoints := buildEndpointMap(bp)
	for i := range bp.Services {
		rewriteServiceEnvVars(&bp.Services[i], endpoints, bp.DBServiceAliases)
	}
}

// buildEndpointMap returns a map from compose DB service name to its Terraform
// endpoint interpolation expression (e.g. "${aws_db_instance.myapp_db.address}").
func buildEndpointMap(bp *models.Blueprint) map[string]string {
	id := strings.ReplaceAll(bp.Name, "-", "_")
	endpoints := make(map[string]string, len(bp.DBServiceAliases))
	for svcName, engine := range bp.DBServiceAliases {
		dbID := id + "_" + strings.ReplaceAll(svcName, "-", "_")
		if ref := tfEndpointRef(engine, dbID); ref != "" {
			endpoints[svcName] = ref
		}
	}
	return endpoints
}

// tfEndpointRef returns the Terraform resource attribute reference for the
// given database engine and resource ID, or "" for unsupported engines.
func tfEndpointRef(engine models.DatabaseEngine, dbID string) string {
	switch engine {
	case models.EnginePostgres, models.EngineMySQL, models.EngineMariaDB,
		models.EngineOracle, models.EngineSQLServer:
		return "${aws_db_instance." + dbID + ".address}"
	case models.EngineAuroraPostgres, models.EngineAuroraMySQL:
		return "${aws_rds_cluster." + dbID + ".endpoint}"
	case models.EngineDocumentDB:
		return "${aws_docdb_cluster." + dbID + ".endpoint}"
	case models.EngineRedis:
		return "${aws_elasticache_replication_group." + dbID + ".primary_endpoint_address}"
	case models.EngineMemcached:
		return "${aws_elasticache_cluster." + dbID + ".cluster_address}"
	case models.EngineNeptune:
		return "${aws_neptune_cluster." + dbID + ".endpoint}"
	}
	return ""
}

// rewriteServiceEnvVars rewrites env var values for a single service:
// exact-match, URL-embedded, and auto-injected host vars.
func rewriteServiceEnvVars(svc *models.Service, endpoints map[string]string, aliases map[string]models.DatabaseEngine) {
	if svc.EnvVars == nil {
		svc.EnvVars = make(map[string]string)
	}
	for k, v := range svc.EnvVars {
		if rewritten, ok := rewriteEnvValue(v, endpoints); ok {
			svc.EnvVars[k] = rewritten
		}
	}
	autoInjectHostVars(svc, endpoints, aliases)
}

// rewriteEnvValue rewrites a single env var value if it matches a DB endpoint
// exactly or contains a DB service name as a URL hostname.
// Returns the rewritten value and true if a replacement was made.
func rewriteEnvValue(v string, endpoints map[string]string) (string, bool) {
	if ref, ok := endpoints[v]; ok {
		return ref, true
	}
	for svcName, ref := range endpoints {
		for _, sep := range []string{"://" + svcName + ":", "://" + svcName + "/", "://" + svcName} {
			if strings.Contains(v, sep) {
				replacement := strings.Replace(sep, svcName, ref, 1)
				return strings.Replace(v, sep, replacement, 1), true
			}
		}
	}
	return "", false
}

// autoInjectHostVars adds canonical *_HOST env vars for any managed DB/cache
// that the service does not already reference in its existing env vars.
func autoInjectHostVars(svc *models.Service, endpoints map[string]string, aliases map[string]models.DatabaseEngine) {
	for svcName, engine := range aliases {
		if !needsHostVar(engine) {
			continue
		}
		ref := endpoints[svcName]
		if ref == "" || envAlreadyReferences(svc.EnvVars, svcName) {
			continue
		}
		varName := strings.ToUpper(strings.ReplaceAll(svcName, "-", "_")) + "_HOST"
		if _, exists := svc.EnvVars[varName]; !exists {
			svc.EnvVars[varName] = ref
		}
	}
}

// needsHostVar returns true for engines that benefit from a *_HOST auto-inject.
func needsHostVar(engine models.DatabaseEngine) bool {
	switch engine {
	case models.EnginePostgres, models.EngineMySQL, models.EngineMariaDB,
		models.EngineOracle, models.EngineSQLServer,
		models.EngineAuroraPostgres, models.EngineAuroraMySQL,
		models.EngineDocumentDB, models.EngineRedis, models.EngineMemcached:
		return true
	}
	return false
}

// envAlreadyReferences returns true when any env var value already contains
// the given DB service name (indicating the service has an explicit reference).
func envAlreadyReferences(envVars map[string]string, svcName string) bool {
	for _, v := range envVars {
		if strings.Contains(v, svcName) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Rule 5 — Explicit secret mapping (SEC-02)
// ---------------------------------------------------------------------------

// wireDBSecrets resolves each name in Service.MappedSecrets to a concrete
// Secrets Manager ARN Terraform expression and stores it in SecretARNOverrides.
func wireDBSecrets(bp *models.Blueprint) {
	if len(bp.Services) == 0 {
		return
	}
	bpID := strings.ReplaceAll(bp.Name, "-", "_")
	dbPasswordARN := buildDBPasswordARNMap(bp, bpID)
	for i := range bp.Services {
		wireServiceSecrets(&bp.Services[i], bpID, dbPasswordARN)
	}
}

// buildDBPasswordARNMap returns a map from DB service name to its SM secret ARN
// expression for engines that generate a password secret (RDS, Aurora, DocDB).
func buildDBPasswordARNMap(bp *models.Blueprint, bpID string) map[string]string {
	m := make(map[string]string)
	for _, db := range bp.Databases {
		if !hasPasswordSecret(db.Engine) {
			continue
		}
		dbID := bpID + "_" + strings.ReplaceAll(db.ServiceName, "-", "_")
		m[db.ServiceName] = "aws_secretsmanager_secret." + dbID + "_password.arn"
	}
	// Backward-compat: single Database field.
	if bp.Database.Engine != models.EngineNone && bp.Database.ServiceName != "" {
		if _, exists := m[bp.Database.ServiceName]; !exists {
			dbID := bpID + "_" + strings.ReplaceAll(bp.Database.ServiceName, "-", "_")
			m[bp.Database.ServiceName] = "aws_secretsmanager_secret." + dbID + "_password.arn"
		}
	}
	return m
}

// hasPasswordSecret returns true for engines that create an SM password secret.
func hasPasswordSecret(engine models.DatabaseEngine) bool {
	switch engine {
	case models.EnginePostgres, models.EngineMySQL, models.EngineMariaDB,
		models.EngineOracle, models.EngineSQLServer,
		models.EngineAuroraPostgres, models.EngineAuroraMySQL,
		models.EngineDocumentDB:
		return true
	}
	return false
}

// wireServiceSecrets populates SecretARNOverrides for a single service.
func wireServiceSecrets(svc *models.Service, bpID string, dbPasswordARN map[string]string) {
	if len(svc.MappedSecrets) == 0 {
		return
	}
	if svc.SecretARNOverrides == nil {
		svc.SecretARNOverrides = make(map[string]string)
	}
	svcID := strings.ReplaceAll(svc.Name, "-", "_")
	for _, varName := range svc.MappedSecrets {
		svc.SecretARNOverrides[varName] = resolveSecretARN(varName, svcID, bpID, dbPasswordARN)
	}
}

// resolveSecretARN picks the right Secrets Manager ARN expression for a
// MappedSecret var: DB password secret if available, otherwise a new secret.
func resolveSecretARN(varName, svcID, bpID string, dbPasswordARN map[string]string) string {
	if isPasswordLikeVar(varName) {
		for _, arn := range dbPasswordARN {
			return arn // use first DB password secret
		}
	}
	varID := strings.ReplaceAll(varName, "-", "_")
	return "aws_secretsmanager_secret." + bpID + "_" + svcID + "_" + varID + ".arn"
}

// isPasswordLikeVar returns true when the name looks like a database password.
func isPasswordLikeVar(name string) bool {
	upper := strings.ToUpper(name)
	for _, kw := range []string{"PASSWORD", "PASSWD", "PASS"} {
		if strings.Contains(upper, kw) {
			return true
		}
	}
	return false
}
