package mcpclient

import (
	"context"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type blockingTransport struct{ started chan struct{} }

func (b *blockingTransport) Connect(ctx context.Context) (mcp.Connection, error) {
	close(b.started)
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestCloseDoesNotWaitForStalledConnect(t *testing.T) {
	client := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stalled := &blockingTransport{started: make(chan struct{})}
	finished := make(chan error, 1)
	go func() { finished <- client.connect(ctx, "stalled", stalled) }()
	<-stalled.started
	closed := make(chan error, 1)
	go func() { closed <- client.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("Close blocked behind external handshake")
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("Close did not cancel the pending connection")
	}
}
