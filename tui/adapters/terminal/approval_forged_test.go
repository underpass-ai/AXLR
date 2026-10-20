package terminal

import (
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// The forge card writes the command as a shell would read it. On 16 October
// 2026 it printed `$ python3 -c import os; print(os.environ)`, one argument
// that read as two shell commands.
func TestForgeCardQuotesTheCommandLikeAShell(t *testing.T) {
	card, ok := forgeCard([]byte(`{"name":"probe","description":"qa","input_schema":{"type":"object"},"program":"python3","args":["-c","import os; print(os.environ)",".axlr/tools/probe/main.py"],"files":[{"path":"main.py","content":"print(1)\n"}]}`))
	if !ok {
		t.Fatal("not a forge card")
	}
	if want := `$ python3 -c "import os; print(os.environ)" .axlr/tools/probe/main.py`; !strings.HasPrefix(card, want+"\n") {
		t.Fatalf("card starts %q, want %q", strings.SplitN(card, "\n", 2)[0], want)
	}
}

// A forge is a write: its card shows the model's why first (20 October 2026).
func TestForgeCardShowsTheWhyFirst(t *testing.T) {
	arguments, _ := root.NewJSONObject([]byte(`{"name":"probe","description":"qa","input_schema":{"type":"object"},"program":"python3","args":[".axlr/tools/probe/main.py"],"files":[{"path":"main.py","content":"print(1)\n"}],"why":"Crear la herramienta que pidió la persona."}`))
	d := NewApprovalDialog(domain.PendingTool{Call: root.ToolCall{ID: "f1", Name: application.HostForgeToolName, Arguments: arguments}}, "probe", Theme{})
	if text := d.Details.Text(); !strings.HasPrefix(strings.TrimSpace(text), "why: Crear la herramienta que pidió la persona.") || !strings.Contains(text, "$ python3") {
		t.Fatalf("card:\n%s", text)
	}
}
