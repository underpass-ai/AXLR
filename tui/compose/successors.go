package compose

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// Successors opens the session a capped prompt continues in: the same
// workspace, model, mode, about and owner, titled after the predecessor,
// with the predecessor in its instance (decision D10). The owner is the
// service principal of the predecessor; a console session has none. The
// execution's policy, budget, sandbox, routing and correlation continue in
// it too: the successor is the same execution (docs/runtime-contract.md).
type Successors struct {
	Create application.CreateSessionUseCase
	Labels application.SessionLabelsPort
	Store  application.SessionStorePort
	// Usage reads the predecessor's ledger, so the successor carries what
	// the execution spent; nil carries its tool calls alone.
	Usage application.SessionUsagePort
}

func (s Successors) Continue(ctx context.Context, predecessor domain.Session) (domain.Session, error) {
	state := predecessor.Export()
	session, err := s.Create.ExecuteNew(ctx, state.Workspace, state.Model)
	if err != nil {
		return domain.Session{}, err
	}
	if state.Owner != "" {
		session.SetServiceMetadata(state.Owner, 0, "")
	}
	if predecessor.Mode() != domain.ModeNormal {
		if err := session.SetMode(predecessor.Mode()); err != nil {
			return domain.Session{}, err
		}
	}
	if err := session.SetInstance(domain.SessionInstance{Predecessor: state.ID}); err != nil {
		return domain.Session{}, err
	}
	if policy, has := predecessor.Policy(); has {
		if err := session.SetPolicy(policy); err != nil {
			return domain.Session{}, err
		}
	}
	if sandbox, has := predecessor.Sandbox(); has {
		if err := session.SetSandbox(sandbox); err != nil {
			return domain.Session{}, err
		}
	}
	if skills, has := predecessor.Skills(); has {
		if err := session.SetSkills(skills); err != nil {
			return domain.Session{}, err
		}
	}
	if routing, has := predecessor.Routing(); has {
		if err := session.SetRouting(routing); err != nil {
			return domain.Session{}, err
		}
	}
	if correlation, has := predecessor.Correlation(); has {
		if err := session.SetCorrelation(correlation); err != nil {
			return domain.Session{}, err
		}
	}
	if account, has := predecessor.Budget(); has {
		var ledger domain.UsageLedger
		if s.Usage != nil {
			if loaded, err := s.Usage.LoadUsage(ctx, state.ID); err == nil {
				ledger = loaded
			}
		}
		if err := session.InheritBudget(account, predecessor.BudgetSpend(ledger, time.Now())); err != nil {
			return domain.Session{}, err
		}
	}
	if s.Labels != nil {
		if labels, err := s.Labels.Load(ctx); err == nil {
			previous := labels[state.ID]
			title := previous.Title
			if title != "" {
				title = root.Text(successorTitle(string(title)))
			}
			_, _ = s.Labels.Initialize(ctx, session.ID(), domain.SessionLabel{Title: title, TitleDerived: previous.TitleDerived, About: previous.About})
		}
	}
	if err := s.Store.Save(ctx, session); err != nil {
		return domain.Session{}, err
	}
	return session, nil
}

// successorTitle numbers a continued session's title: "X" becomes "X (2)"
// and "X (2)" becomes "X (3)", not "X (2) (2)".
func successorTitle(title string) string {
	if open := strings.LastIndex(title, " ("); open >= 0 && strings.HasSuffix(title, ")") {
		if n, err := strconv.Atoi(title[open+2 : len(title)-1]); err == nil && n >= 2 {
			return fmt.Sprintf("%s (%d)", bounded(title[:open], 100), n+1)
		}
	}
	return bounded(title, 100) + " (2)"
}

// bounded cuts text to at most limit bytes on a character boundary.
func bounded(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	cut := limit
	for cut > 0 && text[cut]&0xC0 == 0x80 {
		cut--
	}
	return text[:cut]
}
