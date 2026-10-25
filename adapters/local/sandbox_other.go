//go:build !linux

package local

import (
	"context"
	"errors"

	"github.com/underpass-ai/AXLR/domain"
)

// errSandboxPlatform is why the sandbox does not run here: bubblewrap and
// the namespaces it uses are Linux's, as are the relay and the mounts the
// container profile uses.
var errSandboxPlatform = errors.New("the exec sandbox needs Linux and bubblewrap")

func (s *Sandbox) launch(string, string, string, []string) (launch, error) {
	return launch{}, domain.Reject("sandbox_unavailable", errSandboxPlatform.Error())
}

// ProbeSandbox reports that the sandbox cannot run on this platform.
func ProbeSandbox(context.Context, Sandbox) error { return errSandboxPlatform }

// ContainerRootless reports that the container profile cannot run on this
// platform.
func ContainerRootless(context.Context, Container) (bool, error) { return false, errSandboxPlatform }
