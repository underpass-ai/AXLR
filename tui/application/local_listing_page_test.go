package application

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/runtime"
	"github.com/underpass-ai/AXLR/tui/domain"
)

var (
	localSearchIdentity = domain.ToolIdentity{Kind: domain.ToolKindLocal, LocalOperation: "search"}
	localListIdentity   = domain.ToolIdentity{Kind: domain.ToolKindLocal, LocalOperation: "list"}
)

// listingWorkspace holds a source file of 400 matching lines with the
// quotes, angle brackets and ampersands that JSON escapes, and a directory
// of 600 notes.
func listingWorkspace(t *testing.T) *localReadRuntime {
	t.Helper()
	workspace := t.TempDir()
	write := func(name, content string) {
		path := filepath.Join(workspace, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var text strings.Builder
	for i := 1; i <= 400; i++ {
		fmt.Fprintf(&text, "\tif budget.ToolResultBytes() < limit && name == \"local_search\" { // <línea %d> & más\n", i)
	}
	write("tui/application/budget.go", text.String())
	for i := 0; i < 600; i++ {
		write(fmt.Sprintf("docs/notes/note-%03d-on-the-context-budget.md", i), "x")
	}
	executor, err := runtime.New(runtime.Config{Root: workspace, Env: []string{"PATH=" + os.Getenv("PATH")}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { executor.Close() })
	return &localReadRuntime{executor: executor, workspace: workspace}
}

// listingPage is a search or list page as the projection shows it: the
// runtime's output with its status beside it.
type listingPage struct {
	Status  string `json:"status"`
	Matches []struct {
		Line int `json:"line"`
	} `json:"matches"`
	Entries []struct {
		Path string `json:"path"`
	} `json:"entries"`
	NextOffset int  `json:"next_offset"`
	Truncated  bool `json:"truncated"`
}

func decodeListingPage(t *testing.T, content string) listingPage {
	t.Helper()
	_, _, whole := wholeToolContent(content)
	var page listingPage
	if err := json.Unmarshal([]byte(whole), &page); err != nil || page.Status != "completed" {
		t.Fatalf("listing page %.300s: %v", whole, err)
	}
	return page
}

// Seen on 9 Oct 2026: /improve runs with claude-haiku-5.5 searched through
// dozens of local_exec grep and sed calls. A local_search answers in one
// call, and through the resolver its page reaches the model whole under the
// default prompt budget, with a next_offset where the matches continue.
func TestLocalSearchFitsThePromptBudget(t *testing.T) {
	tools := listingWorkspace(t)
	schema := hostJSON(t, `{"type":"object"}`)
	s := turnSession(t)
	if err := s.BeginTurn("where is the budget used", []domain.AvailableTool{{Identity: localSearchIdentity, Definition: root.ToolDefinition{Name: "local_search", Parameters: schema}}}); err != nil {
		t.Fatal(err)
	}
	call := `{"pattern":"budget","context_lines":2,"max_results":200}`
	if err := s.CompleteAssistant(assistant("", root.ToolCall{ID: "call-1", Name: "local_search", Arguments: hostJSON(t, call)})); err != nil {
		t.Fatal(err)
	}
	var sent root.CompletionRequest
	store := &memoryStore{}
	u := ResolveToolUseCase{Store: store, Tools: tools, Continue: ContinueTurnUseCase{Store: store, Models: streamFunc(func(_ context.Context, req root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		sent = req
		return assistant("found"), nil
	})}}
	if err := u.Execute(context.Background(), &s, "call-1", domain.DecisionApprove, nil); err != nil {
		t.Fatal(err)
	}
	result := sent.Messages[len(sent.Messages)-1]
	if result.Role != root.RoleTool || strings.Contains(string(result.Content), "axlr_tool_result_excerpt") {
		t.Fatalf("search page was excerpted: %.300s", result.Content)
	}
	page := decodeListingPage(t, string(result.Content))
	if len(page.Matches) < 10 || page.NextOffset != len(page.Matches) || !page.Truncated {
		t.Fatalf("first page: %d matches, next_offset %d", len(page.Matches), page.NextOffset)
	}
	if len(tools.reads) > 2 {
		t.Fatalf("%d searches for one page", len(tools.reads))
	}
	// Following next_offset reaches every match, each page whole.
	limit := domain.ContextBudgetForPrompt(domain.DefaultPromptTokens).ToolResultBytes()
	line := len(page.Matches)
	for offset := page.NextOffset; offset != 0; {
		outcome, err := boundedLocalListing(context.Background(), tools, localSearchIdentity, hostJSON(t, fmt.Sprintf(`{"pattern":"budget","context_lines":2,"max_results":200,"offset":%d}`, offset)), limit)
		if err != nil || outcome.IsError || !toolResultFits(string(outcome.Content), limit) {
			t.Fatalf("page at %d: err=%v fits=%v", offset, err, toolResultFits(string(outcome.Content), limit))
		}
		next := decodeListingPage(t, string(outcome.Content))
		if next.Matches[0].Line != line+1 {
			t.Fatalf("page at %d starts at line %d after line %d", offset, next.Matches[0].Line, line)
		}
		line += len(next.Matches)
		offset = next.NextOffset
	}
	if line != 400 {
		t.Fatalf("pages reached %d of 400 lines", line)
	}
}

// A listing is bounded the same way, and a max_bytes larger than the page
// the budget keeps is clamped to it, while a smaller one passes unchanged.
func TestLocalListPagesFitTheBudgetAndClampMaxBytes(t *testing.T) {
	tools := listingWorkspace(t)
	limit := domain.ContextBudgetForPrompt(domain.DefaultPromptTokens).ToolResultBytes()
	listed := 0
	for offset := 0; ; {
		outcome, err := boundedLocalListing(context.Background(), tools, localListIdentity, hostJSON(t, fmt.Sprintf(`{"path":"docs/notes","max_entries":500,"max_bytes":100000,"offset":%d}`, offset)), limit)
		if err != nil || outcome.IsError || !toolResultFits(string(outcome.Content), limit) {
			t.Fatalf("page at %d: err=%v outcome=%.300s", offset, err, outcome.Content)
		}
		page := decodeListingPage(t, string(outcome.Content))
		if want := fmt.Sprintf("docs/notes/note-%03d-on-the-context-budget.md", listed); page.Entries[0].Path != want {
			t.Fatalf("page at %d starts with %s", offset, page.Entries[0].Path)
		}
		listed += len(page.Entries)
		if page.NextOffset == 0 {
			break
		}
		offset = page.NextOffset
	}
	if listed != 600 {
		t.Fatalf("pages listed %d of 600 notes", listed)
	}
	var first struct {
		MaxBytes int `json:"max_bytes"`
	}
	if err := json.Unmarshal([]byte(tools.reads[0]), &first); err != nil || first.MaxBytes != localReadPageBytes(limit) {
		t.Fatalf("max_bytes 100000 listed with %s under a limit of %d", tools.reads[0], limit)
	}
	tools.reads = nil
	small := `{"path":"docs/notes","max_bytes":2000}`
	outcome, err := boundedLocalListing(context.Background(), tools, localListIdentity, hostJSON(t, small), limit)
	if err != nil || outcome.IsError || len(tools.reads) != 1 || tools.reads[0] != small {
		t.Fatalf("max_bytes 2000: err=%v reads=%q", err, tools.reads)
	}
	if encoded := len(decodeListingPage(t, string(outcome.Content)).Entries); encoded == 0 || encoded > 40 {
		t.Fatalf("max_bytes 2000 listed %d entries", encoded)
	}
}
