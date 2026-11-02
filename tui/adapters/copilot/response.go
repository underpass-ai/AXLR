package copilot

import (
	"context"
	"errors"
	"fmt"
	"strings"

	sdk "github.com/github/copilot-sdk/go"
	"github.com/github/copilot-sdk/go/rpc"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// readResponse turns a streaming session's events, in the order the SDK
// delivers them, into the request's result. Each assistant message's
// deltas reach onText as they arrive; its final content adds only what
// the deltas did not carry, all of it when none arrived, so no text is
// shown twice. Reasoning deltas go to the console's reasoning progress,
// never to onText. A message with tool requests ends the request: abort
// stops the SDK's loop before it can retry the calls it was denied, and
// the calls go back to AXLR. The request ends when ctx does.
func readResponse(ctx context.Context, events <-chan sdk.SessionEvent, abort func(), defs []root.ToolDefinition, onText func(root.Text) error) (root.CompletionResult, error) {
	// Its usage is subscription credits: the ledger counts the request
	// under this name as not metered in USD.
	result := root.CompletionResult{Provider: domain.CopilotProvider}
	var content strings.Builder
	// streamed is what each message's deltas carried, in order, until its
	// final content arrives.
	streamed := map[string]string{}
	var order []string
	reasoning := 0
	// finish keeps the text of a message whose final content never came.
	finish := func() {
		for _, id := range order {
			content.WriteString(streamed[id])
			delete(streamed, id)
		}
		order = nil
	}
	for {
		select {
		case <-ctx.Done():
			return root.CompletionResult{}, ctx.Err()
		case event := <-events:
			if err := ctx.Err(); err != nil {
				return root.CompletionResult{}, err
			}
			if event.AgentID != nil && *event.AgentID != "" {
				return root.CompletionResult{}, errors.New("Copilot returned a subagent event")
			}
			switch data := event.Data.(type) {
			case *rpc.AssistantMessageDeltaData:
				if data.DeltaContent == "" {
					continue
				}
				text, err := root.NewText(data.DeltaContent)
				if err != nil {
					return root.CompletionResult{}, err
				}
				if _, seen := streamed[data.MessageID]; !seen {
					order = append(order, data.MessageID)
				}
				streamed[data.MessageID] += data.DeltaContent
				if err := onText(text); err != nil {
					return root.CompletionResult{}, err
				}
			case *rpc.AssistantReasoningDeltaData:
				reasoning += len(data.DeltaContent)
				application.NotifyProviderActivity(ctx, domain.ProviderReasoning)
				application.NotifyReasoningProgress(ctx, reasoning, data.DeltaContent)
			case *rpc.AssistantMessageData:
				// The final content is the message: what the deltas showed is
				// its prefix, and only the rest is still to show. Content that
				// does not extend them replaces nothing already shown.
				shown, seen := streamed[data.MessageID]
				if seen {
					delete(streamed, data.MessageID)
					order = removeID(order, data.MessageID)
				}
				if rest, extends := strings.CutPrefix(data.Content, shown); extends && rest != "" {
					text, err := root.NewText(rest)
					if err != nil {
						return root.CompletionResult{}, err
					}
					if err := onText(text); err != nil {
						return root.CompletionResult{}, err
					}
				}
				content.WriteString(data.Content)
				if len(data.ToolRequests) > 0 {
					calls, err := mapCalls(data.ToolRequests, defs)
					if err != nil {
						return root.CompletionResult{}, err
					}
					abort()
					finish()
					result.Message = root.Message{Role: root.RoleAssistant, Content: root.Text(content.String()), ToolCalls: calls}
					result.FinishReason = "tool_calls"
					return result, nil
				}
			case *rpc.AssistantUsageData:
				usage := &root.TokenUsage{}
				if data.InputTokens != nil {
					usage.PromptTokens = int(*data.InputTokens)
				}
				if data.OutputTokens != nil {
					usage.CompletionTokens = int(*data.OutputTokens)
				}
				if data.CacheReadTokens != nil {
					usage.CachedTokens = int(*data.CacheReadTokens)
				}
				usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
				result.Usage = usage
			case *rpc.SessionErrorData:
				return root.CompletionResult{}, fmt.Errorf("Copilot request failed (%s)", data.ErrorType)
			case *rpc.SessionIdleData:
				finish()
				if content.Len() == 0 {
					return root.CompletionResult{}, errors.New("Copilot returned no assistant response")
				}
				text, err := root.NewText(content.String())
				if err != nil {
					return root.CompletionResult{}, err
				}
				result.Message = root.Message{Role: root.RoleAssistant, Content: text}
				result.FinishReason = "stop"
				return result, nil
			}
		}
	}
}

func removeID(ids []string, id string) []string {
	kept := ids[:0]
	for _, each := range ids {
		if each != id {
			kept = append(kept, each)
		}
	}
	return kept
}
