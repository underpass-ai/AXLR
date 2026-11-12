package storage

import (
	"reflect"
	"testing"
)

func TestCompleteExecSandboxConfinesACleanLinuxInstall(t *testing.T) {
	var s UserSettings
	CompleteExecSandbox(&s, "linux", []string{"/home/you/.cache", "relative", "/tmp/../etc", "/"})
	want := &ExecSandboxSettings{Mode: "auto", Writable: []string{"/home/you/.cache"}}
	if !reflect.DeepEqual(s.Sandbox, want) {
		t.Fatalf("%+v, want %+v", s.Sandbox, want)
	}
	if err := s.Sandbox.validate(); err != nil {
		t.Fatal(err)
	}
}

func TestCompleteExecSandboxLeavesOtherPlatformsAndASetSection(t *testing.T) {
	var other UserSettings
	CompleteExecSandbox(&other, "darwin", []string{"/Users/you/.cache"})
	if other.Sandbox != nil {
		t.Fatalf("darwin got %+v", other.Sandbox)
	}
	set := UserSettings{Sandbox: &ExecSandboxSettings{Mode: "required"}}
	CompleteExecSandbox(&set, "linux", []string{"/home/you/.cache"})
	if set.Sandbox.Mode != "required" || len(set.Sandbox.Writable) != 0 {
		t.Fatalf("set section changed: %+v", set.Sandbox)
	}
}

func TestCompleteExecSandboxBoundsTheWritablePaths(t *testing.T) {
	caches := make([]string, maxSandboxWritable+5)
	for i := range caches {
		caches[i] = "/cache/" + string(rune('a'+i%26)) + string(rune('a'+i/26))
	}
	var s UserSettings
	CompleteExecSandbox(&s, "linux", caches)
	if len(s.Sandbox.Writable) != maxSandboxWritable {
		t.Fatalf("%d writable paths", len(s.Sandbox.Writable))
	}
}
