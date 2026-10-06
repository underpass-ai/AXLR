package service

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/underpass-ai/AXLR/buildinfo"
)

func (s *Server) mcpHandler() http.Handler {
	// Stateless HTTP sessions bind each call to the authenticated HTTP request.
	// A session ID can never transfer a certificate's authority to another client.
	return mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		server := mcp.NewServer(&mcp.Implementation{Name: "axlr", Version: buildinfo.Version}, nil)
		for _, op := range operations() {
			server.AddTool(&mcp.Tool{Name: op.Tool, Description: op.Description, InputSchema: op.schema()}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				var emit func(Event) error
				if token := req.Params.GetProgressToken(); token != nil && op.Name == "StreamEvents" {
					emit = func(event Event) error {
						data, _ := json.Marshal(event)
						return req.Session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{ProgressToken: token, Progress: float64(event.Sequence), Message: string(data)})
					}
				}
				result := s.invokeOperation(ctx, op, req.Params.Arguments, r.TLS, emit)
				encoded, _ := json.Marshal(result)
				return &mcp.CallToolResult{IsError: result.StatusCode >= 400, StructuredContent: json.RawMessage(encoded), Content: []mcp.Content{&mcp.TextContent{Text: string(encoded)}}}, nil
			})
		}
		return server
	}, &mcp.StreamableHTTPOptions{Stateless: true})
}
