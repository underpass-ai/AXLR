package application

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/dto"
	"github.com/underpass-ai/AXLR/runtime"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// localReadRuntime runs local calls on the AXLR runtime the way the
// console's ToolRunner does, and keeps the arguments of every read.
type localReadRuntime struct {
	executor  *runtime.Executor
	workspace string
	reads     []string
}

func (r *localReadRuntime) Execute(ctx context.Context, id domain.ToolIdentity, args root.JSONValue) (domain.ToolOutcome, error) {
	r.reads = append(r.reads, string(args.Bytes()))
	response := r.executor.Execute(ctx, dto.Request{ProtocolVersion: runtime.ProtocolVersion, RequestID: "tui-tool", Tool: id.LocalOperation, Arguments: args.Bytes()})
	content, err := json.Marshal(response)
	if err != nil {
		return domain.ToolOutcome{}, err
	}
	return domain.ToolOutcome{Content: root.Text(content), IsError: response.Status != "completed"}, nil
}

// localReadWorkspace holds docs/console.md: about 30 KB of Markdown with the
// newlines, quotes, tabs, angle brackets and accents that JSON escapes.
func localReadWorkspace(t *testing.T) (*localReadRuntime, string) {
	t.Helper()
	workspace := t.TempDir()
	var text strings.Builder
	for i := 0; text.Len() < 30616; i++ {
		fmt.Fprintf(&text, "## Sección %d\n\nThe console's \"local_read\" pages a file <by byte offset> & continues\tat `next_offset_bytes` — línea %d.\n\n", i, i)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "docs", "console.md"), []byte(text.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	executor, err := runtime.New(runtime.Config{Root: workspace, Env: []string{"PATH=" + os.Getenv("PATH")}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { executor.Close() })
	return &localReadRuntime{executor: executor, workspace: workspace}, text.String()
}

type localReadResult struct {
	Output struct {
		Content          string `json:"content"`
		StartOffsetBytes int64  `json:"start_offset_bytes"`
		NextOffsetBytes  int64  `json:"next_offset_bytes"`
		Truncated        bool   `json:"truncated"`
	} `json:"output"`
}

func decodeLocalRead(t *testing.T, content root.Text) localReadResult {
	t.Helper()
	var result localReadResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		t.Fatalf("local_read result: %v", err)
	}
	return result
}

var localReadIdentity = domain.ToolIdentity{Kind: domain.ToolKindLocal, LocalOperation: "read"}

// projectedLocalRead is what the projection sends for a local_read result
// of the turn in progress.
func projectedLocalRead(t *testing.T, budget domain.ContextBudget, content root.Text) string {
	t.Helper()
	projector, err := NewModelContextProjector(budget)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := projector.Project([]root.Message{
		{Role: root.RoleUser, Content: "lee"},
		{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{{ID: "call-1", Name: "local_read", Arguments: hostJSON(t, `{"path":"docs/console.md"}`)}}},
		{Role: root.RoleTool, ToolCallID: "call-1", Content: content},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(projection.Messages[2].Content)
}

// Measured with z-ai/glm-5.3-flash under the 64,000-token
// prompt budget: local_read of a 30 KB file without max_bytes came back as an
// excerpt whose next_offset_bytes was the end of the file. Read through the
// resolver, the first page now reaches the model whole, and its
// next_offset_bytes is where the file continues.
func TestLocalReadWithoutMaxBytesFitsThePromptBudget(t *testing.T) {
	tools, text := localReadWorkspace(t)
	schema := hostJSON(t, `{"type":"object"}`)
	s := turnSession(t)
	if err := s.BeginTurn("lee docs/console.md", []domain.AvailableTool{{Identity: localReadIdentity, Definition: root.ToolDefinition{Name: "local_read", Parameters: schema}}}); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteAssistant(assistant("", root.ToolCall{ID: "call-1", Name: "local_read", Arguments: hostJSON(t, `{"path":"docs/console.md"}`)})); err != nil {
		t.Fatal(err)
	}
	var sent root.CompletionRequest
	store := &memoryStore{}
	u := ResolveToolUseCase{Store: store, Tools: tools, Continue: ContinueTurnUseCase{Store: store, Models: streamFunc(func(_ context.Context, req root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		sent = req
		return assistant("leído"), nil
	})}}
	if err := u.Execute(context.Background(), &s, "call-1", domain.DecisionApprove, nil); err != nil {
		t.Fatal(err)
	}
	budget := domain.ContextBudgetForPrompt(domain.DefaultPromptTokens)
	if budget.ToolResultBytes() != 17344 {
		t.Fatalf("default prompt budget keeps %d bytes per tool result", budget.ToolResultBytes())
	}
	result := sent.Messages[len(sent.Messages)-1]
	if result.Role != root.RoleTool || strings.Contains(string(result.Content), "axlr_tool_result_excerpt") {
		t.Fatalf("first page was excerpted: %.300s", result.Content)
	}
	var page struct {
		Content         string `json:"content"`
		NextOffsetBytes int64  `json:"next_offset_bytes"`
		Truncated       bool   `json:"truncated"`
	}
	if err := json.Unmarshal([]byte(result.Content), &page); err != nil {
		t.Fatal(err)
	}
	if !page.Truncated || page.NextOffsetBytes <= 8192 || page.NextOffsetBytes >= int64(len(text)) || page.Content != text[:page.NextOffsetBytes] {
		t.Fatalf("page of %d bytes ends at %d of %d (truncated=%v)", len(page.Content), page.NextOffsetBytes, len(text), page.Truncated)
	}
	if len(tools.reads) > 2 {
		t.Fatalf("%d reads for one page", len(tools.reads))
	}
	// Following next_offset_bytes reads the rest, each page whole.
	var read strings.Builder
	read.WriteString(page.Content)
	for offset := page.NextOffsetBytes; offset < int64(len(text)); {
		outcome, err := boundedLocalRead(context.Background(), tools, localReadIdentity, hostJSON(t, fmt.Sprintf(`{"path":"docs/console.md","offset_bytes":%d}`, offset)), budget.ToolResultBytes())
		if err != nil || outcome.IsError {
			t.Fatalf("page at %d: err=%v outcome=%.300s", offset, err, outcome.Content)
		}
		if projected := projectedLocalRead(t, budget, outcome.Content); strings.Contains(projected, "axlr_tool_result_excerpt") {
			t.Fatalf("page at %d was excerpted", offset)
		}
		next := decodeLocalRead(t, outcome.Content).Output
		if next.StartOffsetBytes != offset || next.NextOffsetBytes <= offset {
			t.Fatalf("page at %d starts at %d and continues at %d", offset, next.StartOffsetBytes, next.NextOffsetBytes)
		}
		read.WriteString(next.Content)
		offset = next.NextOffsetBytes
	}
	if read.String() != text {
		t.Fatalf("pages hold %d bytes of a %d-byte file", read.Len(), len(text))
	}
}

// A max_bytes larger than the page the budget keeps is clamped to it; a
// smaller one is passed unchanged.
func TestLocalReadClampsALargerMaxBytes(t *testing.T) {
	tools, _ := localReadWorkspace(t)
	limit := domain.ContextBudgetForPrompt(domain.DefaultPromptTokens).ToolResultBytes()
	outcome, err := boundedLocalRead(context.Background(), tools, localReadIdentity, hostJSON(t, `{"path":"docs/console.md","max_bytes":100000}`), limit)
	if err != nil || outcome.IsError {
		t.Fatalf("err=%v outcome=%.300s", err, outcome.Content)
	}
	var first struct {
		MaxBytes int `json:"max_bytes"`
	}
	if err := json.Unmarshal([]byte(tools.reads[0]), &first); err != nil {
		t.Fatal(err)
	}
	if first.MaxBytes != localReadPageBytes(limit) || !toolResultFits(string(outcome.Content), limit) {
		t.Fatalf("max_bytes 100000 read %s under a limit of %d", tools.reads[0], limit)
	}
	tools.reads = nil
	small := `{"path":"docs/console.md","max_bytes":4096}`
	outcome, err = boundedLocalRead(context.Background(), tools, localReadIdentity, hostJSON(t, small), limit)
	if err != nil || outcome.IsError || len(tools.reads) != 1 || tools.reads[0] != small {
		t.Fatalf("max_bytes 4096: err=%v reads=%q", err, tools.reads)
	}
	if page := decodeLocalRead(t, outcome.Content).Output; page.NextOffsetBytes != 4096 {
		t.Fatalf("max_bytes 4096 continues at %d", page.NextOffsetBytes)
	}
}

// Text that JSON escapes many times over overflows a page sized for plain
// text; it is read again, shorter, and still continues where it stops.
func TestLocalReadShortensAPageItsEscapingOverflows(t *testing.T) {
	tools, _ := localReadWorkspace(t)
	text := strings.Repeat("<\"\\>&\n", 4000)
	if err := os.WriteFile(filepath.Join(tools.workspace, "escaped.txt"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	limit := domain.ContextBudgetForPrompt(domain.DefaultPromptTokens).ToolResultBytes()
	outcome, err := boundedLocalRead(context.Background(), tools, localReadIdentity, hostJSON(t, `{"path":"escaped.txt"}`), limit)
	if err != nil || outcome.IsError {
		t.Fatalf("err=%v outcome=%.300s", err, outcome.Content)
	}
	page := decodeLocalRead(t, outcome.Content).Output
	if len(tools.reads) < 2 || !toolResultFits(string(outcome.Content), limit) || page.Content != text[:page.NextOffsetBytes] || !page.Truncated {
		t.Fatalf("%d reads, page of %d bytes continuing at %d", len(tools.reads), len(page.Content), page.NextOffsetBytes)
	}
}

// A compact ceremony step keeps 8 KiB per tool result, and its pages shrink
// to that.
func TestLocalReadPagesWithinTheCompactBudget(t *testing.T) {
	s := turnSession(t)
	if err := s.BeginTurn("lee", turnTools()); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCeremony(domain.CeremonyRun{Definition: "axlr_delivery", Version: "2.0", Instance: "i", Step: "build", Iteration: 1, Compact: true}); err != nil {
		t.Fatal(err)
	}
	limit := projectionBudget(windowsFunc(func(root.ModelID) domain.ContextWindow { return 0 }), s).ToolResultBytes()
	if limit != domain.CompactContextBudget().ToolResultBytes() {
		t.Fatalf("compact step keeps %d bytes per tool result", limit)
	}
	tools, _ := localReadWorkspace(t)
	outcome, err := boundedLocalRead(context.Background(), tools, localReadIdentity, hostJSON(t, `{"path":"docs/console.md"}`), limit)
	if err != nil || outcome.IsError || !toolResultFits(string(outcome.Content), limit) {
		t.Fatalf("err=%v fits=%v", err, toolResultFits(string(outcome.Content), limit))
	}
}

// A local_read result excerpted all the same (saved under a larger budget,
// say) tells the model to read the file on from the first byte the excerpt
// omits, in pages that fit, not to narrow a query local_read does not have;
// and reads the same once its turn has closed.
func TestExcerptedLocalReadNamesWhereToReadOn(t *testing.T) {
	tools, text := localReadWorkspace(t)
	whole, err := tools.Execute(context.Background(), localReadIdentity, hostJSON(t, `{"path":"docs/console.md","max_bytes":65536}`))
	if err != nil || whole.IsError {
		t.Fatalf("err=%v outcome=%.300s", err, whole.Content)
	}
	budget := domain.ContextBudgetForPrompt(domain.DefaultPromptTokens)
	open := []root.Message{
		{Role: root.RoleUser, Content: "lee"},
		{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{{ID: "call-1", Name: "local_read", Arguments: hostJSON(t, `{"path":"docs/console.md"}`)}}},
		{Role: root.RoleTool, ToolCallID: "call-1", Content: whole.Content},
	}
	projector, _ := NewModelContextProjector(budget)
	during, err := projector.Project(open)
	if err != nil {
		t.Fatal(err)
	}
	closed := append(append([]root.Message(nil), open...), root.Message{Role: root.RoleAssistant, Content: "hecho"}, root.Message{Role: root.RoleUser, Content: "sigue"})
	after, err := projector.Project(closed)
	if err != nil {
		t.Fatal(err)
	}
	if during.Messages[2].Content != after.Messages[2].Content {
		t.Fatalf("excerpt changed when the turn closed:\n%s\n%s", during.Messages[2].Content, after.Messages[2].Content)
	}
	var excerpt struct {
		Kind         string `json:"kind"`
		Retrieval    string `json:"retrieval"`
		ExcerptStart string `json:"excerpt_start"`
	}
	if err := json.Unmarshal([]byte(during.Messages[2].Content), &excerpt); err != nil {
		t.Fatal(err)
	}
	if excerpt.Kind != "axlr_tool_result_excerpt" || strings.Contains(excerpt.Retrieval, "query") || !strings.Contains(excerpt.Retrieval, "call local_read again") {
		t.Fatalf("retrieval: %s", excerpt.Retrieval)
	}
	numbers := regexp.MustCompile(`offset_bytes: (\d+) and max_bytes of at most (\d+)`).FindStringSubmatch(excerpt.Retrieval)
	if numbers == nil {
		t.Fatalf("retrieval names no offset: %s", excerpt.Retrieval)
	}
	offset, _ := strconv.Atoi(numbers[1])
	page, _ := strconv.Atoi(numbers[2])
	// The offset is the first byte excerpt_start does not show.
	shown, _ := json.Marshal(text[:offset])
	if offset == 0 || offset >= len(text) || page != localReadPageBytes(budget.ToolResultBytes()) || !strings.HasPrefix(excerpt.ExcerptStart, `{"content":`+string(shown[:len(shown)-1])) {
		t.Fatalf("offset %d, max_bytes %d, excerpt_start %d bytes", offset, page, len(excerpt.ExcerptStart))
	}
	_, size := utf8.DecodeRuneInString(text[offset:])
	beyond, _ := json.Marshal(text[:offset+size])
	if strings.HasPrefix(excerpt.ExcerptStart, `{"content":`+string(beyond[:len(beyond)-1])) {
		t.Fatalf("excerpt_start shows byte %d as well", offset)
	}
	// Following it continues the file whole.
	outcome, err := boundedLocalRead(context.Background(), tools, localReadIdentity, hostJSON(t, fmt.Sprintf(`{"path":"docs/console.md","offset_bytes":%d,"max_bytes":%d}`, offset, page)), budget.ToolResultBytes())
	if err != nil || outcome.IsError || strings.Contains(projectedLocalRead(t, budget, outcome.Content), "axlr_tool_result_excerpt") {
		t.Fatalf("read on from %d: err=%v", offset, err)
	}
	if next := decodeLocalRead(t, outcome.Content).Output; next.StartOffsetBytes != int64(offset) || next.Content != text[offset:next.NextOffsetBytes] {
		t.Fatalf("read on from %d returned %d bytes from %d", offset, len(next.Content), next.StartOffsetBytes)
	}
}
