package application

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// Failure classes the console accepts as evidence of a defect of AXLR. Every
// other failure a tool can report belongs to the project, the arguments, the
// workspace, a credential, a provider or an external service.
const (
	// classRuntimeFailure is AXLR's own machinery failing: a local tool that
	// reports internal_error, filesystem_error or process_error, a tool whose
	// outcome the console could not read, or a host tool whose execution
	// failed with an effect unknown.
	classRuntimeFailure = "runtime_failure"
	// classHostError is a host tool refusing for a reason that is not an
	// argument error.
	classHostError = "host_error"
	// classWrongResult is a call that succeeded with a result the model
	// shows to be wrong; it needs two cited calls, since the console cannot
	// judge the content.
	classWrongResult = "wrong_result"
)

// repairRequest is what the model sends through axlr_request_repair.
type repairRequest struct {
	Description string   `json:"description"`
	Expected    string   `json:"expected"`
	Observed    string   `json:"observed"`
	Evidence    []string `json:"evidence"`
	ToolCalls   []string `json:"tool_calls"`
}

const (
	minRepairDescription = 20
	maxRepairText        = 2000
	maxRepairEvidence    = 8
	maxRepairEvidenceLen = 1000
	maxRepairCitedCalls  = 8
	maxRepairBrief       = 8 << 10
)

func decodeRepairRequest(arguments root.JSONValue) (repairRequest, error) {
	var request repairRequest
	decoder := json.NewDecoder(bytes.NewReader(arguments.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return repairRequest{}, fmt.Errorf("axlr_request_repair arguments: %w", err)
	}
	request.Description, request.Expected, request.Observed = strings.TrimSpace(request.Description), strings.TrimSpace(request.Expected), strings.TrimSpace(request.Observed)
	switch {
	case len([]rune(request.Description)) < minRepairDescription:
		return repairRequest{}, fmt.Errorf("description needs at least %d characters: say which AXLR operation fails and how", minRepairDescription)
	case len(request.Description) > maxRepairText || len(request.Expected) > maxRepairText || len(request.Observed) > maxRepairText:
		return repairRequest{}, fmt.Errorf("description, expected and observed take at most %d bytes each", maxRepairText)
	case request.Expected == "" || request.Observed == "":
		return repairRequest{}, errors.New("expected and observed are required: what AXLR should have done and what it did")
	case len(request.Evidence) == 0 || len(request.Evidence) > maxRepairEvidence:
		return repairRequest{}, fmt.Errorf("evidence needs between 1 and %d concrete items: outputs, messages or file paths you observed", maxRepairEvidence)
	case len(request.ToolCalls) == 0 || len(request.ToolCalls) > maxRepairCitedCalls:
		return repairRequest{}, fmt.Errorf("tool_calls needs between 1 and %d IDs of calls from this session that show the failure", maxRepairCitedCalls)
	}
	if err := request.normalizeItems(); err != nil {
		return repairRequest{}, err
	}
	return request, nil
}

// normalizeItems trims the evidence and the cited call IDs and refuses empty,
// oversized or repeated ones.
func (r *repairRequest) normalizeItems() error {
	for i, item := range r.Evidence {
		r.Evidence[i] = strings.TrimSpace(item)
		if r.Evidence[i] == "" || len(r.Evidence[i]) > maxRepairEvidenceLen {
			return fmt.Errorf("evidence items must be nonempty and at most %d bytes", maxRepairEvidenceLen)
		}
	}
	seen := map[string]bool{}
	for i, id := range r.ToolCalls {
		r.ToolCalls[i] = strings.TrimSpace(id)
		if r.ToolCalls[i] == "" || seen[r.ToolCalls[i]] {
			return errors.New("tool_calls must be distinct, nonempty call IDs")
		}
		seen[r.ToolCalls[i]] = true
	}
	return nil
}

func (r repairRequest) texts() []string {
	return append([]string{r.Description, r.Expected, r.Observed}, r.Evidence...)
}

// externalCauses are the words that name a cause outside AXLR: a provider,
// a credential, a permission or a network. A request that leans on them is
// refused, whatever its tool evidence says, because repairing AXLR would not
// change them. Matched on word boundaries, case-insensitively.
var externalCauses = []string{
	"openrouter", "api key", "apikey", "credential", "credentials", "unauthorized", "unauthenticated", "forbidden", "401", "403", "429",
	"rate limit", "rate limited", "quota", "billing", "permission denied", "eacces", "eperm", "not logged in", "gh auth", "login required",
	"connection refused", "connection reset", "no such host", "dns", "network unreachable", "proxy", "certificate", "tls handshake", "econnrefused", "etimedout",
}

var externalCausePattern = func() *regexp.Regexp {
	quoted := make([]string, 0, len(externalCauses))
	for _, cause := range externalCauses {
		quoted = append(quoted, regexp.QuoteMeta(cause))
	}
	return regexp.MustCompile(`(?i)(?:^|[^a-z0-9_])(` + strings.Join(quoted, "|") + `)(?:$|[^a-z0-9_])`)
}()

// externalCause returns the first external cause the texts name, or "".
func externalCause(texts ...string) string {
	for _, text := range texts {
		if match := externalCausePattern.FindStringSubmatch(text); match != nil {
			return strings.ToLower(match[1])
		}
	}
	return ""
}

// citedFailure is one call of the session the request names, as the console
// classified it.
type citedFailure struct {
	ID     root.ToolCallID
	Tool   root.ToolName
	Class  string
	Detail string
}

// localFaultsOfAXLR are failed-status codes the runtime reports for its own
// trouble; the other failed codes describe the program or the workspace.
var localFaultsOfAXLR = map[string]bool{"internal_error": true, "filesystem_error": true, "process_error": true}

// argumentWords mark a host tool error that describes the model's arguments.
var argumentWords = []string{"must ", "must be", "unknown host argument", "duplicate host argument", "requires", "exceeds", "invalid", "only supported", "needs ", "not a ", "cannot include", "extra host argument", "unknown registered", "define the session title"}

// classifyCall decides whether a resolved call is evidence of a defect of
// AXLR. The refusal names what the failure belongs to instead, so the model
// learns the boundary rather than guessing a rephrasing.
func classifyCall(snapshot []domain.AvailableTool, record domain.PendingTool) (citedFailure, string) {
	cited := citedFailure{ID: record.Call.ID, Tool: record.Call.Name}
	if record.Outcome == nil {
		return cited, fmt.Sprintf("call %s has no outcome yet", record.Call.ID)
	}
	tool, known, err := hostFindTool(snapshot, record.Call.Name)
	if err != nil || !known {
		return cited, fmt.Sprintf("call %s names a tool this session does not know; a refused unknown tool is not a defect", record.Call.ID)
	}
	if tool.Identity.Kind == domain.ToolKindPlugin {
		return cited, fmt.Sprintf("call %s is %s, an MCP plugin tool of %s: its failures belong to that server, not to AXLR", record.Call.ID, record.Call.Name, tool.Identity.Plugin.PluginID)
	}
	if record.Decision == domain.DecisionDeny {
		return cited, fmt.Sprintf("call %s was denied by the user or the mode; a denial is not a defect", record.Call.ID)
	}
	content := string(record.Outcome.Content)
	switch {
	case content == "tool call cancelled" || strings.HasPrefix(content, "tool call denied"):
		return cited, fmt.Sprintf("call %s was cancelled or denied; that is not a defect", record.Call.ID)
	case strings.HasPrefix(content, "tool execution failed; effect unknown: "):
		detail := strings.TrimPrefix(content, "tool execution failed; effect unknown: ")
		if cause := externalCause(detail); cause != "" {
			return cited, fmt.Sprintf("call %s failed for an external cause (%s), not a defect of AXLR", record.Call.ID, cause)
		}
		if strings.Contains(detail, "context canceled") || strings.Contains(detail, "deadline exceeded") {
			return cited, fmt.Sprintf("call %s was cancelled or timed out; that is not proof of a defect", record.Call.ID)
		}
		cited.Class, cited.Detail = classRuntimeFailure, bounded(detail, 300)
		return cited, ""
	}
	if tool.Identity.Kind == domain.ToolKindHost {
		var host struct {
			Error *string `json:"error"`
		}
		if json.Unmarshal([]byte(content), &host) != nil {
			cited.Class, cited.Detail = classRuntimeFailure, "unreadable host result"
			return cited, ""
		}
		if host.Error == nil {
			cited.Class = classWrongResult
			return cited, ""
		}
		lower := strings.ToLower(*host.Error)
		for _, word := range argumentWords {
			if strings.Contains(lower, word) {
				return cited, fmt.Sprintf("call %s was refused for its arguments (%s); correct the call instead of repairing AXLR", record.Call.ID, bounded(*host.Error, 160))
			}
		}
		if cause := externalCause(*host.Error); cause != "" {
			return cited, fmt.Sprintf("call %s failed for an external cause (%s), not a defect of AXLR", record.Call.ID, cause)
		}
		cited.Class, cited.Detail = classHostError, bounded(*host.Error, 300)
		return cited, ""
	}
	var envelope struct {
		Status string `json:"status"`
		Output *struct {
			ExitCode *int `json:"exit_code"`
		} `json:"output"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(content), &envelope) != nil || envelope.Status == "" {
		cited.Class, cited.Detail = classRuntimeFailure, "unreadable tool outcome"
		return cited, ""
	}
	code, message := "", ""
	if envelope.Error != nil {
		code, message = envelope.Error.Code, envelope.Error.Message
	}
	switch envelope.Status {
	case "completed":
		if envelope.Output != nil && envelope.Output.ExitCode != nil && *envelope.Output.ExitCode != 0 {
			return cited, fmt.Sprintf("call %s ran a program that exited %d: a failure of that program or of the project, not of AXLR", record.Call.ID, *envelope.Output.ExitCode)
		}
		cited.Class = classWrongResult
		return cited, ""
	case "rejected":
		return cited, fmt.Sprintf("call %s was rejected by AXLR (%s: %s); correct the arguments instead of repairing AXLR", record.Call.ID, code, bounded(message, 160))
	case "timed_out", "cancelled":
		return cited, fmt.Sprintf("call %s %s; that is not proof of a defect", record.Call.ID, strings.ReplaceAll(envelope.Status, "_", " "))
	case "failed":
		if cause := externalCause(message); cause != "" {
			return cited, fmt.Sprintf("call %s failed for an external cause (%s), not a defect of AXLR", record.Call.ID, cause)
		}
		if !localFaultsOfAXLR[code] {
			return cited, fmt.Sprintf("call %s failed with %s (%s): the workspace, the program or a permission, not AXLR", record.Call.ID, code, bounded(message, 160))
		}
		cited.Class, cited.Detail = classRuntimeFailure, bounded(code+": "+message, 300)
		return cited, ""
	}
	cited.Class, cited.Detail = classRuntimeFailure, "unknown outcome status "+envelope.Status
	return cited, ""
}

// citedFailures resolves the request's call IDs against the session and
// classifies each. One refusal refuses the request.
func citedFailures(s domain.Session, request repairRequest) ([]citedFailure, string) {
	state := s.Export()
	byID := map[root.ToolCallID]domain.PendingTool{}
	for _, record := range state.Activity {
		byID[record.Call.ID] = record
	}
	var failures []citedFailure
	for _, id := range request.ToolCalls {
		record, ok := byID[root.ToolCallID(id)]
		if !ok {
			return nil, fmt.Sprintf("call %s is not in this session; cite IDs of tool calls you made here", id)
		}
		cited, refusal := classifyCall(state.ToolSnapshot, record)
		if refusal != "" {
			return nil, refusal
		}
		failures = append(failures, cited)
	}
	return failures, ""
}

// recurs reports whether the failure is more than an isolated error: the
// same tool failed the same way at least twice in this session, counting
// calls the model did not cite. A wrong result cannot be found in the
// transcript by the console, so it needs two cited calls.
func recurs(s domain.Session, failures []citedFailure) bool {
	state := s.Export()
	counts := map[string]int{}
	for _, record := range state.Activity {
		if record.Outcome == nil {
			continue
		}
		cited, refusal := classifyCall(state.ToolSnapshot, record)
		if refusal != "" || cited.Class == classWrongResult {
			continue
		}
		counts[string(cited.Tool)+"|"+cited.Class]++
	}
	wrong := 0
	for _, failure := range failures {
		if failure.Class == classWrongResult {
			wrong++
			continue
		}
		if counts[string(failure.Tool)+"|"+failure.Class] >= 2 {
			return true
		}
	}
	return wrong >= 2
}

// repairSignature identifies the defect by the repository and the tools and
// failure classes that showed it, not by the model's wording.
func repairSignature(repository string, failures []citedFailure) string {
	keys := map[string]bool{}
	for _, failure := range failures {
		keys[string(failure.Tool)+"|"+failure.Class] = true
	}
	sorted := make([]string, 0, len(keys))
	for key := range keys {
		sorted = append(sorted, key)
	}
	sort.Strings(sorted)
	sum := sha256.Sum256([]byte(repository + "\n" + strings.Join(sorted, "\n")))
	return hex.EncodeToString(sum[:8])
}

// repairBrief is the failure brief the repair session starts with: the
// model's account plus the exact outcomes it cited, so the repair session
// reproduces what the origin session saw rather than a paraphrase.
func repairBrief(s domain.Session, request repairRequest, failures []citedFailure, build string) string {
	return requestBrief(s, request, failures, build, "Failing calls as the origin session saw them:",
		"Reproduce the defect in this clone with a command that fails before any change; a failure that cannot be shown here ends the repair blocked.")
}

// requestBrief is the brief a repair or improvement session starts with: the
// model's account, the evidence and the cited calls, then the closing line.
func requestBrief(s domain.Session, request repairRequest, cited []citedFailure, build, heading, closing string) string {
	state := s.Export()
	byID := map[root.ToolCallID]domain.PendingTool{}
	for _, record := range state.Activity {
		byID[record.Call.ID] = record
	}
	var brief strings.Builder
	fmt.Fprintf(&brief, "%s\n\n", request.Description)
	fmt.Fprintf(&brief, "Expected: %s\nObserved: %s\n\nEvidence from session %s (AXLR build %s, workspace %s):\n", request.Expected, request.Observed, state.ID, build, state.Workspace)
	for _, item := range request.Evidence {
		fmt.Fprintf(&brief, "- %s\n", item)
	}
	brief.WriteString("\n" + heading + "\n")
	for _, call := range cited {
		record := byID[call.ID]
		fmt.Fprintf(&brief, "- %s %s %s → %s\n", call.ID, call.Tool, bounded(singleLineText(string(record.Call.Arguments.Bytes())), 400), bounded(singleLineText(string(record.Outcome.Content)), 600))
	}
	brief.WriteString("\n" + closing)
	return bounded(brief.String(), maxRepairBrief)
}

func singleLineText(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// RepairSlug names a repair, its clone directory and its branch: a timestamp
// and the brief's first words, kebab-cased.
func RepairSlug(brief string, now time.Time) string {
	return now.Format("20060102-1504") + "-" + slugWords(brief)
}

// slugWords is the brief's first words, lowercased and kebab-cased, at most
// about 40 characters; "failure" when it has none.
func slugWords(brief string) string {
	var words []string
	length := 0
	for _, field := range strings.Fields(strings.ToLower(brief)) {
		var word strings.Builder
		for _, r := range field {
			if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
				word.WriteRune(r)
			}
		}
		if word.Len() == 0 {
			continue
		}
		if length+word.Len()+1 > 40 {
			break
		}
		words = append(words, word.String())
		length += word.Len() + 1
	}
	if len(words) == 0 {
		words = []string{"failure"}
	}
	return strings.Join(words, "-")
}
