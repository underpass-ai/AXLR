package runtime

import (
	"time"

	"github.com/underpass-ai/AXLR/adapters/local"
	"github.com/underpass-ai/AXLR/application"
)

const (
	ProtocolVersion    = 1
	MaxRequestBytes    = 4 << 20
	MaxResponseBytes   = 4 << 20
	hardFileBytes      = 1 << 20
	defaultReadBytes   = 64 << 10
	defaultOutputBytes = 256 << 10
)

// HardTimeout is the longest timeout an exec request may ask for.
const HardTimeout = 5 * time.Minute

type Config struct {
	Root string
	Env  []string
	// Sandbox confines exec; nil runs commands unconfined.
	Sandbox        *local.Sandbox
	Plugins        application.PluginToolPort
	MaxReadBytes   int
	MaxFileBytes   int
	MaxOutputBytes int
	MaxTimeout     time.Duration
}
