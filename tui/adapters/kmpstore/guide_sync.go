package kmpstore

import (
	"context"
	"errors"
	"fmt"
)

// GuideSync writes KMP's guide into a store.
type GuideSync struct {
	CLI CLI
	// PluginRoot holds guide/guide.requests.json; empty syncs no guide.
	PluginRoot string
}

// Configured reports whether a guide can be written: a binary and the
// plugin root that holds the guide assets.
func (g GuideSync) Configured() bool { return g.CLI.Binary != "" && g.PluginRoot != "" }

// Sync writes the guide into the selected store: the console calls it
// when kmp_guide reports the guide missing (an existing store the console
// did not create). A live engine on the store does not block it (seen on
// 13 October 2026 with Claude Code's engine on AXLR/.kernel).
func (g GuideSync) Sync(ctx context.Context, selection Selection, env []string) error {
	if !g.Configured() {
		return errors.New("no KMP plugin root holds the guide assets")
	}
	_, err := g.CLI.run(ctx, []string{"guide", "sync", "--plugin-root", g.PluginRoot}, WithStore(env, selection.Path))
	if err != nil {
		return fmt.Errorf("synchronise the KMP guide: %w", err)
	}
	return nil
}
