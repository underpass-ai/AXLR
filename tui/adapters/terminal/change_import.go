package terminal

import (
	"github.com/underpass-ai/AXLR/tui/domain"
)

// A session imported with --import from another workspace carries the
// file changes it made there. On 14 October 2026 Ctrl+D listed them as
// this workspace's ("2 archivos") and offered to restore them, which then
// failed with the runtime's English refusal. The session's label says where
// it came from and how many messages it had; Ctrl+D marks the changes of
// those messages as made elsewhere and refuses to restore them, and so does
// /undo.

// syncChangeImport reads, once per session, where the session on screen
// was imported from.
func (m *AppModel) syncChangeImport() {
	id := m.Header.State.ID
	if id == m.Changes.importFor {
		return
	}
	var from domain.Workspace
	after := 0
	if m.deps.SessionLabels != nil && id != "" {
		if labels, err := m.deps.SessionLabels.Load(m.lifetime.ctx); err == nil {
			label := labels[id]
			if label.ImportedFrom != "" && label.ImportedFrom != m.Header.State.Workspace {
				from, after = label.ImportedFrom, label.ImportedAfter
			}
		}
	}
	m.Changes.importFor, m.Changes.importedFrom, m.Changes.importedAfter = id, from, after
}

// markForeign marks the records made before the session was imported.
func (v ChangeViewer) markForeign(records []changeRecord) {
	for i := range records {
		records[i].Foreign = v.importedFrom != "" && records[i].Message >= 0 && records[i].Message < v.importedAfter
	}
}

// fileForeign reports a file any of whose steps was made elsewhere.
func (v ChangeViewer) fileForeign(file changeFile) bool {
	for _, step := range file.Steps {
		if v.records[step].Foreign {
			return true
		}
	}
	return false
}

// foreignCalls reports changes /undo would restore that were made in the
// workspace the session was imported from.
func (v ChangeViewer) foreignCalls(changes []domain.RestorableChange) bool {
	for _, change := range changes {
		for _, record := range v.records {
			if record.ID == change.Call && record.Foreign {
				return true
			}
		}
	}
	return false
}
