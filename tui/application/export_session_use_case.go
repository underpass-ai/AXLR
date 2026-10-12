package application

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// ExportFilePort creates an export file: owner-only, its directory created
// when missing, and never over an existing file.
type ExportFilePort interface {
	Create(ctx context.Context, path string, data []byte) error
}

// ExportSessionUseCase writes a session as Markdown for the person: its
// title, model and date, each prompt and reply, a row per tool call (tool,
// short arguments, status, size of the result) and the console lines, and
// the cost the ledger knows. /copy copied only the last reply. The ledger
// keeps totals, never a list of requests, so the cost is the session's,
// not each turn's.
type ExportSessionUseCase struct {
	Files ExportFilePort
	// Usage gives the session's cost; nil exports none.
	Usage SessionUsagePort
	// Labels gives the session's title; nil titles it by its first prompt.
	Labels SessionLabelsPort
	Now    func() time.Time
}

// ExportDirectory is where an export without a path goes, under the
// session's workspace.
const ExportDirectory = ".axlr/exports"

// Execute writes state to path, relative to the session's workspace when
// not absolute, or to <workspace>/.axlr/exports/<title>-<yyyymmdd-hhmm>.md
// when empty, and returns the absolute path written.
func (u ExportSessionUseCase) Execute(ctx context.Context, state domain.SessionState, path string) (string, error) {
	if u.Files == nil {
		return "", errors.New("export requires a file port")
	}
	if state.ID == "" || len(state.Messages) == 0 {
		return "", errors.New("the session has nothing to export yet")
	}
	now := time.Now()
	if u.Now != nil {
		now = u.Now()
	}
	title := ""
	if u.Labels != nil {
		if labels, err := u.Labels.Load(ctx); err == nil {
			title = string(labels[state.ID].Title)
		}
	}
	if title == "" {
		if prompts := personPrompts(state.Messages, 1); len(prompts) > 0 {
			title = titlePart(prompts[0])
		}
	}
	if title == "" {
		title = "Session " + string(state.ID)
	}
	var ledger *domain.UsageLedger
	if u.Usage != nil {
		if loaded, err := u.Usage.LoadUsage(ctx, state.ID); err == nil && loaded.Requests > 0 {
			ledger = &loaded
		}
	}
	data := []byte(RenderSessionMarkdown(state, title, ledger, now))
	if path == "" {
		// A second export within the minute gets a suffix rather than a
		// refusal: the person named no file to protect.
		base := filepath.Join(string(state.Workspace), filepath.FromSlash(ExportDirectory), exportSlug(title)+"-"+now.Format("20060102-1504"))
		for n := 1; ; n++ {
			path = base + ".md"
			if n > 1 {
				path = fmt.Sprintf("%s-%d.md", base, n)
			}
			err := u.Files.Create(ctx, path, data)
			if err == nil {
				return path, nil
			}
			if !errors.Is(err, fs.ErrExist) || n == maxExportSuffix {
				return "", err
			}
		}
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(string(state.Workspace), path)
	}
	path = filepath.Clean(path)
	if err := u.Files.Create(ctx, path, data); err != nil {
		return "", err
	}
	return path, nil
}

// exportSlug names an export after the session's title: its first words in
// ASCII, kebab-cased; "session" when it has none.
func exportSlug(title string) string {
	folded := foldToASCII(title)
	if !strings.ContainsFunc(strings.ToLower(folded), func(r rune) bool { return r >= 'a' && r <= 'z' || r >= '0' && r <= '9' }) {
		return "session"
	}
	return slugWords(folded)
}

// maxExportSuffix bounds the default names tried within one minute.
const maxExportSuffix = 99

// maxExportArgumentRunes bounds a tool call's arguments in its row.
const maxExportArgumentRunes = 80

// RenderSessionMarkdown is the Markdown an export writes.
func RenderSessionMarkdown(state domain.SessionState, title string, ledger *domain.UsageLedger, exported time.Time) string {
	var out strings.Builder
	fmt.Fprintf(&out, "# %s\n\n", singleLineText(title))
	fmt.Fprintf(&out, "- Model: `%s`\n- Session: `%s`\n- Workspace: `%s`\n", state.Model, state.ID, state.Workspace)
	if started := firstTime(state.MessageTimes); !started.IsZero() {
		fmt.Fprintf(&out, "- Started: %s\n", started.Local().Format("2006-01-02 15:04"))
	}
	fmt.Fprintf(&out, "- Exported: %s\n", exported.Local().Format("2006-01-02 15:04"))
	if ledger != nil {
		fmt.Fprintf(&out, "- Requests: %d · prompt tokens %d · completion tokens %d\n", ledger.Requests, ledger.PromptTokens, ledger.CompletionTokens)
		switch {
		case ledger.CostRequests == 0:
			out.WriteString("- Cost: unknown\n")
		case ledger.CostRequests < ledger.Requests:
			fmt.Fprintf(&out, "- Cost: %s for %d of %d requests\n", exportUSD(ledger.Cost), ledger.CostRequests, ledger.Requests)
		default:
			fmt.Fprintf(&out, "- Cost: %s\n", exportUSD(ledger.Cost))
		}
		if ledger.Unmetered > 0 {
			fmt.Fprintf(&out, "- Unmetered: %d cancelled or failed requests whose cost is not included\n", ledger.Unmetered)
		}
	}
	activity := map[root.ToolCallID]domain.PendingTool{}
	for _, a := range state.Activity {
		activity[a.Call.ID] = a
	}
	lines, drafts := 0, 0
	flush := func(after int) {
		for drafts < len(state.ArchivedDrafts) && state.ArchivedDrafts[drafts].AfterMessage == after {
			fmt.Fprintf(&out, "\n## Interrupted reply\n\n%s\n", strings.TrimSpace(string(state.ArchivedDrafts[drafts].Content)))
			drafts++
		}
		for lines < len(state.ConsoleLines) && state.ConsoleLines[lines].AfterMessage == after {
			fmt.Fprintf(&out, "\n> %s\n", consoleLineText(state.ConsoleLines[lines]))
			lines++
		}
	}
	flush(0)
	for i, message := range state.Messages {
		at := ""
		if i < len(state.MessageTimes) && !state.MessageTimes[i].IsZero() {
			at = " · " + state.MessageTimes[i].Local().Format("15:04")
		}
		switch message.Role {
		case root.RoleUser:
			if strings.HasPrefix(string(message.Content), "[AXLR") || domain.BudgetNote(message.Content) {
				fmt.Fprintf(&out, "\n> Console%s: %s\n", at, singleLineText(string(message.Content)))
			} else {
				fmt.Fprintf(&out, "\n## You%s\n\n%s\n", at, strings.TrimSpace(string(message.Content)))
			}
		case root.RoleAssistant:
			if text := strings.TrimSpace(string(message.Content)); text != "" || len(message.ToolCalls) == 0 {
				fmt.Fprintf(&out, "\n## Assistant%s\n\n%s\n", at, text)
			}
			if len(message.ToolCalls) > 0 {
				out.WriteString("\n| Tool | Arguments | Status | Size |\n|:--|:--|:--|--:|\n")
				for _, call := range message.ToolCalls {
					status, size := toolCallStatus(activity[call.ID])
					fmt.Fprintf(&out, "| `%s` | %s | %s | %s |\n", call.Name, tableCell(shortArguments(call.Arguments)), status, size)
				}
			}
		}
		flush(i + 1)
	}
	if draft := strings.TrimSpace(string(state.Draft)); draft != "" {
		fmt.Fprintf(&out, "\n## Assistant (unfinished)\n\n%s\n", draft)
	}
	return out.String()
}

func firstTime(times []time.Time) time.Time {
	for _, t := range times {
		if !t.IsZero() {
			return t
		}
	}
	return time.Time{}
}

// exportUSD writes dollars like the console: four decimals below a dollar,
// two from a dollar on.
func exportUSD(v float64) string {
	if v >= 1 {
		return fmt.Sprintf("$%.2f", v)
	}
	text := strings.TrimRight(fmt.Sprintf("%.4f", v), "0")
	if decimals := len(text) - strings.IndexByte(text, '.') - 1; decimals < 2 {
		text += strings.Repeat("0", 2-decimals)
	}
	return "$" + text
}

func toolCallStatus(a domain.PendingTool) (string, string) {
	if a.Outcome == nil {
		return "not run", "—"
	}
	size := fmt.Sprintf("%d B", len(a.Outcome.Content))
	switch {
	case a.Decision == domain.DecisionDeny:
		return "denied", size
	case a.Outcome.Uncertain:
		return "uncertain", size
	case a.Outcome.IsError:
		return "error", size
	}
	return "ok", size
}

func shortArguments(args root.JSONValue) string {
	text := singleLineText(string(args.Bytes()))
	if text == "{}" || text == "" {
		return ""
	}
	if utf8.RuneCountInString(text) > maxExportArgumentRunes {
		text = string([]rune(text)[:maxExportArgumentRunes-1]) + "…"
	}
	return "`" + strings.ReplaceAll(text, "`", "'") + "`"
}

// tableCell keeps a value inside its Markdown table cell.
func tableCell(text string) string {
	return strings.ReplaceAll(text, "|", `\|`)
}

func consoleLineText(line domain.ConsoleLine) string {
	if p := line.Plan; p != nil {
		text := fmt.Sprintf("Plan %s finished %s: %d of %d tasks done", p.PlanID, p.Status, p.Done, p.Tasks)
		if p.CostKnown {
			text += ", " + exportUSD(p.CostUSD)
		}
		return text
	}
	if r := line.Restore; r != nil {
		var files []string
		for _, f := range r.Files {
			files = append(files, fmt.Sprintf("`%s` (to before %s)", f.Path, f.To))
		}
		if r.Undo {
			return "Undid the last turn: restored " + strings.Join(files, ", ")
		}
		return "Restored " + strings.Join(files, ", ")
	}
	if f := line.Failure; f != nil {
		return "Failed: " + f.Message
	}
	return ""
}
