// Package hardener applies SOC2 / NIST compliance rules to a Blueprint.
// Every rule is auto-remediated in place and recorded as a ComplianceControl
// so the generator can emit the audit trail as Terraform resource tags.
// Call Harden before passing the Blueprint to the generator.
package hardener

import (
	"fmt"
	"net"

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

	if bp.Database.Engine == models.EngineNone {
		return
	}

	bp.Database.IsPrivate = true

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
