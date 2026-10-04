package ceremonyhost

import (
	"context"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
)

type scriptedModel struct {
	replies  []string
	requests []root.CompletionRequest
}

func (m *scriptedModel) Stream(_ context.Context, request root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
	m.requests = append(m.requests, request)
	reply := m.replies[0]
	m.replies = m.replies[1:]
	arguments, _ := root.NewJSONObject([]byte(reply))
	return root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{{ID: "c1", Name: reviewVerdictTool, Arguments: arguments}}}}, nil
}

func TestReviewerSeesOnlyTheDraftAndRetriesAnUnparseableVerdict(t *testing.T) {
	model := &scriptedModel{replies: []string{`{"accepted":false,"findings":[]}`, `{"accepted":false,"findings":["names the on-call engineer"]}`}}
	verdict, err := (Reviewer{Models: model, Model: "judge/model"}).Review(context.Background(), application.ReviewRequest{SessionModel: "session/model", Rubric: "rubric", Draft: "# draft", ReturnReason: "falta impacto"})
	if err != nil || verdict.Accepted || verdict.Findings[0] != "names the on-call engineer" || verdict.Model != "judge/model" {
		t.Fatalf("%+v %v", verdict, err)
	}
	request := model.requests[0]
	if len(model.requests) != 2 || request.Model != "judge/model" || len(request.Messages) != 2 || len(request.Tools) != 1 {
		t.Fatalf("reviewer context is not fresh: %+v", request)
	}
	if !strings.Contains(string(request.Messages[1].Content), "falta impacto") {
		t.Fatal("reviewer did not see the person's reason")
	}
}

func TestReviewerDefaultsToTheSessionModelAndGivesUpAfterTwoBadVerdicts(t *testing.T) {
	model := &scriptedModel{replies: []string{`{"verdict":"ok"}`, `{"accepted":"yes"}`}}
	if _, err := (Reviewer{Models: model}).Review(context.Background(), application.ReviewRequest{SessionModel: "session/model", Draft: "x"}); err == nil {
		t.Fatal("bad verdicts accepted")
	}
	if model.requests[0].Model != "session/model" {
		t.Fatalf("model %s", model.requests[0].Model)
	}
}
