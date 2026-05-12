// Package hardener applies SOC2 / NIST compliance rules to a Blueprint.
// Every rule is auto-remediated in place and recorded as a ComplianceControl
// so the generator can emit the audit trail as Terraform resource tags.
// Call Harden before passing the Blueprint to the generator.
package hardener

import (
	"fmt"
	"net"
	"strings"

	"github.com/justinclev/transcribe/pkg/models"
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

	// Build a map from compose service name → Terraform endpoint expression.
	id := strings.ReplaceAll(bp.Name, "-", "_")
	endpoints := map[string]string{} // keyed by compose service name
	for svcName, engine := range bp.DBServiceAliases {
		dbID := id + "_" + strings.ReplaceAll(svcName, "-", "_")
		switch engine {
		case models.EnginePostgres, models.EngineMySQL, models.EngineMariaDB,
			models.EngineOracle, models.EngineSQLServer:
			endpoints[svcName] = "${aws_db_instance." + dbID + ".address}"
		case models.EngineAuroraPostgres, models.EngineAuroraMySQL:
			endpoints[svcName] = "${aws_rds_cluster." + dbID + ".endpoint}"
		case models.EngineDocumentDB:
			endpoints[svcName] = "${aws_docdb_cluster." + dbID + ".endpoint}"
		case models.EngineRedis:
			endpoints[svcName] = "${aws_elasticache_replication_group." + dbID + ".primary_endpoint_address}"
		case models.EngineMemcached:
			endpoints[svcName] = "${aws_elasticache_cluster." + dbID + ".cluster_address}"
		case models.EngineNeptune:
			endpoints[svcName] = "${aws_neptune_cluster." + dbID + ".endpoint}"
		}
	}

	for i := range bp.Services {
		svc := &bp.Services[i]
		if svc.EnvVars == nil {
			svc.EnvVars = make(map[string]string)
		}

		for k, v := range svc.EnvVars {
			// Exact match: DB_HOST=db → DB_HOST=${aws_db_instance...}
			if ref, ok := endpoints[v]; ok {
				svc.EnvVars[k] = ref
				continue
			}
			// URL embedding: DATABASE_URL=postgres://db:5432/... →
			//   replace the hostname portion with the Terraform ref.
			for svcName, ref := range endpoints {
				// Match "://svcName:" or "://svcName/" or "://svcName" at end.
				for _, sep := range []string{"://" + svcName + ":", "://" + svcName + "/", "://" + svcName} {
					if strings.Contains(v, sep) {
						// ref is already in "${expr}" form — embed it directly
						// so Terraform can interpolate the hostname in the URL.
						replacement := strings.Replace(sep, svcName, ref, 1)
						svc.EnvVars[k] = strings.Replace(v, sep, replacement, 1)
						break
					}
				}
			}
		}

		// Auto-inject: if this service has no env var referencing a managed DB/cache
		// endpoint at all, add canonical host vars so containers can connect.
		for svcName, engine := range bp.DBServiceAliases {
			ref := endpoints[svcName]
			if ref == "" {
				continue
			}
			// Check if any existing env var already references this service name.
			alreadyReferenced := false
			for _, v := range svc.EnvVars {
				if strings.Contains(v, svcName) {
					alreadyReferenced = true
					break
				}
			}
			if alreadyReferenced {
				continue
			}
			// Inject a canonical host var based on engine type.
			switch engine {
			case models.EnginePostgres, models.EngineMySQL, models.EngineMariaDB,
				models.EngineOracle, models.EngineSQLServer,
				models.EngineAuroraPostgres, models.EngineAuroraMySQL,
				models.EngineDocumentDB:
				varName := strings.ToUpper(strings.ReplaceAll(svcName, "-", "_")) + "_HOST"
				if _, exists := svc.EnvVars[varName]; !exists {
					svc.EnvVars[varName] = ref
				}
			case models.EngineRedis:
				varName := strings.ToUpper(strings.ReplaceAll(svcName, "-", "_")) + "_HOST"
				if _, exists := svc.EnvVars[varName]; !exists {
					svc.EnvVars[varName] = ref
				}
			case models.EngineMemcached:
				varName := strings.ToUpper(strings.ReplaceAll(svcName, "-", "_")) + "_HOST"
				if _, exists := svc.EnvVars[varName]; !exists {
					svc.EnvVars[varName] = ref
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Rule 5 — Explicit secret mapping (SEC-02)
// ---------------------------------------------------------------------------

// wireDBSecrets resolves each name in Service.MappedSecrets to a concrete
// Secrets Manager ARN Terraform expression and stores it in SecretARNOverrides.
//
// Resolution order:
//  1. If the name matches a DB-generated password secret
//     (i.e. any DatabaseConfig.ServiceName is in DBServiceAliases and the name
//     looks like a password var), point to that secret.
//  2. Otherwise assume the var is managed by secretsTmpl and point to the
//     auto-generated aws_secretsmanager_secret resource for that service/var.
func wireDBSecrets(bp *models.Blueprint) {
	if len(bp.Services) == 0 {
		return
	}

	bpID := strings.ReplaceAll(bp.Name, "-", "_")

	// Build a quick lookup: DB service name → password secret resource id.
	// Relational + DocDB engines get a _password secret; cache engines don't.
	dbPasswordARN := map[string]string{} // DB ServiceName → tf expr
	for _, db := range bp.Databases {
		switch db.Engine {
		case models.EnginePostgres, models.EngineMySQL, models.EngineMariaDB,
			models.EngineOracle, models.EngineSQLServer,
			models.EngineAuroraPostgres, models.EngineAuroraMySQL,
			models.EngineDocumentDB:
			dbID := bpID + "_" + strings.ReplaceAll(db.ServiceName, "-", "_")
			dbPasswordARN[db.ServiceName] = "aws_secretsmanager_secret." + dbID + "_password.arn"
		}
	}
	// Backward-compat: single Database field with a ServiceName.
	if bp.Database.Engine != models.EngineNone && bp.Database.ServiceName != "" {
		if _, exists := dbPasswordARN[bp.Database.ServiceName]; !exists {
			dbID := bpID + "_" + strings.ReplaceAll(bp.Database.ServiceName, "-", "_")
			dbPasswordARN[bp.Database.ServiceName] = "aws_secretsmanager_secret." + dbID + "_password.arn"
		}
	}

	isPasswordLike := func(name string) bool {
		upper := strings.ToUpper(name)
		for _, kw := range []string{"PASSWORD", "PASSWD", "PASS"} {
			if strings.Contains(upper, kw) {
				return true
			}
		}
		return false
	}

	for i := range bp.Services {
		svc := &bp.Services[i]
		if len(svc.MappedSecrets) == 0 {
			continue
		}
		if svc.SecretARNOverrides == nil {
			svc.SecretARNOverrides = make(map[string]string)
		}
		svcID := strings.ReplaceAll(svc.Name, "-", "_")
		for _, varName := range svc.MappedSecrets {
			varID := strings.ReplaceAll(varName, "-", "_")
			// Try to match to a DB password secret first.
			matched := false
			if isPasswordLike(varName) {
				for _, arn := range dbPasswordARN {
					svc.SecretARNOverrides[varName] = arn
					matched = true
					break // use first DB password — caller can override via explicit mapping later
				}
			}
			if !matched {
				// Fall back to the service-scoped auto-generated secret.
				svc.SecretARNOverrides[varName] = "aws_secretsmanager_secret." + bpID + "_" + svcID + "_" + varID + ".arn"
			}
		}
	}
}
