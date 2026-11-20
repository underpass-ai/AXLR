package madesetup

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/application"
)

// ApproverGrantID names the approver's action set; a changed set needs a new
// id, as with WorkGrantID, and the grant issued carries the approver's suffix.
const ApproverGrantID = "axlr-approver-v1"

// approverActions let the person's keypress grant a human guard and nothing
// else; the work identity never holds them.
var approverActions = []string{"approve_ceremony_guard", "get_ceremony_instance"}

// approvalSourceKey is MADE's setting for where a human guard decision may
// come from: `host` records one the MCP session relays, `terminal` only one
// typed at `made-mcp approve-guard`.
const approvalSourceKey = "MADE_HUMAN_APPROVAL_SOURCE"

// approverIdentity is the identity that acts for the person at the console.
func approverIdentity(work string) string { return work + "-approver" }

// Approver is application.ApproverPort: it calls MADE in a fresh process as
// the approver identity, which P granted next to the work identity.
type Approver struct {
	ConfigPath string
	Getenv     func(string) string
	Call       ToolCaller
}

var _ application.ApproverPort = (*Approver)(nil)

func (a *Approver) ApproveGuard(ctx context.Context, instance, guard string) error {
	if a.ConfigPath == "" || !filepath.IsAbs(a.ConfigPath) {
		return errors.New("MADE approval is not configured")
	}
	configuration, err := storage.LoadMCPConfiguration(a.ConfigPath, a.Getenv)
	if err != nil {
		return err
	}
	for i := range configuration.Registrations {
		made := &configuration.Registrations[i]
		if made.Manifest.ID != "made" {
			continue
		}
		work := environment(made.Env)[hostIdentityKey]
		if !isWorkIdentity(work) {
			return errors.New("MADE has no AXLR work identity; prepare it: open /mcp, select MADE and press p")
		}
		// The approver relays the person's keypress at this console: MADE
		// records it as a relayed human decision (`host`). Its launcher
		// defaults to `terminal`, which refused every console approval of
		// a plan, a repair merge or a postmortem (20 November 2026); the
		// work identity's engine keeps that default, so the model still
		// cannot approve a guard. An entry that names a source keeps it.
		env := append(without(made.Env, hostIdentityKey), hostIdentityKey+"="+approverIdentity(work))
		if _, named := environment(made.Env)[approvalSourceKey]; !named {
			env = append(env, approvalSourceKey+"=host")
		}
		approver := server(made, env)
		call := a.Call
		if call == nil {
			call = CallOnce
		}
		if _, err := call(ctx, approver, "made_approve_ceremony_guard", map[string]any{"ceremony_id": instance, "guard_name": guard, "role_id": "HUMAN_APPROVER", "role_kind": "human"}); err != nil {
			return fmt.Errorf("approve %s as %s: %w", guard, approverIdentity(work), err)
		}
		return nil
	}
	return errors.New("MADE is not configured")
}
