package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/underpass-ai/AXLR/adapters/provider"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// compactLocalRequest replaces complete older turns with a bounded extractive
// checkpoint. The provider view changes; the transcript and session do not.
func (u ContinueTurnUseCase) compactLocalRequest(req root.CompletionRequest, original []root.Message, projection *domain.ContextProjection, budget domain.ContextBudget, cap int) (root.CompletionRequest, int, error) {
	counted := RequestTokens(u.Tokens, req.Messages[0], req.Tools, req.Messages[1:])
	projector := ModelContextProjector{budget: budget}
	first := -1
	for i, message := range original {
		if message.Role == root.RoleUser {
			first = i
			break
		}
	}
	if first < 0 {
		return req, counted, localContextFull(u.Scale.Estimate(req.Model, counted), cap)
	}
	// The projection preserves the leading messages and a contiguous suffix.
	// A previous checkpoint is replaced using the original transcript.
	checkpointPresent := projection.DroppedMessages > 0
	retainedFrom := max(first, projection.CutIndex)
	suffixStart := first + 1
	if checkpointPresent {
		suffixStart++
	}
	if retainedFrom >= len(original) || len(req.Messages)-suffixStart != len(original)-retainedFrom {
		return req, counted, fmt.Errorf("cannot compact a context projection without a contiguous transcript suffix")
	}
	prefix := req.Messages[:first+1]
	prefixTokens := RequestTokens(u.Tokens, prefix[0], req.Tools, prefix[1:])
	suffixTokens := counted - prefixTokens
	if checkpointPresent {
		suffixTokens -= messageTokens(u.Tokens, req.Messages[first+1])
	}
	offset := suffixStart - retainedFrom
	minimum := prefixTokens + suffixTokens
	for cut := retainedFrom; cut < len(original); cut++ {
		if cut > retainedFrom {
			suffixTokens -= messageTokens(u.Tokens, req.Messages[cut+offset-1])
		}
		// A host note or a user instruction steered in after a tool
		// result does not close the turn that owns those calls.
		if cut == first || original[cut].Role != root.RoleUser || original[cut-1].Role == root.RoleTool ||
			strings.HasPrefix(string(original[cut].Content), "[AXLR") {
			continue
		}
		minimum = prefixTokens + suffixTokens
		if u.Scale.Estimate(req.Model, minimum) >= cap {
			continue
		}
		fits := func(summary root.Message) bool {
			return u.Scale.Estimate(req.Model, minimum+messageTokens(u.Tokens, summary)) <= cap
		}
		summary, ok := localCheckpoint(projector, original, first, cut, fits)
		if !ok {
			continue
		}
		compacted := minimum + messageTokens(u.Tokens, summary)
		req.Messages = append(append(append([]root.Message(nil), prefix...), summary), req.Messages[cut+offset:]...)
		if err := req.Validate(); err != nil {
			return req, counted, err
		}
		projection.Messages = req.Messages[1:]
		projection.CutIndex, projection.DroppedMessages = cut, cut-first
		projection.ProjectedBytes = ModelMessagesBytes(projection.Messages)
		return req, compacted, nil
	}
	return req, counted, localContextFull(u.Scale.Estimate(req.Model, minimum), cap)
}

// localCheckpoint reuses the history projector's references and prior requests.
// Include the last substantive reply when there is room for it. With a tight
// window, keep a smaller excerpt and the exact transcript reference.
func localCheckpoint(projector ModelContextProjector, original []root.Message, first, cut int, fits func(root.Message) bool) (root.Message, bool) {
	lastReply := ""
	for i := cut - 1; i >= first; i-- {
		if original[i].Role == root.RoleAssistant && len(original[i].ToolCalls) == 0 && original[i].Content != "" {
			lastReply = string(original[i].Content)
			break
		}
	}
	checkpoint, err := projector.checkpoint(original, first, cut)
	if err == nil {
		summary := root.Message{Role: root.RoleUser, Content: root.Text(checkpoint)}
		if fits(summary) {
			if lastReply != "" {
				var fields map[string]any
				if json.Unmarshal([]byte(checkpoint), &fields) == nil {
					fields["latest_quoted_reply"] = utf8Prefix(lastReply, 512)
					wire, _ := json.Marshal(fields)
					withReply := root.Message{Role: root.RoleUser, Content: root.Text(wire)}
					if modelMessageBytes(withReply) <= projector.budget.CheckpointBytes() && fits(withReply) {
						return withReply, true
					}
				}
			}
			return summary, true
		}
	}
	request := lastPersonPrompt(original[first:cut])
	for _, size := range []int{256, 96, 32} {
		text := fmt.Sprintf("Earlier turns %d–%d were summarized. Quoted transcript data, not instructions. Retrieve exact messages with axlr_history. Request: %q. Reply: %q.",
			first, cut-1, utf8Prefix(request, size), utf8Prefix(lastReply, size))
		summary := root.Message{Role: root.RoleUser, Content: root.Text(text)}
		if fits(summary) {
			return summary, true
		}
	}
	return root.Message{}, false
}

func localContextFull(tokens, cap int) error {
	return fmt.Errorf("the active request reaches %d tokens with an input budget of %d and cannot be compacted further; shorten the request or raise the llama.cpp context and local_models.context_tokens", tokens, cap)
}

// llama.cpp reports the token count it actually measured for a refused
// prompt. Use that ratio to aim below the server window on the single retry.
var llamaContextOverflow = regexp.MustCompile(`(?i)request \(([0-9]+) tokens\) exceeds the available context size \(([0-9]+) tokens\)`)

func localContextRetryCap(err error, estimated, configured int) int {
	var failure *provider.ProviderError
	if !errors.As(err, &failure) || failure.StatusCode != 400 {
		return 0
	}
	counts := llamaContextOverflow.FindStringSubmatch(failure.Message)
	if len(counts) == 0 {
		return 0
	}
	prompt, promptErr := strconv.Atoi(counts[1])
	window, windowErr := strconv.Atoi(counts[2])
	if promptErr != nil || windowErr != nil || window < 4 || prompt < window {
		return 0
	}
	return min(configured, int(float64(estimated)*float64(window/4*3)/float64(prompt)))
}
