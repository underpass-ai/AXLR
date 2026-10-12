package terminal

import (
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

const codeReply = "Run:\n\n```sh\necho one\n```\n\nthen\n\n```go\nfunc main() {\n    fmt.Println(\"" + "a long line that has to wrap inside the block when the screen is narrow" + "\")\n}\n```\n"

// A code line that wraps continues under its own indentation inside the
// block, not at the transcript's edge.
func TestWrappedCodeLinesKeepTheirIndentation(t *testing.T) {
	tr := NewTranscript()
	tr.SetWidth(40)
	tr.SetSession(domain.SessionState{Messages: []root.Message{{Role: root.RoleAssistant, Content: codeReply}}}, "", Theme{Locale: English, Monochrome: true})
	lines := strings.Split(tr.Viewport.GetContent(), "\n")
	for i, line := range lines {
		if strings.Contains(line, "fmt.Println") {
			next := lines[i+1]
			if !strings.HasPrefix(next, strings.Repeat(" ", 6)) || strings.HasPrefix(next, strings.Repeat(" ", 7)) {
				t.Fatalf("continuation %q after %q", next, line)
			}
			return
		}
	}
	t.Fatalf("no code line:\n%s", tr.Viewport.GetContent())
}

// /copy code copies the last reply's last code block, /copy N its N-th,
// and a number out of range says how many there are.
func TestCopyCodeBlocks(t *testing.T) {
	if blocks := codeBlocks(codeReply); len(blocks) != 2 || blocks[0] != "echo one" || !strings.HasPrefix(blocks[1], "func main() {\n    fmt.Println") {
		t.Fatalf("blocks %q", blocks)
	}
	m := sized()
	defer m.zones.Close()
	m.Header.State.Messages = []root.Message{{Role: root.RoleUser, Content: "how"}, {Role: root.RoleAssistant, Content: codeReply}}
	for _, tc := range []struct{ draft, notice, err string }{
		{"/copy code", "Code block 2 of 2 · Copied 3 lines", ""},
		{"/copiar 1", "Code block 1 of 2 · Copied 1 line", ""},
		{"/copy 3", "", "The last reply has 2 code blocks: /copy 1 to /copy 2, or /copy code for the last"},
		{"/copy everything", "Not sent · usage: /copy [code|N]", ""},
	} {
		m.Composer.Input.SetValue(tc.draft)
		m = update(m, ControlIntent("send"))
		if m.Status.Notice != tc.notice || m.Status.Error != tc.err {
			t.Fatalf("%s: notice %q, error %q", tc.draft, m.Status.Notice, m.Status.Error)
		}
	}
	m.Header.State.Messages = m.Header.State.Messages[:1]
	m.Composer.Input.SetValue("/copy code")
	if m = update(m, ControlIntent("send")); m.Status.Error != "The last reply has no code block" {
		t.Fatalf("no blocks: %q", m.Status.Error)
	}
}
