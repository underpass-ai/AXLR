//go:build linux

package local

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// On 10 October 2026 bwrap failed on an Ubuntu whose AppArmor restricts
// unprivileged user namespaces and the probe named only bwrap's symptom.
// The sysctl is read through readAppArmorUserNamespaces, replaced here, so
// the test does not depend on the machine.
func TestTheSandboxProbeNamesUbuntusAppArmorRestriction(t *testing.T) {
	failing, err := exec.LookPath("false")
	if err != nil {
		t.Skip("no false program to stand in for a refusing bwrap")
	}
	saved := readAppArmorUserNamespaces
	t.Cleanup(func() { readAppArmorUserNamespaces = saved })
	for value, named := range map[string]bool{"1\n": true, "0\n": false, "": false} {
		readAppArmorUserNamespaces = func() ([]byte, error) {
			if value == "" {
				return nil, errors.New("no such file")
			}
			return []byte(value), nil
		}
		err := ProbeSandbox(context.Background(), Sandbox{Program: failing})
		if err == nil || !strings.HasPrefix(err.Error(), "bwrap cannot create a sandbox here: ") {
			t.Fatalf("sysctl %q: %v", value, err)
		}
		if strings.Contains(err.Error(), "kernel.apparmor_restrict_unprivileged_userns=1") != named || strings.Contains(err.Error(), "AppArmor profile for /usr/bin/bwrap") != named {
			t.Fatalf("sysctl %q: %v", value, err)
		}
	}
}
