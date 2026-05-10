package generator

import (
	"fmt"

	"github.com/justinclev/transcribe/pkg/models"
)

// validateBlueprint checks for configuration errors that would produce invalid
// IaC output. Call this before dispatching to provider-specific generators.
func validateBlueprint(bp *models.Blueprint) error {
	for _, svc := range bp.Services {
		if err := validateFargateSizing(svc.Name, svc.CPU, svc.Memory); err != nil {
			return err
		}
	}
	return nil
}

// validateFargateSizing ensures the CPU/memory combination is valid for AWS Fargate.
// See: https://docs.aws.amazon.com/AmazonECS/latest/developerguide/task-cpu-memory-error.html
func validateFargateSizing(serviceName string, cpu, memory int) error {
	validCombinations := map[int][]int{
		256:  {512, 1024, 2048},
		512:  {1024, 2048, 3072, 4096},
		1024: {2048, 3072, 4096, 5120, 6144, 7168, 8192},
		2048: {4096, 5120, 6144, 7168, 8192, 9216, 10240, 11264, 12288, 13312, 14336, 15360, 16384},
		4096: {8192, 9216, 10240, 11264, 12288, 13312, 14336, 15360, 16384, 17408, 18432, 19456, 20480, 21504, 22528, 23552, 24576, 25600, 26624, 27648, 28672, 29696, 30720},
	}

	validMemory, cpuSupported := validCombinations[cpu]
	if !cpuSupported {
		return fmt.Errorf(
			"service %q: invalid Fargate CPU value %d (must be 256, 512, 1024, 2048, or 4096)",
			serviceName, cpu,
		)
	}

	for _, m := range validMemory {
		if m == memory {
			return nil
		}
	}

	return fmt.Errorf(
		"service %q: invalid Fargate cpu/memory combination (%d/%d). For cpu=%d, memory must be one of: %v",
		serviceName, cpu, memory, cpu, validMemory,
	)
}
