package runtime

import (
	"encoding/json"

	"github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/dto"
)

type ResponseMapper struct{}

func (ResponseMapper) Map(result any) any {
	switch v := result.(type) {
	case []domain.PluginTool:
		tools := make([]dto.PluginToolOutput, 0, len(v))
		for _, tool := range v {
			item := dto.PluginToolOutput{PluginID: tool.Ref.PluginID.String(), ToolName: tool.Ref.ToolName.String(), Description: tool.Description, InputSchema: tool.InputSchema.Bytes()}
			if tool.OutputSchema != nil {
				item.OutputSchema = tool.OutputSchema.Bytes()
			}
			tools = append(tools, item)
		}
		return dto.PluginListOutput{Tools: tools}
	case domain.PluginResult:
		content := make([]json.RawMessage, 0, len(v.Content))
		for _, block := range v.Content {
			content = append(content, block.Bytes())
		}
		output := dto.PluginCallOutput{Content: content, IsError: v.IsError}
		if v.StructuredContent != nil {
			output.StructuredContent = v.StructuredContent.Bytes()
		}
		return output
	case domain.ReadResult:
		return dto.ReadOutput{Content: v.Content, StartOffsetBytes: int64(v.StartOffset), ReturnedBytes: v.ReturnedBytes, NextOffsetBytes: int64(v.NextOffset), Truncated: v.Truncated, ContentSHA256: string(v.Digest)}
	case domain.WriteResult:
		return dto.WriteOutput{WrittenBytes: v.WrittenBytes, ContentSHA256: string(v.Digest)}
	case domain.ExecResult:
		return dto.ExecOutput{ExitCode: v.ExitCode, Stdout: v.Stdout, Stderr: v.Stderr, CapturedBytes: v.CapturedBytes, DiscardedBytes: v.DiscardedBytes, Truncated: v.Truncated}
	default:
		return nil
	}
}
