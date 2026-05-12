package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/justinclev/transcribe/pkg/models"
)

// ---------------------------------------------------------------------------
// TranscribeConfig — sidecar config schema
// ---------------------------------------------------------------------------

// TranscribeConfig is the schema for the optional transcribe.yml sidecar file.
// Any field left empty/zero retains the value already set by the compose parser.
type TranscribeConfig struct {
	// Name overrides the project name derived from the directory.
	Name string `yaml:"name"`

	// Region sets the target cloud region (e.g. "eu-west-1", "eastus").
	Region string `yaml:"region"`

	// VPCCidr overrides the default 10.0.0.0/16 network block.
	VPCCidr string `yaml:"vpc_cidr"`

	// Domain sets the domain name for the ACM certificate (e.g. "api.example.com").
	// If empty, the ALB template will use a Terraform variable placeholder.
	Domain string `yaml:"domain"`

	// Database allows overriding auto-detected DB settings.
	Database struct {
		// Engine overrides the engine inferred from the compose image name.
		// Must be one of the models.DatabaseEngine constants.
		Engine string `yaml:"engine"`

		// InstanceClass sets the RDS/ElastiCache instance size (e.g. "db.r6g.large").
		InstanceClass string `yaml:"instance_class"`
	} `yaml:"database"`

	// Services is a map from service name to per-service sizing config.
	// Only services named here are affected; unmentioned services keep their defaults.
	Services map[string]ServiceConfig `yaml:"services"`
}

// ServiceConfig holds the per-service overrides from transcribe.yml.
type ServiceConfig struct {
	CPU             int      `yaml:"cpu"`               // Fargate CPU units (256, 512, 1024, 2048, 4096)
	Memory          int      `yaml:"memory"`            // Fargate memory in MiB
	MinCount        int      `yaml:"min_count"`         // minimum desired running task count
	MaxCount        int      `yaml:"max_count"`         // maximum desired task count (autoscaling ceiling)
	HealthCheckPath string   `yaml:"health_check_path"` // ALB target group health check path (default: /health)
	// Secrets lists env var names that must be injected from Secrets Manager.
	// For password-like names, the hardener wires them to the DB-generated
	// SM secret so the container receives the actual RDS password, not a
	// separately generated random value.
	Secrets []string `yaml:"secrets"`
}

// ---------------------------------------------------------------------------
// ParseConfig
// ---------------------------------------------------------------------------

// ParseConfig reads configPath, parses it as TranscribeConfig YAML, and
// merges the non-zero values into bp. It is safe to call with an empty path
// (no-op) and safe to call when the file doesn't exist (also a no-op).
func ParseConfig(configPath string, bp *models.Blueprint) error {
	if configPath == "" {
		return nil
	}

	data, err := os.ReadFile(filepath.Clean(configPath))
	if err != nil {
		if os.IsNotExist(err) {
			return nil // sidecar is optional
		}
		return fmt.Errorf("config: reading %s: %w", configPath, err)
	}

	var cfg TranscribeConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("config: invalid YAML in %s: %w", configPath, err)
	}

	applyConfig(&cfg, bp)
	return nil
}

// applyConfig merges non-zero fields from cfg into bp.
func applyConfig(cfg *TranscribeConfig, bp *models.Blueprint) {
	if cfg.Name != "" {
		bp.Name = cfg.Name
	}
	if cfg.Region != "" {
		bp.Region = cfg.Region
	}
	if cfg.VPCCidr != "" {
		bp.Network.VPCCidr = cfg.VPCCidr
	}
	if cfg.Domain != "" {
		bp.Network.Domain = cfg.Domain
	}

	// Database overrides.
	if cfg.Database.Engine != "" {
		engine := models.DatabaseEngine(strings.ToLower(cfg.Database.Engine))
		bp.Database.Engine = engine
	}
	if cfg.Database.InstanceClass != "" {
		bp.Database.InstanceClass = cfg.Database.InstanceClass
	}

	// Per-service sizing overrides.
	for i := range bp.Services {
		svc := &bp.Services[i]
		override, ok := cfg.Services[svc.Name]
		if !ok {
			continue
		}
		if override.CPU > 0 {
			svc.CPU = override.CPU
		}
		if override.Memory > 0 {
			svc.Memory = override.Memory
		}
		if override.MinCount > 0 {
			svc.MinCount = override.MinCount
		}
		if override.MaxCount > 0 {
			svc.MaxCount = override.MaxCount
		}
		if override.HealthCheckPath != "" {
			svc.HealthCheckPath = override.HealthCheckPath
		}
		if len(override.Secrets) > 0 {
			svc.MappedSecrets = append(svc.MappedSecrets, override.Secrets...)
		}
	}
}
