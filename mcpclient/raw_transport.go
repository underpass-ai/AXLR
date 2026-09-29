package mcpclient

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type rawTransport struct {
	base     mcp.Transport
	observer *rawCallObserver
}

func (t rawTransport) Connect(ctx context.Context) (mcp.Connection, error) {
	connection, err := t.base.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return rawConnection{Connection: connection, observer: t.observer}, nil
}
