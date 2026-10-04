package ceremonyhost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
)

const reviewVerdictTool = "review_verdict"

// Reviewer is application.CeremonyReviewerPort: one model generation with no
// transcript and no tool except the verdict. Its independence is of context,
// not of identity.
type Reviewer struct {
	Models application.ModelStreamPort
	// Model overrides the session model (settings.json reviewer_model).
	Model string
}

var _ application.CeremonyReviewerPort = Reviewer{}

func (r Reviewer) Review(ctx context.Context, request application.ReviewRequest) (application.ReviewVerdict, error) {
	model := r.Model
	if model == "" {
		model = request.SessionModel
	}
	id, err := root.NewModelID(model)
	if err != nil {
		return application.ReviewVerdict{}, fmt.Errorf("reviewer model: %w", err)
	}
	schema, _ := root.NewJSONObject([]byte(`{"type":"object","properties":{"accepted":{"type":"boolean"},"findings":{"type":"array","items":{"type":"string"},"maxItems":12}},"required":["accepted","findings"],"additionalProperties":false}`))
	tools := []root.ToolDefinition{{Name: reviewVerdictTool, Description: "Give the verdict: accepted, and the findings that must change (empty when accepted).", Parameters: schema}}
	prompt := "Draft postmortem:\n\n" + request.Draft
	if request.ReturnReason != "" {
		prompt += "\n\nThe person returned an earlier version of this draft for this reason; check it is addressed:\n" + request.ReturnReason
	}
	messages := []root.Message{
		{Role: root.RoleSystem, Content: root.Text(request.Rubric + "\nAnswer only by calling " + reviewVerdictTool + " once.")},
		{Role: root.RoleUser, Content: root.Text(prompt)},
	}
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		result, err := r.Models.Stream(ctx, root.CompletionRequest{Model: id, Messages: messages, Tools: tools}, func(root.Text) error { return nil })
		if err != nil {
			return application.ReviewVerdict{}, err
		}
		verdict, err := parseVerdict(result.Message)
		if err == nil {
			verdict.Model = model
			return verdict, nil
		}
		lastErr = err // an unparseable verdict gets one more generation
	}
	return application.ReviewVerdict{}, lastErr
}

func parseVerdict(message root.Message) (application.ReviewVerdict, error) {
	for _, call := range message.ToolCalls {
		if call.Name != reviewVerdictTool {
			continue
		}
		var verdict struct {
			Accepted *bool    `json:"accepted"`
			Findings []string `json:"findings"`
		}
		decoder := json.NewDecoder(bytes.NewReader(call.Arguments.Bytes()))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&verdict); err != nil || verdict.Accepted == nil {
			return application.ReviewVerdict{}, errors.New("the reviewer's verdict is malformed")
		}
		findings := verdict.Findings[:0:0]
		for _, finding := range verdict.Findings {
			if strings.TrimSpace(finding) != "" {
				findings = append(findings, finding)
			}
		}
		if !*verdict.Accepted && len(findings) == 0 {
			return application.ReviewVerdict{}, errors.New("the reviewer rejected the draft without findings")
		}
		return application.ReviewVerdict{Accepted: *verdict.Accepted, Findings: findings}, nil
	}
	return application.ReviewVerdict{}, errors.New("the reviewer did not call " + reviewVerdictTool)
}
