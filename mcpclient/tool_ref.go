package mcpclient

// ToolRef identifies a tool within its MCP server.
type ToolRef struct {
	Server ServerName `json:"server"`
	Name   ToolName   `json:"name"`
}

func (r ToolRef) Validate() error {
	if _, err := NewServerName(r.Server.String()); err != nil {
		return err
	}
	_, err := NewToolName(r.Name.String())
	return err
}
