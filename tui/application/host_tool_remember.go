package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// HostRememberName records one memory in KMP in one call; see RememberTool.
const HostRememberName root.ToolName = "axlr_remember"

// Recording one memory and one relation took 18 model requests on 8
// October 2026: kmp_guide, kmp_inspect five times (23.6 KB of results), and
// kmp_write_memory four times, each answered needs_review, besides 7.4 KB of
// schema reads kept in the history. axlr_remember takes what the model
// knows and does the rest in the console.
const rememberSchema = `{"type":"object","properties":{` +
	`"kind":{"type":"string","enum":["decision","constraint","observation","feedback","preference","error_path","success_path","semantic_delta","derived_value"]},` +
	`"text":{"type":"string","minLength":1,"maxLength":4000},` +
	`"evidence":{"type":"array","minItems":1,"maxItems":8,"items":{"type":"string","minLength":1,"maxLength":1000}},` +
	`"links":{"type":"array","maxItems":8,"items":{"type":"object","properties":{"ref":{"type":"string","minLength":1},"rel":{"type":"string","minLength":1},"why":{"type":"string","minLength":1,"maxLength":1000},"evidence":{"type":"string","maxLength":1000}},"required":["ref","rel","why"],"additionalProperties":false}},` +
	`"about":{"type":"string","minLength":1,"maxLength":256},` +
	`"continuation":{"type":"string","minLength":1,"maxLength":256}},"additionalProperties":false}`

// RememberTool is axlr_remember, offered while the model records memory
// itself: KMP has kmp_write_memory and no ceremony or mode refuses it.
func RememberTool() domain.AvailableTool {
	identity, _ := domain.NewHostToolIdentity(domain.HostOperationRemember)
	schema, _ := root.NewJSONObject([]byte(rememberSchema))
	return domain.AvailableTool{Identity: identity, Definition: root.ToolDefinition{
		Name:        HostRememberName,
		Description: "Record one durable memory in KMP in one call: kind, text, its evidence and optional links to refs you saw (rel such as supports, depends_on, supersedes, contradicts, corrects, restates or derived_from, and why). The console writes it under the session's exact about unless you name one, labels it with the session and workspace, and derives the idempotency key, so a retry never duplicates it; it needs no kmp_guide or schema read. A write with links returns needs_review with the stored context they touch and writes nothing: check it, then call again with only the continuation to commit, or correct the links. Approved like kmp_write_memory.",
		Parameters:  schema,
	}}
}

// offersRemember reports whether a request offers axlr_remember: KMP is
// connected with kmp_write_memory and the model records memory itself.
func (u ContinueTurnUseCase) offersRemember(s domain.Session) bool {
	return u.Remember != nil && modelRecordsMemory(s)
}

// RememberRequest is one memory for KMP as the console writes it.
type RememberRequest struct {
	About, Kind, Text string
	Evidence          []string
	Links             []MemoryLink
	Labels            map[string][]string
	IdempotencyKey    string
	// Continuation resumes a write KMP returned for review; the other
	// fields are then empty.
	Continuation string
}

// RememberPort writes a RememberRequest and returns KMP's answer, compact.
type RememberPort interface {
	Remember(ctx context.Context, request RememberRequest) (map[string]any, error)
}

// hostRemember turns axlr_remember's arguments into a request: the
// session's exact about unless one is named, labels for the session and
// workspace, and a key derived from what is recorded.
func (u HostToolUseCase) hostRemember(ctx context.Context, session domain.Session, arguments root.JSONValue) (any, error) {
	if u.Memory == nil {
		return nil, errors.New("KMP is not connected in this console")
	}
	if !modelRecordsMemory(session) {
		return nil, errors.New("this session does not record memory itself: the mode or the running ceremony leaves it to the console")
	}
	var args struct {
		About, Kind, Text, Continuation string
		Evidence                        []string
		Links                           []struct{ Ref, Rel, Why, Evidence string }
	}
	if err := json.Unmarshal(arguments.Bytes(), &args); err != nil {
		return nil, errors.New("axlr_remember arguments must match its schema")
	}
	if args.Continuation != "" {
		if args.Kind != "" || args.Text != "" || len(args.Evidence) != 0 || len(args.Links) != 0 || args.About != "" {
			return nil, errors.New("send the continuation alone to commit a reviewed write")
		}
		return u.Memory.Remember(ctx, RememberRequest{Continuation: args.Continuation})
	}
	if args.Kind == "" || strings.TrimSpace(args.Text) == "" || len(args.Evidence) == 0 {
		return nil, errors.New("axlr_remember needs kind, text and at least one evidence item, or a continuation")
	}
	state := session.Export()
	about := args.About
	if about == "" {
		about = "ws:" + string(state.ID)
		if u.Labels != nil {
			labels, err := u.Labels.Load(ctx)
			if err != nil {
				return nil, err
			}
			if selected := labels[state.ID].About; selected != "" {
				about = selected
			}
		}
	}
	request := RememberRequest{About: about, Kind: args.Kind, Text: args.Text, Evidence: args.Evidence,
		Labels: map[string][]string{"session": {string(state.ID)}, "ws": {string(state.Workspace)}}}
	for _, link := range args.Links {
		request.Links = append(request.Links, MemoryLink{Ref: link.Ref, Rel: link.Rel, Why: link.Why, Evidence: link.Evidence})
	}
	sum := sha256.Sum256([]byte(about + "\x00" + args.Kind + "\x00" + args.Text))
	request.IdempotencyKey = "axlr:remember:" + hex.EncodeToString(sum[:8])
	return u.Memory.Remember(ctx, request)
}

// rememberAccepted reads an axlr_remember result: a host result, not a
// runtime envelope, whose memory KMP accepted.
func rememberAccepted(result string) (about, key string, ok bool) {
	var written struct {
		Accepted       bool   `json:"accepted"`
		About          string `json:"about"`
		IdempotencyKey string `json:"idempotency_key"`
	}
	if json.Unmarshal([]byte(result), &written) != nil || !written.Accepted {
		return "", "", false
	}
	return written.About, written.IdempotencyKey, true
}
