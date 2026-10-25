package service

import (
	"path/filepath"

	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/application"
)

// Sessions is the service's session store, opened before the loop is
// composed: the session's MADE instance, its usage ledger and the successor
// of a capped session write where the API reads, under the owner and
// revision the API checks.
type Sessions struct {
	base    *storage.SessionStore
	wrapped *sessionStore
}

func OpenSessions(cfg Config) (*Sessions, error) {
	if err := secureStateDirectory(cfg.StateDir); err != nil {
		return nil, err
	}
	base, err := storage.NewService(filepath.Join(cfg.StateDir, "sessions"))
	if err != nil {
		return nil, err
	}
	return &Sessions{base: base, wrapped: &sessionStore{next: base}}, nil
}

// Store saves under monotone service revisions.
func (s *Sessions) Store() application.SessionStorePort { return s.wrapped }

// Usage keeps each session's ledger beside its snapshot.
func (s *Sessions) Usage() application.SessionUsagePort { return s.base }

// Receipts keeps each session's receipt journal beside its snapshot.
func (s *Sessions) Receipts() application.SessionReceiptPort { return s.base }

// Lister lists and loads the snapshots for the startup sweep.
func (s *Sessions) Lister() application.SessionLister { return s.base }

// Close closes a store no server took; a server closes its own.
func (s *Sessions) Close() error { return s.base.Close() }
