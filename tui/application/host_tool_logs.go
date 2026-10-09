package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// HostLogsName reads the console's own log.
const HostLogsName root.ToolName = "axlr_logs"

// AppLogQuery selects the last entries of the console's log: at most Lines
// of them, at Level or above (INFO, WARN or ERROR), containing Contains
// when set, from this console only unless AllConsoles.
type AppLogQuery struct {
	Lines       int
	Level       string
	Contains    string
	AllConsoles bool
}

// AppLogPage is what axlr_logs returns: the log's path, this console's
// process ID, the entries, oldest first, and how many earlier entries
// matched too.
type AppLogPage struct {
	Path    string   `json:"path"`
	Console int      `json:"console_pid"`
	Entries []string `json:"entries"`
	Omitted int      `json:"omitted,omitempty"`
}

// AppLogPort reads the console's log.
type AppLogPort interface {
	TailLog(context.Context, AppLogQuery) (AppLogPage, error)
}

// LogsTool is axlr_logs, offered when the console keeps an app log.
func LogsTool() domain.AvailableTool {
	identity, _ := domain.NewHostToolIdentity(domain.HostOperationLogs)
	schema, _ := root.NewJSONObject([]byte(`{"type":"object","properties":{"lines":{"type":"integer","minimum":1,"maximum":200},"level":{"type":"string","enum":["INFO","WARN","ERROR"]},"contains":{"type":"string","minLength":1,"maxLength":200},"all_consoles":{"type":"boolean"}},"additionalProperties":false}`))
	return domain.AvailableTool{Identity: identity, Definition: root.ToolDefinition{
		Name:        HostLogsName,
		Description: "Read the last entries of this console's own log (outside the workspace): startup warnings, MCP plugin connections and failures by plugin ID, the plugins' stderr, failed tool calls. Use it when an axlr_*, local_* or plugin call fails without a clear reason, before telling the user or requesting a repair. lines defaults to 50; level filters INFO, WARN or ERROR and above; contains filters by text; all_consoles includes other consoles of this account. Read-only.",
		Parameters:  schema,
	}}
}

// maxLogEntryBytes bounds one entry in a page; the log caps them already.
const maxLogEntryBytes = 2048

func hostLogs(ctx context.Context, port AppLogPort, arguments root.JSONValue) (any, error) {
	if port == nil {
		return nil, errors.New("this console keeps no app log")
	}
	var args struct {
		Lines       *int    `json:"lines"`
		Level       *string `json:"level"`
		Contains    *string `json:"contains"`
		AllConsoles *bool   `json:"all_consoles"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(arguments.Bytes())))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&args); err != nil {
		return nil, errors.New("axlr_logs takes lines, level, contains and all_consoles")
	}
	query := AppLogQuery{Lines: 50, Level: "INFO"}
	if args.Lines != nil {
		if *args.Lines < 1 || *args.Lines > 200 {
			return nil, errors.New("lines must be between 1 and 200")
		}
		query.Lines = *args.Lines
	}
	if args.Level != nil {
		switch *args.Level {
		case "INFO", "WARN", "ERROR":
			query.Level = *args.Level
		default:
			return nil, errors.New("level must be INFO, WARN or ERROR")
		}
	}
	if args.Contains != nil {
		query.Contains = *args.Contains
	}
	if args.AllConsoles != nil {
		query.AllConsoles = *args.AllConsoles
	}
	page, err := port.TailLog(ctx, query)
	if err != nil {
		return nil, err
	}
	// Keep the newest entries that fit the host result bound.
	budget := MaxHostResultBytes - 1024 - len(page.Path)
	kept := len(page.Entries)
	for i := len(page.Entries) - 1; i >= 0; i-- {
		entry := page.Entries[i]
		if len(entry) > maxLogEntryBytes {
			entry = strings.ToValidUTF8(entry[:maxLogEntryBytes], "")
			page.Entries[i] = entry
		}
		encoded, _ := json.Marshal(entry)
		if budget -= len(encoded) + 1; budget < 0 {
			break
		}
		kept = i
	}
	page.Omitted += kept
	page.Entries = page.Entries[kept:]
	if page.Entries == nil {
		page.Entries = []string{}
	}
	return page, nil
}
