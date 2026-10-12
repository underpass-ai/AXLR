package storage

import (
	"os"
	"path/filepath"
	"testing"
)

// A launch holds its run token for its lifetime; another launch sees it
// alive until it closes, and a lock file nobody holds is a stopped launch's,
// removed when seen.
func TestRunLockTellsWhetherALaunchStillRuns(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "runs")
	stale := filepath.Join(dir, "00000000deadbeef.lock")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, nil, 0600); err != nil {
		t.Fatal(err)
	}
	first, err := HoldRun(dir, "1111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("the stale lock file was kept: %v", err)
	}
	second, err := HoldRun(dir, "2222222222222222")
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if !second.Alive("1111111111111111") || !first.Alive("2222222222222222") || !first.Alive("1111111111111111") {
		t.Fatal("an open launch is not alive")
	}
	if second.Alive("3333333333333333") || second.Alive("") || second.Alive("../x") {
		t.Fatal("a launch without a lock is alive")
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if second.Alive("1111111111111111") {
		t.Fatal("a closed launch is alive")
	}
	if _, err := os.Stat(filepath.Join(dir, "1111111111111111.lock")); !os.IsNotExist(err) {
		t.Fatalf("the closed launch's file was kept: %v", err)
	}
	if _, err := HoldRun("relative", "1111111111111111"); err == nil {
		t.Fatal("a relative directory was accepted")
	}
	if _, err := HoldRun(dir, "not-hex"); err == nil {
		t.Fatal("an invalid token was accepted")
	}
}
