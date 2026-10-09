package plugins

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/domain"
)

type recordedConnection struct {
	id     domain.PluginID
	event  ConnectionEvent
	failed bool
}

type recordingObserver struct {
	mu     sync.Mutex
	events []recordedConnection
	stderr map[domain.PluginID]*lockedBuffer
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (o *recordingObserver) PluginConnection(id domain.PluginID, event ConnectionEvent, _ bool, err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, recordedConnection{id: id, event: event, failed: err != nil})
}

func (o *recordingObserver) PluginStderr(id domain.PluginID) io.Writer {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.stderr == nil {
		o.stderr = map[domain.PluginID]*lockedBuffer{}
	}
	if o.stderr[id] == nil {
		o.stderr[id] = &lockedBuffer{}
	}
	return o.stderr[id]
}

func (o *recordingObserver) recorded() []recordedConnection {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]recordedConnection(nil), o.events...)
}

// The observer hears each connection, its loss and a failed launch by
// plugin ID, and receives the stderr of the servers the manager launches.
func TestManagerReportsConnectionsAndStderrToItsObserver(t *testing.T) {
	missing, err := NewRegistration(Manifest{ID: "missing", Command: "/nonexistent/axlr-plugin", AllowTools: []domain.PluginToolName{"echo"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewManager([]Registration{pluginRegistration(t, "flaky", []domain.PluginToolName{"echo", "die"}, "AXLR_STDERR_NOTE=flaky starting"), missing})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	observer := &recordingObserver{}
	m.SetObserver(observer)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := m.Call(ctx, pluginCall(t, "flaky", "echo", `{"text":"one"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Call(ctx, pluginCall(t, "flaky", "die", `{}`)); err == nil {
		t.Fatal("call that killed the server succeeded")
	}
	if _, err := m.Call(ctx, pluginCall(t, "flaky", "echo", `{"text":"two"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Call(ctx, pluginCall(t, "missing", "echo", `{}`)); err == nil {
		t.Fatal("missing server connected")
	}
	want := []recordedConnection{
		{id: "flaky", event: Connected},
		{id: "flaky", event: ConnectionLost, failed: true},
		{id: "flaky", event: Connected},
		{id: "missing", event: ConnectFailed, failed: true},
	}
	got := observer.recorded()
	if len(got) != len(want) {
		t.Fatalf("events = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("events = %+v, want %+v", got, want)
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(observer.PluginStderr("flaky").(*lockedBuffer).String(), "flaky starting") {
		if time.Now().After(deadline) {
			t.Fatalf("stderr = %q", observer.PluginStderr("flaky").(*lockedBuffer).String())
		}
		time.Sleep(10 * time.Millisecond)
	}
}
