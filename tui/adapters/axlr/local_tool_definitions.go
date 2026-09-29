package axlr

import (
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func localToolDefinitions() []domain.AvailableTool {
	specs := []struct{ op, description, schema string }{
		{"read", "Read a workspace file by byte offset.", `{"type":"object","properties":{"path":{"type":"string"},"offset_bytes":{"type":"integer","minimum":0},"max_bytes":{"type":"integer","minimum":0}},"required":["path"],"additionalProperties":false}`},
		{"write", "Create or replace a workspace file; optional digest guards against changes.", `{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"},"mode":{"type":"string","enum":["create","replace"]},"expected_sha256":{"type":"string"}},"required":["path","content","mode"],"additionalProperties":false}`},
		{"edit", "Replace one exact occurrence of old_text in a workspace file.", `{"type":"object","properties":{"path":{"type":"string"},"old_text":{"type":"string","minLength":1},"new_text":{"type":"string"},"expected_sha256":{"type":"string"}},"required":["path","old_text","new_text"],"additionalProperties":false}`},
		{"exec", "Execute a program with literal arguments in the workspace.", `{"type":"object","properties":{"program":{"type":"string"},"args":{"type":"array","items":{"type":"string"}},"cwd":{"type":"string"},"stdin":{"type":"string"},"timeout_ms":{"type":"integer","minimum":0},"max_output_bytes":{"type":"integer","minimum":0}},"required":["program"],"additionalProperties":false}`},
	}
	result := make([]domain.AvailableTool, 0, len(specs))
	for _, s := range specs {
		schema, _ := root.NewJSONObject([]byte(s.schema))
		id, _ := domain.NewLocalToolIdentity(s.op)
		result = append(result, domain.AvailableTool{Identity: id, Definition: root.ToolDefinition{Name: root.ToolName("local_" + s.op), Description: root.Text(s.description), Parameters: schema}})
	}
	return result
}
