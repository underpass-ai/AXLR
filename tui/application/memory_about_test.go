package application

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestProjectAboutIsDerivedFromTheWorkspaceRoot(t *testing.T) {
	for workspace, want := range map[domain.Workspace]string{
		"/home/tirso/ai/AXLR":        "project:axlr",
		"/home/tirso/ai/AXLR/":       "project:axlr",
		"/srv/My Service":            "project:my-service",
		"/home/tirso/ai/kmp\tengine": "project:kmp-engine",
		"/":                          "",
	} {
		if got := ProjectAbout(workspace); got != want {
			t.Fatalf("%q: got %q, want %q", workspace, got, want)
		}
	}
}

// On 10 October 2026 session A of claude-haiku-5.5 recorded its memory
// under ws:<session A>; session B, in the same workspace, looked for
// project:AXLR and project:axlr and answered "nothing recorded". The
// console's default is now the workspace's project about; a selected about
// still wins and the session stays a label.
func TestRememberDefaultsToTheWorkspacesProjectAbout(t *testing.T) {
	s := rememberSession(t, domain.ModeNormal)
	state := s.Export()
	labels := &contextLabels{labels: map[domain.SessionID]domain.SessionLabel{}}
	recorder := &rememberRecorder{answer: map[string]any{"accepted": true}}
	u := HostToolUseCase{Memory: recorder, Labels: labels}
	if _, err := u.Execute(context.Background(), s, rememberIdentity(t), mustObject(t, `{"kind":"decision","text":"Keep the parser strict.","evidence":["parser_test.go"]}`)); err != nil {
		t.Fatal(err)
	}
	first := recorder.requests[0]
	if want := ProjectAbout(state.Workspace); want == "" || first.About != want || first.Labels["session"][0] != string(state.ID) {
		t.Fatalf("default about %q, want %q (labels %v)", first.About, want, first.Labels)
	}
	labels.labels[state.ID] = domain.SessionLabel{About: "project:AXLR"}
	if _, err := u.Execute(context.Background(), s, rememberIdentity(t), mustObject(t, `{"kind":"decision","text":"Keep the parser strict.","evidence":["parser_test.go"]}`)); err != nil {
		t.Fatal(err)
	}
	if recorder.requests[1].About != "project:AXLR" {
		t.Fatalf("a selected about must win: %q", recorder.requests[1].About)
	}
}

// The model is told the console's about, so it never derives one itself.
func TestSessionToolAndPromptCarryTheDefaultAbout(t *testing.T) {
	s := rememberSession(t, domain.ModeNormal)
	state := s.Export()
	labels := &contextLabels{labels: map[domain.SessionID]domain.SessionLabel{}}
	identity, _ := domain.NewHostToolIdentity(domain.HostOperationSession)
	out, err := (HostToolUseCase{Labels: labels}).Execute(context.Background(), s, identity, mustObject(t, `{}`))
	if err != nil || out.IsError {
		t.Fatalf("%+v %v", out, err)
	}
	var value struct {
		About   string `json:"about"`
		Default bool   `json:"about_is_default"`
	}
	if err := json.Unmarshal([]byte(out.Content), &value); err != nil {
		t.Fatal(err)
	}
	if value.About != ProjectAbout(state.Workspace) || !value.Default {
		t.Fatalf("axlr_session: %s", out.Content)
	}
	guidance, err := sessionContextGuidance(context.Background(), s, labels)
	if err != nil || !strings.Contains(guidance, `"about":"`+ProjectAbout(state.Workspace)+`"`) {
		t.Fatalf("prompt metadata: %s %v", guidance, err)
	}
}
