package storage

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

// Consoles starting at once agree on one id (review of 14 October 2026:
// the id was written in place, and a racing reader could draw another).
func TestInstallIDIsOneAcrossRacingStarts(t *testing.T) {
	dir := t.TempDir()
	ids := make(chan string, 8)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := InstallID(dir)
			if err != nil {
				t.Error(err)
			}
			ids <- id
		}()
	}
	wg.Wait()
	close(ids)
	first := ""
	for id := range ids {
		if first == "" {
			first = id
		}
		if id != first {
			t.Fatalf("ids differ: %s and %s", first, id)
		}
	}
	// Windows has no owner-only mode bits to check.
	if info, err := os.Stat(filepath.Join(dir, "install-id")); err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o600) {
		t.Fatalf("install-id: %v %v", info, err)
	}
}
