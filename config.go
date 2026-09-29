package axlr

import "time"

const (
	ProtocolVersion    = 1
	MaxRequestBytes    = 4 << 20
	MaxResponseBytes   = 4 << 20
	hardFileBytes      = 1 << 20
	hardTimeout        = 5 * time.Minute
	defaultReadBytes   = 64 << 10
	defaultOutputBytes = 256 << 10
)

type Config struct {
	Root           string
	Env            []string
	MaxReadBytes   int
	MaxFileBytes   int
	MaxOutputBytes int
	MaxTimeout     time.Duration
}
