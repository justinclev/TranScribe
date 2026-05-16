package hardener

import (
	"strings"
	"testing"

	"github.com/justinclev/transcribe/internal/models"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newBP(engine models.DatabaseEngine, svcs ...models.Service) *models.Blueprint {
	return &models.Blueprint{
		Name:     "test-app",
		Region:   "us-east-1",
		Services: svcs,
		Network:  models.NetworkConfig{VPCCidr: "10.0.0.0/16"},
		Database: models.DatabaseConfig{Engine: engine},
	}
}

func svc(name string, ports ...string) models.Service {
	return models.Service{Name: name, Ports: ports}
}

func hasCtrl(controls []models.ComplianceControl, id string) bool {
	for _, c := range controls {
		if c.ControlID == id {
			return true
		}
	}
	return false
}

func countCtrl(controls []models.ComplianceControl, id string) int {
	n := 0
	for _, c := range controls {
		if c.ControlID == id {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// IsHardened flag
// ---------------------------------------------------------------------------

func TestHarden_SetsIsHardened(t *testing.T) {
	bp := newBP(models.EngineNone)
	if bp.IsHardened {
		t.Fatal("blueprint must not start as hardened")
	}
	Harden(bp)
	if !bp.IsHardened {
		t.Error("IsHardened must be true after Harden")
	}
}

// ---------------------------------------------------------------------------
// Idempotency
// ---------------------------------------------------------------------------

func TestHarden_Idempotent_DoesNotDoubleStamp(t *testing.T) {
	bp := newBP(models.EnginePostgres, svc("api", "8080:8080"))
	Harden(bp)
	ctrlsBefore := len(bp.ComplianceControls)
	roleBefore := bp.Services[0].IAMRoleName
	Harden(bp)
	if len(bp.ComplianceControls) != ctrlsBefore {
		t.Errorf("second Harden added controls: was %d, now %d", ctrlsBefore, len(bp.ComplianceControls))
	}
	if bp.Services[0].IAMRoleName != roleBefore {
		t.Error("second Harden mutated IAMRoleName")
	}
}

func TestHarden_Idempotent_ServiceControls(t *testing.T) {
	bp := newBP(models.EngineNone, svc("api"))
	Harden(bp)
	svcCtrlsBefore := len(bp.Services[0].ComplianceControls)
	Harden(bp)
	if len(bp.Services[0].ComplianceControls) != svcCtrlsBefore {
		t.Errorf("second Harden added service-level controls: was %d, now %d",
			svcCtrlsBefore, len(bp.Services[0].ComplianceControls))
	}
}

// ---------------------------------------------------------------------------
// Rule 1 — NET-01: Zero-Trust Networking
// ---------------------------------------------------------------------------

func TestHarden_NET01_WithDatabase_SetsPrivate(t *testing.T) {
	bp := newBP(models.EnginePostgres)
	bp.Database.IsPrivate = false
	Harden(bp)
	if !bp.Database.IsPrivate {
		t.Error("Database.IsPrivate must be forced to true")
	}
}

func TestHarden_NET01_WithDatabase_StampsControl(t *testing.T) {
	bp := newBP(models.EnginePostgres)
	Harden(bp)
	if !hasCtrl(bp.ComplianceControls, "NET-01") {
		t.Error("NET-01 control not found on blueprint")
	}
}

func TestHarden_NET01_WithDatabase_ControlDescription(t *testing.T) {
	bp := newBP(models.EngineMySQL)
	Harden(bp)
	for _, c := range bp.ComplianceControls {
		if c.ControlID == "NET-01" {
			want := "Database isolation enforced in private subnets"
			if c.Description != want {
				t.Errorf("NET-01 description = %q, want %q", c.Description, want)
			}
		}
	}
}

func TestHarden_NET01_NoDatabase_NoControl(t *testing.T) {
	bp := newBP(models.EngineNone)
	Harden(bp)
	if hasCtrl(bp.ComplianceControls, "NET-01") {
		t.Error("NET-01 must not be stamped when there is no database")
	}
}

func TestHarden_NET01_AllDatabaseEngines(t *testing.T) {
	engines := []models.DatabaseEngine{
		models.EnginePostgres, models.EngineMySQL, models.EngineMariaDB,
		models.EngineOracle, models.EngineSQLServer,
		models.EngineAuroraPostgres, models.EngineAuroraMySQL,
		models.EngineDocumentDB, models.EngineRedis, models.EngineMemcached,
		models.EngineDynamoDB, models.EngineNeptune,
		models.EngineCassandra, models.EngineTimestream,
	}
	for _, eng := range engines {
		eng := eng
		t.Run(string(eng), func(t *testing.T) {
			bp := newBP(eng)
			Harden(bp)
			if !bp.Database.IsPrivate {
				t.Errorf("engine %q: IsPrivate must be true", eng)
			}
			if !hasCtrl(bp.ComplianceControls, "NET-01") {
				t.Errorf("engine %q: NET-01 missing", eng)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// VPC CIDR validation (inside enforceZeroTrustNetworking)
// ---------------------------------------------------------------------------

func TestHarden_InvalidCIDR_IsDefaulted(t *testing.T) {
	bp := newBP(models.EngineNone)
	bp.Network.VPCCidr = "not-a-cidr"
	Harden(bp)
	if bp.Network.VPCCidr != "10.0.0.0/16" {
		t.Errorf("invalid CIDR not replaced: %q", bp.Network.VPCCidr)
	}
}

func TestHarden_EmptyCIDR_IsDefaulted(t *testing.T) {
	bp := newBP(models.EngineNone)
	bp.Network.VPCCidr = ""
	Harden(bp)
	if bp.Network.VPCCidr != "10.0.0.0/16" {
		t.Errorf("empty CIDR not replaced: %q", bp.Network.VPCCidr)
	}
}

func TestHarden_ValidCIDR_IsPreserved(t *testing.T) {
	for _, cidr := range []string{"172.16.0.0/12", "192.168.0.0/24", "10.1.2.0/23"} {
		cidr := cidr
		t.Run(cidr, func(t *testing.T) {
			bp := newBP(models.EngineNone)
			bp.Network.VPCCidr = cidr
			Harden(bp)
			if bp.Network.VPCCidr != cidr {
				t.Errorf("valid CIDR was changed to: %q", bp.Network.VPCCidr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Rule 2 — NET-02: Automatic Perimeter
// ---------------------------------------------------------------------------

func TestHarden_NET02_ServiceWithPorts_SetsALB(t *testing.T) {
	bp := newBP(models.EngineNone, svc("api", "8080:8080"))
	Harden(bp)
	if !bp.Network.PublicLoadBalancer {
		t.Error("PublicLoadBalancer must be true when a service exposes ports")
	}
}

func TestHarden_NET02_ServiceWithPorts_StampsControl(t *testing.T) {
	bp := newBP(models.EngineNone, svc("api", "80:80"))
	Harden(bp)
	if !hasCtrl(bp.ComplianceControls, "NET-02") {
		t.Error("NET-02 control not found")
	}
}

func TestHarden_NET02_ServiceWithPorts_ControlDescription(t *testing.T) {
	bp := newBP(models.EngineNone, svc("api", "80:80"))
	Harden(bp)
	for _, c := range bp.ComplianceControls {
		if c.ControlID == "NET-02" {
			want := "Internet-facing ALB provisioned for public services"
			if c.Description != want {
				t.Errorf("NET-02 description = %q, want %q", c.Description, want)
			}
		}
	}
}

func TestHarden_NET02_ServiceWithoutPorts_NoALB(t *testing.T) {
	bp := newBP(models.EngineNone, svc("worker"))
	Harden(bp)
	if bp.Network.PublicLoadBalancer {
		t.Error("PublicLoadBalancer must be false when no service has ports")
	}
	if hasCtrl(bp.ComplianceControls, "NET-02") {
		t.Error("NET-02 must not be stamped when no service has ports")
	}
}

func TestHarden_NET02_NoServices_NoALB(t *testing.T) {
	bp := newBP(models.EngineNone)
	Harden(bp)
	if bp.Network.PublicLoadBalancer {
		t.Error("PublicLoadBalancer must be false with no services")
	}
	if hasCtrl(bp.ComplianceControls, "NET-02") {
		t.Error("NET-02 must not be stamped with no services")
	}
}

func TestHarden_NET02_MultipleServices_OnlyOneWithPorts_StampedOnce(t *testing.T) {
	bp := newBP(models.EngineNone,
		svc("worker"),
		svc("api", "8080:8080"),
		svc("metrics", "9090:9090"),
	)
	Harden(bp)
	if !bp.Network.PublicLoadBalancer {
		t.Error("PublicLoadBalancer must be true")
	}
	if n := countCtrl(bp.ComplianceControls, "NET-02"); n != 1 {
		t.Errorf("NET-02 stamped %d times, want exactly 1", n)
	}
}

func TestHarden_NET02_MultiplePortsOnSameService_StampedOnce(t *testing.T) {
	bp := newBP(models.EngineNone, svc("api", "80:80", "443:443", "8080:8080"))
	Harden(bp)
	if n := countCtrl(bp.ComplianceControls, "NET-02"); n != 1 {
		t.Errorf("NET-02 stamped %d times, want exactly 1", n)
	}
}

func TestHarden_NET02_AllServicesHavePorts_StampedOnce(t *testing.T) {
	bp := newBP(models.EngineNone,
		svc("frontend", "80:80"),
		svc("backend", "8080:8080"),
	)
	Harden(bp)
	if n := countCtrl(bp.ComplianceControls, "NET-02"); n != 1 {
		t.Errorf("NET-02 stamped %d times, want exactly 1", n)
	}
}

// ---------------------------------------------------------------------------
// Rule 3 — IAM-01: Least Privilege
// ---------------------------------------------------------------------------

func TestHarden_IAM01_RoleNameFormat(t *testing.T) {
	bp := newBP(models.EngineNone, svc("api"))
	bp.Name = "my-app"
	Harden(bp)
	if bp.Services[0].IAMRoleName != "my-app-api-task-role" {
		t.Errorf("unexpected role name: %q", bp.Services[0].IAMRoleName)
	}
}

func TestHarden_IAM01_PerServiceControl(t *testing.T) {
	bp := newBP(models.EngineNone, svc("api"), svc("worker"), svc("cron"))
	Harden(bp)
	for _, s := range bp.Services {
		if !hasCtrl(s.ComplianceControls, "IAM-01") {
			t.Errorf("service %q missing IAM-01 control", s.Name)
		}
	}
}

func TestHarden_IAM01_ControlDescription(t *testing.T) {
	bp := newBP(models.EngineNone, svc("api"))
	Harden(bp)
	for _, c := range bp.Services[0].ComplianceControls {
		if c.ControlID == "IAM-01" {
			want := "Unique task-specific IAM role generated"
			if c.Description != want {
				t.Errorf("IAM-01 description = %q, want %q", c.Description, want)
			}
		}
	}
}

func TestHarden_IAM01_NoServices_NoPanic(t *testing.T) {
	bp := newBP(models.EngineNone)
	Harden(bp) // must not panic
}

func TestHarden_IAM01_UniqueRolesAcrossServices(t *testing.T) {
	bp := newBP(models.EngineNone,
		svc("frontend"), svc("backend"), svc("worker"), svc("cron"),
	)
	bp.Name = "myapp"
	Harden(bp)
	seen := make(map[string]bool)
	for _, s := range bp.Services {
		if seen[s.IAMRoleName] {
			t.Errorf("duplicate IAM role name: %q", s.IAMRoleName)
		}
		seen[s.IAMRoleName] = true
		if !strings.HasPrefix(s.IAMRoleName, "myapp-") {
			t.Errorf("role %q missing prefix 'myapp-'", s.IAMRoleName)
		}
		if !strings.HasSuffix(s.IAMRoleName, "-task-role") {
			t.Errorf("role %q missing suffix '-task-role'", s.IAMRoleName)
		}
	}
}

func TestHarden_IAM01_RoleContainsServiceName(t *testing.T) {
	for _, name := range []string{"api", "worker", "metrics-exporter", "db-migrator"} {
		name := name
		t.Run(name, func(t *testing.T) {
			bp := newBP(models.EngineNone, svc(name))
			bp.Name = "app"
			Harden(bp)
			if !strings.Contains(bp.Services[0].IAMRoleName, name) {
				t.Errorf("role %q does not contain service name %q",
					bp.Services[0].IAMRoleName, name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Combined full-stack scenarios
// ---------------------------------------------------------------------------

func TestHarden_FullStack_AllRulesApplied(t *testing.T) {
	bp := &models.Blueprint{
		Name:   "fullstack",
		Region: "us-west-2",
		Services: []models.Service{
			{Name: "web", Ports: []string{"80:80"}},
			{Name: "api", Ports: []string{"8080:8080"}},
			{Name: "worker"},
		},
		Network:  models.NetworkConfig{VPCCidr: "10.0.0.0/16"},
		Database: models.DatabaseConfig{Engine: models.EnginePostgres},
	}
	Harden(bp)

	if !bp.IsHardened {
		t.Error("IsHardened not set")
	}
	if !bp.Database.IsPrivate {
		t.Error("Database.IsPrivate not set")
	}
	if !hasCtrl(bp.ComplianceControls, "NET-01") {
		t.Error("NET-01 missing from blueprint")
	}
	if !bp.Network.PublicLoadBalancer {
		t.Error("PublicLoadBalancer not set")
	}
	if !hasCtrl(bp.ComplianceControls, "NET-02") {
		t.Error("NET-02 missing from blueprint")
	}
	for _, s := range bp.Services {
		if s.IAMRoleName == "" {
			t.Errorf("service %q has empty IAMRoleName", s.Name)
		}
		if !hasCtrl(s.ComplianceControls, "IAM-01") {
			t.Errorf("service %q missing IAM-01", s.Name)
		}
	}
}

func TestHarden_DBOnly_NoNet02_NoIAM01(t *testing.T) {
	bp := newBP(models.EngineRedis) // no services
	Harden(bp)
	if hasCtrl(bp.ComplianceControls, "NET-02") {
		t.Error("NET-02 must not appear with no services")
	}
	if len(bp.Services) != 0 {
		for _, s := range bp.Services {
			if hasCtrl(s.ComplianceControls, "IAM-01") {
				t.Errorf("IAM-01 on service %q but services was empty", s.Name)
			}
		}
	}
}

func TestHarden_AppOnly_NoNet01(t *testing.T) {
	bp := newBP(models.EngineNone, svc("api", "8080:8080"))
	Harden(bp)
	if hasCtrl(bp.ComplianceControls, "NET-01") {
		t.Error("NET-01 must not appear when there is no database")
	}
}
