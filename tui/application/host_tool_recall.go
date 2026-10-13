package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// HostRecallName reads KMP memory in one call; see RecallTool.
const HostRecallName root.ToolName = "axlr_recall"

// recallHistoryEntries is how many entries a history read returns.
const recallHistoryEntries = 10

const recallSchema = `{"type":"object","properties":{` +
	`"question":{"type":"string","minLength":1,"maxLength":2000,"description":"What you want to know (ask), or what you are resuming (wake)."},` +
	`"about":{"type":"string","minLength":1,"maxLength":256,"description":"The about to read; by default the session's (the one axlr_session shows)."},` +
	`"since":{"type":"string","minLength":4,"maxLength":40,"description":"An RFC 3339 instant or a date: history from it onwards, or a question answered within it."},` +
	`"mode":{"type":"string","enum":["ask","wake","history"],"description":"ask (default with a question): an evidence-backed answer or UNKNOWN. wake (default without one): where the work stands. history: the newest entries, or those since the instant."}},` +
	`"additionalProperties":false}`

// RecallTool is axlr_recall, offered whenever KMP is connected: the console
// supplies the session's about, its KMP context and the budget, and
// answers compactly, so a question costs one call and no guide, schema or
// page read (decision D3 of the lifecycle integration design).
func RecallTool() domain.AvailableTool {
	identity, _ := domain.NewHostToolIdentity(domain.HostOperationRecall)
	schema, _ := root.NewJSONObject([]byte(recallSchema))
	return domain.AvailableTool{Identity: identity, Definition: root.ToolDefinition{
		Name:        HostRecallName,
		Description: "Read the workspace's memory in one call. With a question: an evidence-backed answer under the session's about, or UNKNOWN, which is a valid answer; never invent a memory. mode wake: where the work on the about stands. mode history: its newest entries, or those since an instant. The console adds the about, the KMP context and the budget; it needs no kmp_guide or schema read. Read-only.",
		Parameters:  schema,
	}}
}

// hostRecall turns axlr_recall's arguments into a RecallRequest and
// compacts the result.
func (u HostToolUseCase) hostRecall(ctx context.Context, session domain.Session, arguments root.JSONValue) (any, error) {
	if u.Recall == nil {
		return nil, errors.New("memory is not connected in this console")
	}
	args, err := decodeHostArguments(arguments, "question", "about", "since", "mode", WhyArgument)
	if err != nil {
		return nil, err
	}
	var question, about, since, mode string
	for key, target := range map[string]*string{"question": &question, "about": &about, "since": &since, "mode": &mode} {
		if raw, exists := args[key]; exists {
			if err := json.Unmarshal(raw, target); err != nil {
				return nil, fmt.Errorf("%s must be a string", key)
			}
		}
	}
	question, about, since = strings.TrimSpace(question), strings.TrimSpace(about), strings.TrimSpace(since)
	switch mode {
	case "":
		mode = RecallWake
		if question != "" {
			mode = RecallAsk
		}
	case RecallAsk, RecallWake, RecallHistory:
	default:
		return nil, errors.New("mode must be ask, wake or history")
	}
	if mode == RecallAsk && question == "" {
		return nil, errors.New("ask needs a question")
	}
	if since != "" {
		instant, err := parseSince(since)
		if err != nil {
			return nil, err
		}
		since = instant
	}
	state := session.Export()
	if about == "" {
		if about, _, err = memoryAbout(ctx, u.Labels, state); err != nil {
			return nil, err
		}
	}
	request := RecallRequest{Mode: mode, About: about, Intent: question, Since: since, Entries: recallHistoryEntries}
	if instance, ok := session.Instance(); ok {
		request.ContextID = instance.KMPContext
	}
	result, err := u.Recall.Recall(ctx, request)
	if err != nil {
		return nil, err
	}
	answer := map[string]any{"about": result.About, "mode": result.Mode, "recorded": !result.Empty}
	if result.Empty {
		answer["memory"] = "nothing recorded under " + result.About
		return answer, nil
	}
	switch result.Mode {
	case RecallWake:
		answer["memory"] = result.Text
		if len(result.Refs) > 0 {
			answer["refs"] = result.Refs
		}
	default:
		for key, value := range result.Answer {
			answer[key] = value
		}
	}
	return answer, nil
}

// parseSince reads an RFC 3339 instant or a date (the start of that day,
// UTC) and returns the instant in RFC 3339.
func parseSince(text string) (string, error) {
	if instant, err := time.Parse(time.RFC3339, text); err == nil {
		return instant.UTC().Format(time.RFC3339), nil
	}
	if day, err := time.Parse("2006-01-02", text); err == nil {
		return day.UTC().Format(time.RFC3339), nil
	}
	return "", errors.New("since must be an RFC 3339 instant or a date (2026-10-13)")
}
