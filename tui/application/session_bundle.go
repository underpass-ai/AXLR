package application

import (
	"fmt"
	"time"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// Sessions travel between machines as bundles (decision I4 of the
// lifecycle integration design): /session export writes one, axlr-tui
// --import reads it and resumes the session here, where its MADE instance
// continues in a successor and its memory records the handoff.

// SessionBundleSuffix ends every bundle's file name.
const SessionBundleSuffix = ".axlr-session.json"

// ImportedInstance is the bundle's instance made this console's: what
// bound it to the origin's host and store is dropped (the live claim, the
// host reports, the KMP agent and context), it is closed so the next turn
// opens its successor with it as predecessor, and the handoff is queued
// as an observation under the session's about, written at the next turn.
func ImportedInstance(instance *domain.SessionInstance, id domain.SessionID, workspace domain.Workspace, meta SessionBundleMeta, host, source string, now time.Time) *domain.SessionInstance {
	next := domain.SessionInstance{}
	if instance != nil {
		next = domain.SessionInstance{Instance: instance.Instance, Turns: instance.Turns, Closed: instance.Instance != "", LastRecord: instance.LastRecord, Recalled: instance.Recalled, RecalledMemories: instance.RecalledMemories, Predecessor: instance.Predecessor}
		for _, encoded := range instance.Pending {
			entry, err := decodeEntry(encoded)
			if err != nil {
				continue
			}
			entry.ContextID = ""
			next.Pending = append(next.Pending, encodeEntry(entry))
		}
	}
	exported := "an unknown time"
	if !meta.ExportedAt.IsZero() {
		exported = meta.ExportedAt.UTC().Format(time.RFC3339)
	}
	origin := meta.Host
	if origin == "" {
		origin = "another console"
	}
	entry := TurnEntry{About: "ws:" + string(id), Kind: "observation", OccurredAt: now.UTC(),
		Key:      fmt.Sprintf("axlr:import:%s:%s", id, now.UTC().Format(time.RFC3339)),
		Summary:  fmt.Sprintf("Session imported from %s (exported %s) and continued on %s.", origin, exported, host),
		Evidence: []string{"axlr-tui --import " + source},
		Labels:   map[string][]string{"ws": {string(workspace)}, "session": {string(id)}, "event": {"import"}}}
	if keep := domain.MaxPendingEntries - 1; len(next.Pending) > keep {
		// The handoff makes one more: a full queue would fail Validate and
		// the import with it. The oldest entries go, as the journal drops
		// them.
		next.Pending = next.Pending[len(next.Pending)-keep:]
	}
	next.Pending = append(next.Pending, encodeEntry(entry))
	if next.Instance == "" {
		// The origin had no instance: the record needs a reason to exist
		// until the first prompt opens one here and takes the entry.
		next.Unavailable = "imported from " + origin + " without a ceremony instance"
	}
	return &next
}
