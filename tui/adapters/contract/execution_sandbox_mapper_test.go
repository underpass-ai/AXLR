package contract

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// The sandbox object reads bare or wrapped, takes its egress default from
// its profile, writes each destination as the contract reads it, and
// refuses what the contract does not describe.
func TestTheSandboxObjectReadsStrictly(t *testing.T) {
	sandbox, err := ParseExecutionSandbox([]byte(`{"sandbox": {"profile": "process", "egress": "allowlist", "allow": ["API.GitHub.com:443", "*.pypi.org"], "writable": ["/cache"], "readable": ["/models"]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if sandbox.Profile != domain.SandboxProfileProcess || sandbox.Egress != domain.SandboxEgressAllowlist || strings.Join(sandbox.Allow, " ") != "api.github.com:443 *.pypi.org" {
		t.Fatalf("%+v", sandbox)
	}
	for raw, egress := range map[string]domain.SandboxEgress{
		`{"profile": "process"}`: domain.SandboxEgressNone,
		`{"profile": "none"}`:    domain.SandboxEgressFull,
		`{"profile": "container", "container": {"image": "tools:1", "runtime": "docker", "memory_mb": 256, "cpus": 0.5, "pids": 32}}`: domain.SandboxEgressNone,
		`{"profile": "process", "egress": "full"}`: domain.SandboxEgressFull,
	} {
		parsed, err := ParseExecutionSandbox([]byte(raw))
		if err != nil || parsed.Egress != egress {
			t.Fatalf("%s: %+v %v", raw, parsed, err)
		}
	}
	for _, raw := range []string{
		`{"profile": "process", "network": false}`,
		`{"egress": "none"}`,
		`{"profile": "vm"}`,
		`{"profile": "process", "egress": "some"}`,
		`{"profile": "process", "egress": "allowlist"}`,
		`{"profile": "process", "allow": ["example.com"]}`,
		`{"profile": "process", "egress": "allowlist", "allow": ["http://example.com"]}`,
		`{"profile": "none", "egress": "none"}`,
		`{"profile": "none", "writable": ["/cache"]}`,
		`{"profile": "process", "writable": ["relative"]}`,
		`{"profile": "process", "readable": ["/"]}`,
		`{"profile": "process", "writable": ["/a,b"]}`,
		`{"profile": "process", "container": {"image": "tools:1"}}`,
		`{"profile": "container"}`,
		`{"profile": "container", "container": {"image": ""}}`,
		`{"profile": "container", "container": {"image": "--privileged"}}`,
		`{"profile": "container", "container": {"image": "tools:1", "runtime": "lxc"}}`,
		`{"profile": "container", "container": {"image": "tools:1", "memory_mb": -1}}`,
		`{"profile": "container", "container": {"image": "tools:1", "cpus": 2000}}`,
		`{"profile": "process"} {}`,
		`"process"`,
	} {
		if parsed, err := ParseExecutionSandbox([]byte(raw)); err == nil {
			t.Fatalf("accepted %s as %+v", raw, parsed)
		}
	}
	if _, err := ParseExecutionSandbox([]byte(`{"profile": "process", "allow": [` + strings.Repeat(`"a.example.com",`, 4000) + `"b.example.com"]}`)); err == nil || !strings.Contains(err.Error(), "32 KiB") {
		t.Fatalf("%v", err)
	}
}

// The wire form spells the egress out, so the receipt's digest does not
// depend on whether the default was written.
func TestTheSandboxWireFormIsTheNormalForm(t *testing.T) {
	bare, err := ParseExecutionSandbox([]byte(`{"profile": "process"}`))
	if err != nil {
		t.Fatal(err)
	}
	spelled, err := ParseExecutionSandbox([]byte(`{"sandbox": {"egress": "none", "profile": "process"}}`))
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(ExecutionSandbox(bare))
	b, _ := json.Marshal(ExecutionSandbox(spelled))
	if string(a) != string(b) || string(a) != `{"profile":"process","egress":"none"}` {
		t.Fatalf("%s %s", a, b)
	}
	if jsonDigest(ExecutionSandbox(bare)) != jsonDigest(ExecutionSandbox(spelled)) {
		t.Fatal("the digests differ")
	}
}
