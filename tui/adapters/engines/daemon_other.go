//go:build !unix

package engines

import (
	"context"
	"errors"
	"io"
	"time"
)

// IdleExit is how long a daemon runs without a client.
const IdleExit = 30 * time.Second

var errUnsupported = errors.New("shared engines need a unix system")

// Supervisor is unavailable here: every console starts its own engines.
type Supervisor struct {
	Dir        string
	Executable string
	Version    string
}

func (Supervisor) Socket(context.Context, Spec) (string, error) { return "", errUnsupported }

func Serve(context.Context, string, io.Reader, string, time.Duration) error { return errUnsupported }
