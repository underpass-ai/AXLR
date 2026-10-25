package dto

// ExecutionSandboxContainer is the container of the profile container: an
// image on the host, the runtime (podman or docker) and the limits, zero
// for the runtime's default.
type ExecutionSandboxContainer struct {
	Image    string  `json:"image"`
	Runtime  string  `json:"runtime,omitempty"`
	MemoryMB int     `json:"memory_mb,omitempty"`
	CPUs     float64 `json:"cpus,omitempty"`
	PIDs     int     `json:"pids,omitempty"`
}
