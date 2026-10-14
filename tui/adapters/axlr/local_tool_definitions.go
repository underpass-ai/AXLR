package axlr

import (
	"fmt"

	root "github.com/underpass-ai/AXLR/domain"
	axlrruntime "github.com/underpass-ai/AXLR/runtime"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func localToolDefinitions() []domain.AvailableTool {
	specs := []struct{ op, description, schema string }{
		{"read", "Read a workspace file by byte offset. A page holds at most max_bytes, or what the model's context keeps per tool result when that is less; while truncated is true, continue with next_offset_bytes.", `{"type":"object","properties":{"path":{"type":"string"},"offset_bytes":{"type":"integer","minimum":0},"max_bytes":{"type":"integer","minimum":0}},"required":["path"],"additionalProperties":false}`},
		{"write", "Create or replace a workspace file. create fails if the file exists; replace requires expected_sha256 = the content_sha256 of a full local_read of the file (offset 0, not truncated), and fails if the file changed since.", `{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"},"mode":{"type":"string","enum":["create","replace"]},"expected_sha256":{"type":"string"}},"required":["path","content","mode"],"additionalProperties":false}`},
		{"edit", "Replace one exact occurrence of old_text in a workspace file.", `{"type":"object","properties":{"path":{"type":"string"},"old_text":{"type":"string","minLength":1},"new_text":{"type":"string"},"expected_sha256":{"type":"string"}},"required":["path","old_text","new_text"],"additionalProperties":false}`},
		// Seen on 10 Oct 2026: claude-haiku-5.5 asked for timeout_ms 600000,
		// the person approved and the runtime refused it. The maxima are the
		// runtime's own limits, and the console checks them before the card.
		//
		// On 14 October 2026 glm-5.3-flash, asked to run "echo hola", sent
		// {"program":"echo","args":["echo","hola"]}, as an argv whose first
		// item is the program, and the person read "$ echo echo hola" on the
		// card: args never holds the program.
		{"exec", fmt.Sprintf("Execute a program with literal arguments in the workspace: program is the executable and args only its arguments, never the program again (echo hola is {\"program\":\"echo\",\"args\":[\"hola\"]}). timeout_ms is at most %d (default 30000).", axlrruntime.HardTimeout.Milliseconds()), fmt.Sprintf(`{"type":"object","properties":{"program":{"type":"string","description":"The executable alone, such as go or echo."},"args":{"type":"array","items":{"type":"string"},"description":"The program's arguments, one item each, without the program itself: echo hola is args [\"hola\"]."},"cwd":{"type":"string","description":"Directory to run in, relative to the workspace root (default: the root)."},"stdin":{"type":"string"},"timeout_ms":{"type":"integer","minimum":0,"maximum":%d},"max_output_bytes":{"type":"integer","minimum":0,"maximum":%d}},"required":["program"],"additionalProperties":false}`, axlrruntime.HardTimeout.Milliseconds(), axlrruntime.HardOutputBytes)},
		// Seen on 9 Oct 2026: claude-haiku-5.5 spent dozens of local_exec
		// grep and sed calls per /improve run and hit the 32-call limit.
		{"search", "Search workspace text files line by line for a Go RE2 pattern (literal: true for plain text); returns path, line, column and text, with context_lines around each. Use it instead of local_exec grep, rg or sed. glob filters files (*.go, docs/*.md, **/*_test.go). Skips .git, binary files and files over 1 MiB; never follows symlinks. While next_offset is present, repeat with offset: next_offset.", `{"type":"object","properties":{"pattern":{"type":"string","minLength":1},"path":{"type":"string"},"glob":{"type":"string"},"literal":{"type":"boolean"},"ignore_case":{"type":"boolean"},"context_lines":{"type":"integer","minimum":0,"maximum":5},"max_results":{"type":"integer","minimum":1,"maximum":200},"offset":{"type":"integer","minimum":0},"max_bytes":{"type":"integer","minimum":0}},"required":["pattern"],"additionalProperties":false}`},
		{"list", "List a workspace directory: path, type and file size of each entry, to max_depth levels when recursive. Use it instead of local_exec ls or find. glob filters names; symlinks are listed, not followed; .git is not entered. While next_offset is present, repeat with offset: next_offset.", `{"type":"object","properties":{"path":{"type":"string"},"glob":{"type":"string"},"recursive":{"type":"boolean"},"max_depth":{"type":"integer","minimum":1,"maximum":8},"max_entries":{"type":"integer","minimum":1,"maximum":500},"offset":{"type":"integer","minimum":0},"max_bytes":{"type":"integer","minimum":0}},"additionalProperties":false}`},
	}
	result := make([]domain.AvailableTool, 0, len(specs))
	for _, s := range specs {
		schema, _ := root.NewJSONObject([]byte(s.schema))
		id, _ := domain.NewLocalToolIdentity(s.op)
		result = append(result, domain.AvailableTool{Identity: id, Definition: root.ToolDefinition{Name: root.ToolName("local_" + s.op), Description: root.Text(s.description), Parameters: schema}})
	}
	return result
}
