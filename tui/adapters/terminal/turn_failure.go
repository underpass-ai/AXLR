package terminal

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/adapters/openrouter"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// providerHintKind is what the person can do about a provider's error, as
// the hint.* labels word it: the adapter's kind, with a refused key of a
// local endpoint told apart from OpenRouter's. "" when there is nothing.
func providerHintKind(err error) string {
	var provider *openrouter.ProviderError
	if !errors.As(err, &provider) {
		return ""
	}
	kind := string(provider.HintKind())
	if kind == string(openrouter.HintCheckKey) && provider.Provider != "" {
		kind += "_local"
	}
	return kind
}

// providerHint is a hint kind in the person's language, "" for none or one
// this console does not know.
func (t Theme) providerHint(kind string) string {
	if _, ok := enMessages["hint."+kind]; kind == "" || !ok {
		return ""
	}
	return t.T("hint." + kind)
}

// withProviderHint adds to an error's text what the person can do about
// it: on 12 October 2026 the footer read "OpenRouter invalid_request (HTTP
// 400)" for a model id the provider does not serve.
func (m AppModel) withProviderHint(err error, text string) string {
	if hint := m.Theme.providerHint(providerHintKind(err)); hint != "" {
		return text + " · " + hint
	}
	return text
}

// streamTimeoutText says a stream the provider adapter cut in the
// person's language: on 14 October 2026 a Spanish console read "!
// OpenRouter stream maximum duration exceeded" after glm-5.3-flash had
// reasoned for five minutes.
func (m AppModel) streamTimeoutText(err error, text string) string {
	var timeout *openrouter.StreamTimeoutError
	if !errors.As(err, &timeout) {
		return text
	}
	label := "error.streamInactivity"
	if timeout.MaximumDuration() {
		label = "error.streamMaxDuration"
	}
	return strings.Replace(text, timeout.Error(), m.Theme.Tf(label, timeout.Provider()), 1)
}

// streamTimeoutMessage reads a saved failure the adapter's stream timeout
// left, which the session keeps in English, in the person's language as
// the footer said it; any other message stays as saved.
func streamTimeoutMessage(message string, theme Theme) string {
	for suffix, label := range map[string]string{" stream maximum duration exceeded": "error.streamMaxDuration", " stream inactivity timeout": "error.streamInactivity"} {
		if provider, ok := strings.CutSuffix(message, suffix); ok && provider != "" && !strings.ContainsAny(provider, ":·") {
			return theme.Tf(label, provider)
		}
	}
	return message
}

// turnFailure is the console line a failed turn leaves, or false for an
// ending that is not a failure: the person's cancel, the tool-call limit
// and the budget pause say how to go on in the footer.
func turnFailure(err error) (domain.TurnFailure, bool) {
	var budget *domain.SessionBudgetError
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, domain.ErrToolCallLimit) || errors.As(err, &budget) {
		return domain.TurnFailure{}, false
	}
	message := singleLine(err.Error())
	var provider *openrouter.ProviderError
	if errors.As(err, &provider) {
		// The provider's own words, without what was joined around them.
		message = singleLine(provider.Error())
	}
	if utf8.RuneCountInString(message) > domain.MaxTurnFailureRunes {
		message = string([]rune(message)[:domain.MaxTurnFailureRunes-1]) + "…"
	}
	if strings.TrimSpace(message) == "" {
		return domain.TurnFailure{}, false
	}
	return domain.TurnFailure{Message: message, Hint: providerHintKind(err)}, true
}

// recordTurnFailure adds a failed turn to the session as a console line, so
// the transcript keeps it after the footer moves on and after reopening.
// It runs as an operation, like a plan's outcome, so a queued message
// starts after it; the footer keeps the error meanwhile. nil when the
// session cannot take a line now.
func (m *AppModel) recordTurnFailure(failure domain.TurnFailure) tea.Cmd {
	if m.Busy || m.deps.Session == nil || m.deps.Store == nil || len(m.deps.Session.Pending()) != 0 {
		return nil
	}
	switch m.deps.Session.Status() {
	case domain.StatusIdle, domain.StatusComplete, domain.StatusInterrupted:
	default:
		return nil
	}
	store := m.deps.Store
	shown := m.Status.Error
	cmd := m.BeginOperation(func(ctx context.Context, s *domain.Session, _ func(application.Event) error) error {
		next := *s
		if err := next.AddTurnFailure(failure); err != nil {
			return err
		}
		if err := store.Save(ctx, next); err != nil {
			return err
		}
		*s = next
		return nil
	})
	m.Status.Error = shown
	m.failureOperation = m.operationID
	return cmd
}
