package mcpclient

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type rawConnection struct {
	mcp.Connection
	observer *rawCallObserver
}

func (c rawConnection) Read(ctx context.Context) (jsonrpc.Message, error) {
	message, err := c.Connection.Read(ctx)
	if err == nil {
		if response, ok := message.(*jsonrpc.Response); ok {
			c.observer.received(response)
		}
	}
	return message, err
}

func (c rawConnection) Write(ctx context.Context, message jsonrpc.Message) error {
	if request, ok := message.(*jsonrpc.Request); ok && request.Method == "tools/call" {
		c.observer.sent(request.ID)
	}
	return c.Connection.Write(ctx, message)
}
