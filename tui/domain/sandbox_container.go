package domain

import (
	"errors"
	"math"
	"strings"
)

// SandboxContainer is the container of an execution whose profile is
// container: an image already on the host, the runtime that runs it
// (podman or docker; empty picks podman, else docker) and its limits, each
// zero for the runtime's default.
type SandboxContainer struct {
	Image    string
	Runtime  string
	MemoryMB int
	CPUs     float64
	PIDs     int
}

// Bounds of a container's limits.
const (
	MaxSandboxMemoryMB = 1 << 20
	MaxSandboxCPUs     = 1024
	MaxSandboxPIDs     = 1 << 22
)

func (c SandboxContainer) Validate() error {
	switch {
	case c.Image == "" || len(c.Image) > 255 || strings.HasPrefix(c.Image, "-") || strings.ContainsAny(c.Image, ", \t\r\n\x00"):
		return errors.New("a sandbox's container image is a reference of at most 255 characters, without spaces or commas")
	case c.Runtime != "" && c.Runtime != "podman" && c.Runtime != "docker":
		return errors.New("a sandbox's container runtime is podman or docker")
	case c.MemoryMB < 0 || c.MemoryMB > MaxSandboxMemoryMB:
		return errors.New("a sandbox's container memory_mb is 0 to 1048576")
	case math.IsNaN(c.CPUs) || c.CPUs < 0 || c.CPUs > MaxSandboxCPUs:
		return errors.New("a sandbox's container cpus is 0 to 1024")
	case c.PIDs < 0 || c.PIDs > MaxSandboxPIDs:
		return errors.New("a sandbox's container pids is 0 to 4194304")
	}
	return nil
}
