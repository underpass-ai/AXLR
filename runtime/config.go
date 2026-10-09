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

// Search and list bounds. A walk stops at searchFiles files or
// searchScanBytes bytes read and reports limit_reached; a page holds at most
// its result count and max_bytes of encoded output, 64 KiB by default.
const (
	defaultSearchResults = 50
	maxSearchResults     = 200
	maxSearchContext     = 5
	defaultListEntries   = 200
	maxListEntries       = 500
	defaultListDepth     = 3
	maxListDepth         = 8
	defaultListingBytes  = 64 << 10
	searchFiles          = 20000
	searchScanBytes      = 64 << 20
	listVisits           = 20000
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
