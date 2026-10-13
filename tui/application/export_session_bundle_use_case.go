package application

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/underpass-ai/AXLR/tui/domain"
)

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
