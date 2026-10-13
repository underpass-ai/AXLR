package application

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// Sessions travel between machines as bundles (decision I4 of the
// lifecycle integration design): /session export writes one, axlr-tui
// --import reads it and resumes the session here, where its MADE instance
// continues in a successor and its memory records the handoff.

// SessionBundleMeta is what the bundle says about itself.
type SessionBundleMeta struct {
	ExportedAt time.Time
	// Host is the machine that exported it.
	Host  string
	Title string
	// About is the KMP about selected for the session, "" for the default.
	About string
}

// SessionBundleCodec encodes a session with its sidecars and decodes it
// back; the storage adapter owns the format.
type SessionBundleCodec interface {
	Encode(session domain.Session, meta SessionBundleMeta) ([]byte, error)
	Decode(data []byte) (domain.Session, SessionBundleMeta, error)
}

// SessionBundleSuffix ends every bundle's file name.
const SessionBundleSuffix = ".axlr-session.json"

// ExportSessionBundleUseCase writes a session as a bundle under the
// workspace's export directory, named by the session and the time.
type ExportSessionBundleUseCase struct {
	Codec SessionBundleCodec
	Files ExportFilePort
	// Labels gives the title and the about; nil exports neither.
	Labels SessionLabelsPort
	// Sessions loads another session than the open one; nil exports only
	// the open one.
	Sessions SessionLister
	Host     string
	Now      func() time.Time
}

// Execute exports the open session, or the saved one id names, and
// returns the absolute path written.
func (u ExportSessionBundleUseCase) Execute(ctx context.Context, open domain.Session, id domain.SessionID) (string, error) {
	if u.Codec == nil || u.Files == nil {
		return "", errors.New("session export requires a codec and a file port")
	}
	session := open
	if id != "" && id != open.ID() {
		if u.Sessions == nil {
			return "", errors.New("only the open session can be exported in this console")
		}
		loaded, err := u.Sessions.Load(ctx, id)
		if err != nil {
			return "", fmt.Errorf("load session %s: %w", id, err)
		}
		defer func() { _ = u.Sessions.Release(id) }()
		session = loaded
	}
	state := session.Export()
	if state.ID == "" || len(state.Messages) == 0 {
		return "", errors.New("the session has nothing to export yet")
	}
	now := time.Now()
	if u.Now != nil {
		now = u.Now()
	}
	meta := SessionBundleMeta{ExportedAt: now.UTC(), Host: u.Host}
	if u.Labels != nil {
		if labels, err := u.Labels.Load(ctx); err == nil {
			meta.Title, meta.About = string(labels[state.ID].Title), labels[state.ID].About
		}
	}
	data, err := u.Codec.Encode(session, meta)
	if err != nil {
		return "", err
	}
	path := filepath.Join(string(state.Workspace), ExportDirectory, string(state.ID)+"-"+now.Format("20060102-1504")+SessionBundleSuffix)
	if err := u.Files.Create(ctx, path, data); err != nil {
		return "", err
	}
	return path, nil
}

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
	}
	now := time.Now()
	if u.Now != nil {
		now = u.Now()
	}
	state.Workspace = u.Workspace
	state.Ceremony = nil // a driven ceremony is bound to the origin's MADE
	state.Instance = ImportedInstance(state.Instance, state.ID, u.Workspace, meta, u.Host, u.Source, now)
	restored, err := domain.RestoreSession(state)
	if err != nil {
		return domain.Session{}, meta, err
	}
	if err := u.Store.Save(ctx, restored); err != nil {
		return domain.Session{}, meta, err
	}
	if u.Labels != nil && (meta.Title != "" || meta.About != "") {
		if _, err := u.Labels.Initialize(ctx, state.ID, domain.SessionLabel{Title: root.Text(meta.Title), About: meta.About}); err != nil {
			return domain.Session{}, meta, err
		}
	}
	return restored, meta, nil
}

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
	next.Pending = append(next.Pending, encodeEntry(entry))
	if next.Instance == "" {
		// The origin had no instance: the record needs a reason to exist
		// until the first prompt opens one here and takes the entry.
		next.Unavailable = "imported from " + origin + " without a MADE instance"
	}
	return &next
}
