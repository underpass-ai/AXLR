package application

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// An improvement the agent requests has no failure class to check: the
// console can prove that the cited calls exist, were resolved and belong to
// AXLR's own surface, not that their results were awkward. The judgement is
// the model's, under the guidance; the console bounds how often it acts on it.
const (
	// classFriction is a cited call on AXLR's surface, whatever its outcome:
	// a program that exited non-zero while working around a missing
	// capability is the friction itself.
	classFriction = "friction"
	// minImprovementCalls is how many distinct calls an improvement cites.
	minImprovementCalls = 2
	// MaxImprovementsPerBuild bounds the improvement sessions agents of one
	// console build may start; more are the person's to start by hand.
	MaxImprovementsPerBuild = 3
)

// decodeImprovementRequest reads axlr_request_improvement's arguments, which
// have repair's shape: description, expected, observed, evidence, tool_calls.
func decodeImprovementRequest(arguments root.JSONValue) (repairRequest, error) {
	var request repairRequest
	decoder := json.NewDecoder(bytes.NewReader(arguments.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return repairRequest{}, fmt.Errorf("axlr_request_improvement arguments: %w", err)
	}
	request.Description, request.Expected, request.Observed = strings.TrimSpace(request.Description), strings.TrimSpace(request.Expected), strings.TrimSpace(request.Observed)
	switch {
	case len([]rune(request.Description)) < minRepairDescription:
		return repairRequest{}, fmt.Errorf("description needs at least %d characters: say what AXLR should do better", minRepairDescription)
	case len(request.Description) > maxRepairText || len(request.Expected) > maxRepairText || len(request.Observed) > maxRepairText:
		return repairRequest{}, fmt.Errorf("description, expected and observed take at most %d bytes each", maxRepairText)
	case request.Expected == "" || request.Observed == "":
		return repairRequest{}, errors.New("expected and observed are required: what AXLR should let you do and what you had to do instead")
	case len(request.Evidence) == 0 || len(request.Evidence) > maxRepairEvidence:
		return repairRequest{}, fmt.Errorf("evidence needs between 1 and %d concrete items: outputs, messages or file paths you observed", maxRepairEvidence)
	case len(request.ToolCalls) < minImprovementCalls || len(request.ToolCalls) > maxRepairCitedCalls:
		return repairRequest{}, fmt.Errorf("tool_calls needs between %d and %d IDs of calls from this session that show the friction", minImprovementCalls, maxRepairCitedCalls)
	}
	if err := request.normalizeItems(); err != nil {
		return repairRequest{}, err
	}
	return request, nil
}

// citedFriction resolves the request's call IDs against the session. Each
// must be a resolved call of AXLR's own tools that the person or the mode did
// not deny and nobody cancelled; one that is not refuses the request.
func citedFriction(s domain.Session, request repairRequest) ([]citedFailure, string) {
	state := s.Export()
	byID := map[root.ToolCallID]domain.PendingTool{}
	for _, record := range state.Activity {
		byID[record.Call.ID] = record
	}
	var calls []citedFailure
	for _, id := range request.ToolCalls {
		record, ok := byID[root.ToolCallID(id)]
		switch {
		case !ok:
			return nil, fmt.Sprintf("call %s is not in this session; cite IDs of tool calls you made here", id)
		case record.Outcome == nil:
			return nil, fmt.Sprintf("call %s has no outcome yet", id)
		case record.Decision == domain.DecisionDeny:
			return nil, fmt.Sprintf("call %s was denied by the user or the mode; a denial is not friction of AXLR", id)
		}
		tool, known, err := hostFindTool(state.ToolSnapshot, record.Call.Name)
		switch {
		case err != nil || !known:
			return nil, fmt.Sprintf("call %s names a tool this session does not know", id)
		case tool.Identity.Kind == domain.ToolKindPlugin:
			return nil, fmt.Sprintf("call %s is %s, an MCP plugin tool of %s: improving it belongs to that server, not to AXLR", id, record.Call.Name, tool.Identity.Plugin.PluginID)
		}
		content := string(record.Outcome.Content)
		if content == "tool call cancelled" || strings.HasPrefix(content, "tool call denied") {
			return nil, fmt.Sprintf("call %s was cancelled or denied; that is not friction of AXLR", id)
		}
		calls = append(calls, citedFailure{ID: record.Call.ID, Tool: record.Call.Name, Class: classFriction})
	}
	return calls, ""
}

// improvementSignature identifies an improvement by the repository, the
// tools that showed the friction and the description's first words, so the
// same request is not started twice by one build.
func improvementSignature(repository string, calls []citedFailure, description string) string {
	tools := map[string]bool{}
	for _, call := range calls {
		tools[string(call.Tool)] = true
	}
	sorted := make([]string, 0, len(tools))
	for tool := range tools {
		sorted = append(sorted, tool)
	}
	sort.Strings(sorted)
	sum := sha256.Sum256([]byte(repository + "\nimprove\n" + strings.Join(sorted, "\n") + "\n" + slugWords(description)))
	return hex.EncodeToString(sum[:8])
}

// improvementBrief is the brief the improvement session starts with.
func improvementBrief(s domain.Session, request repairRequest, calls []citedFailure, build string) string {
	return requestBrief(s, request, calls, build, "Calls that showed the friction, as the origin session saw them:", improvementClosing)
}

// admitImprovement applies the rules that bound agent-requested
// improvements: the slots shared with repairs, one per origin session, the
// cap per build and no duplicate of a running or merged improvement.
func (r *SelfRepair) admitImprovement(records []domain.RepairRecord, session domain.SessionID, signature string) string {
	active, started := 0, 0
	for _, record := range records {
		if record.Session == session {
			return fmt.Sprintf("this session is the %s session of %s; it cannot request an improvement", record.Kind(), record.ID)
		}
		if record.Active() {
			active++
		}
		if !record.Improvement {
			continue
		}
		if record.Parent == session && record.Session != "" {
			return fmt.Sprintf("this session already requested improvement %s (%s); an agent requests at most one improvement per session", record.ID, record.Status)
		}
		if record.Signature == signature && record.Build == r.Build && record.Status != domain.RepairFailed {
			if record.Status == domain.RepairCompleted {
				return fmt.Sprintf("already improved: pull request #%d (%s) merged this improvement as %s, but this console still runs build %s; update or rebuild AXLR and restart", record.PullRequest, record.URL, record.MergeSHA, r.Build)
			}
			return fmt.Sprintf("duplicate: improvement %s is already %s; consult axlr_repair_status instead of requesting it again", record.ID, record.Status)
		}
		if record.Build == r.Build && record.Session != "" {
			started++
		}
	}
	if limit := r.maxActive(); active >= limit {
		return fmt.Sprintf("another self-repair or improvement is active (%d of %d allowed by jobs.max_active); wait for it or consult axlr_repair_status", active, limit)
	}
	if started >= MaxImprovementsPerBuild {
		return fmt.Sprintf("this console build already started %d improvements; tell the user, who can start more with axlr-tui --improve \"<brief>\"", started)
	}
	return ""
}
