package local

// Container runs a sandbox's processes in a container of a rootless Podman
// or a Docker daemon: the image's filesystem read-only, the workspace and
// the writable paths bound read-write at their own paths, the readable
// ones read-only, a private /tmp, no capability, no new privilege, and the
// limits below. AXLR never pulls the image: it must be on the host.
type Container struct {
	// Runtime is the absolute path of podman or docker; Kind names which.
	Runtime string
	Kind    string
	Image   string
	// MemoryMB, CPUs and PIDs limit the container; zero for the runtime's
	// default.
	MemoryMB int
	CPUs     float64
	PIDs     int
	// User is the uid:gid the process runs as, the console's own, so what
	// it writes in the workspace is the person's.
	User string
}

// Container runtimes.
const (
	ContainerDocker = "docker"
	ContainerPodman = "podman"
)
