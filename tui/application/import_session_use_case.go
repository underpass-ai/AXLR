package application

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// ImportSessionUseCase saves a bundle's session in this console's store,
// bound to this workspace, with its instance ready to continue here.
type ImportSessionUseCase struct {
	Codec     SessionBundleCodec
	Store     SessionStorePort
	Labels    SessionLabelsPort
	Workspace domain.Workspace
	Host      string
	// Source is the bundle's path, kept as evidence of the handoff.
	Source string
	Now    func() time.Time
}

// Execute imports data and returns the saved session and the bundle's
// metadata. A session already saved here is refused: it is resumed, not
// imported twice.
func (u ImportSessionUseCase) Execute(ctx context.Context, data []byte) (domain.Session, SessionBundleMeta, error) {
	if u.Codec == nil || u.Store == nil {
		return domain.Session{}, SessionBundleMeta{}, errors.New("session import requires a codec and a store")
	}
	session, meta, err := u.Codec.Decode(data)
	if err != nil {
		return domain.Session{}, meta, fmt.Errorf("read the session bundle: %w", err)
	}
	state := session.Export()
	if _, err := u.Store.Load(ctx, state.ID); err == nil {
		return domain.Session{}, meta, fmt.Errorf("session %s is saved in this console already: resume it with --session %s", state.ID, state.ID)
	} else if errors.Is(err, domain.ErrSessionHeld) {
		// On 14 October 2026 this read "may be saved in this console
		// already: session already has a writer: resource temporarily
		// unavailable": the lock's errno, for a session merely open in
		// another console.
		return domain.Session{}, meta, fmt.Errorf("session %s is saved here already and open in another console: continue it there, or close it there and resume it with --session %s", state.ID, state.ID)
	} else if !errors.Is(err, fs.ErrNotExist) {
		// A saved copy that cannot be read, or one another console holds,
		// is not overwritten.
		return domain.Session{}, meta, fmt.Errorf("session %s may be saved in this console already: %w", state.ID, err)
	}
	now := time.Now()
	if u.Now != nil {
		now = u.Now()
	}
	label := domain.SessionLabel{Title: root.Text(meta.Title), About: meta.About}
	if state.Workspace != u.Workspace {
		// Its file changes so far were made there: Ctrl+D says so and
		// does not restore them here.
		label.ImportedFrom, label.ImportedAfter = state.Workspace, len(state.Messages)
	}
	state.Workspace = u.Workspace
	state.Ceremony = nil // a driven ceremony is bound to the origin's orchestrator
	state.Instance = ImportedInstance(state.Instance, state.ID, u.Workspace, meta, u.Host, u.Source, now)
	restored, err := domain.RestoreSession(state)
	if err != nil {
		return domain.Session{}, meta, err
	}
	if err := u.Store.Save(ctx, restored); err != nil {
		return domain.Session{}, meta, err
	}
	if u.Labels != nil && label != (domain.SessionLabel{}) {
		if _, err := u.Labels.Initialize(ctx, state.ID, label); err != nil {
			return domain.Session{}, meta, err
		}
	}
	return restored, meta, nil
}
