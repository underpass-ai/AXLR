package service

import (
	"os"
	"testing"
	"time"
)

func TestCloseWaitsForStartedBackgroundWork(t *testing.T) {
	cfg := validConfig(t.TempDir())
	if err := os.WriteFile(cfg.PrincipalsFile, []byte(`{"version":1,"entries":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := NewServer(cfg, Dependencies{})
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	if !s.startBackground(func() {
		close(started)
		<-s.root.Done()
		<-release
	}) {
		t.Fatal("background work was not started")
	}
	<-started
	closed := make(chan error, 1)
	go func() { closed <- s.Close() }()
	select {
	case err := <-closed:
		t.Fatalf("closed store before worker finished: %v", err)
	case <-s.root.Done():
	}
	select {
	case err := <-closed:
		t.Fatalf("close returned before worker finished: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("close did not wait for worker")
	}
	if s.startBackground(func() { t.Error("started work after shutdown") }) {
		t.Fatal("accepted background work after shutdown")
	}
}
