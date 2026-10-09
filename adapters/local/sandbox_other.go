//go:build !linux

package local

import (
	"context"
	"errors"

	"github.com/underpass-ai/AXLR/domain"
)

// errSandboxPlatform is why the sandbox does not run here: bubblewrap and
// the namespaces it uses are Linux's.
var errSandboxPlatform = errors.New("the exec sandbox needs Linux and bubblewrap")

func (s *Sandbox) wrap(string, string, string, []string) (string, []string, error) {
	return "", nil, domain.Reject("sandbox_unavailable", errSandboxPlatform.Error())
}

// ProbeSandbox reports that the sandbox cannot run on this platform.
func ProbeSandbox(context.Context, Sandbox) error { return errSandboxPlatform }
