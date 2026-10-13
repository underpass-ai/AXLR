package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// bundleStore saves sessions and loads them back, as the import checks.
type bundleStore struct {
	saved map[domain.SessionID]domain.Session
}

func (s *bundleStore) List(context.Context) ([]domain.SessionSummary, error) { return nil, nil }
func (s *bundleStore) Save(_ context.Context, session domain.Session) error {
	if s.saved == nil {
		s.saved = map[domain.SessionID]domain.Session{}
	}
	s.saved[session.ID()] = session
	return nil
}
func (s *bundleStore) Load(_ context.Context, id domain.SessionID) (domain.Session, error) {
	if session, ok := s.saved[id]; ok {
		return session, nil
	}
	return domain.Session{}, errors.New("not found")
}

type bundleCodecStub struct {
	encoded []byte
	session domain.Session
	meta    SessionBundleMeta
}

func (c *bundleCodecStub) Encode(session domain.Session, meta SessionBundleMeta) ([]byte, error) {
	c.session, c.meta = session, meta
	return []byte("bundle:" + string(session.ID())), nil
}

func (c *bundleCodecStub) Decode([]byte) (domain.Session, SessionBundleMeta, error) {
	return c.session, c.meta, nil
}

// /session export names the file by the session and the time under the
// workspace's export directory, with the title and the about from the
// labels and the host that exported it.
func TestExportSessionBundleNamesTheFileAndCarriesTheLabels(t *testing.T) {
	s := changedSession(t, []changeStep{{"w1", domain.FileChange{Path: "a.go", Before: "one", After: "two"}}})
	state := s.Export()
	files := exportedFiles{}
	labels := &contextLabels{labels: map[domain.SessionID]domain.SessionLabel{state.ID: {Title: "Parser UTC", About: "project:Repo-B"}}}
	codec := &bundleCodecStub{}
	now := time.Date(2026, 10, 13, 21, 5, 0, 0, time.UTC)
	u := ExportSessionBundleUseCase{Codec: codec, Files: files, Labels: labels, Host: "gx10", Now: func() time.Time { return now }}
	path, err := u.Execute(context.Background(), s, "")
	if err != nil || !strings.HasSuffix(path, "/.axlr/exports/"+string(state.ID)+"-20261013-2105"+SessionBundleSuffix) {
		t.Fatalf("path %q %v", path, err)
	}
	if files[path] != "bundle:"+string(state.ID) || codec.meta != (SessionBundleMeta{ExportedAt: now, Host: "gx10", Title: "Parser UTC", About: "project:Repo-B"}) {
		t.Fatalf("file %q meta %+v", files[path], codec.meta)
	}
	if _, err := u.Execute(context.Background(), s, "ffffffffffffffffffffffffffffffff"); err == nil || !strings.Contains(err.Error(), "only the open session") {
		t.Fatalf("another session without a lister: %v", err)
	}
}

// --import binds the session to this workspace, drops the driven ceremony
// and what bound the instance to the origin, closes the instance so the
// next turn opens its successor, queues the handoff observation, and
// refuses a session saved here already.
func TestImportSessionMakesTheBundleThisConsoles(t *testing.T) {
	s := changedSession(t, []changeStep{{"w1", domain.FileChange{Path: "a.go", Before: "one", After: "two"}}})
	state := s.Export()
	state.Instance = &domain.SessionInstance{Instance: "axlr-session-" + string(state.ID), Turns: 3, Fence: "fence-1", OperationID: "op-1", ClaimedAt: time.Now(), LeaseMS: 1000, Reports: 2, ClaimIncarnation: "inc", KMPAgent: "agent-1", KMPContext: "ctx-1", Recalled: "parser", LastRecord: `{"turn":3}`, Pending: []string{encodeEntry(TurnEntry{About: "ws:" + string(state.ID), Key: "k1", Summary: "pending", Evidence: []string{"e"}, ContextID: "ctx-1"})}}
	origin, err := domain.RestoreSession(state)
	if err != nil {
		t.Fatal(err)
	}
	codec := &bundleCodecStub{session: origin, meta: SessionBundleMeta{ExportedAt: time.Date(2026, 10, 13, 20, 0, 0, 0, time.UTC), Host: "laptop", Title: "Parser UTC", About: "project:Repo-B"}}
	store := &bundleStore{}
	labels := &contextLabels{labels: map[domain.SessionID]domain.SessionLabel{}}
	now := time.Date(2026, 10, 13, 21, 0, 0, 0, time.UTC)
	u := ImportSessionUseCase{Codec: codec, Store: store, Labels: labels, Workspace: domain.Workspace("/elsewhere/Repo-B"), Host: "gx10", Source: "/tmp/b.json", Now: func() time.Time { return now }}
	imported, meta, err := u.Execute(context.Background(), []byte("x"))
	if err != nil || meta.Host != "laptop" {
		t.Fatal(err, meta)
	}
	got := imported.Export()
	instance := got.Instance
	if got.Workspace != "/elsewhere/Repo-B" || got.Ceremony != nil || instance == nil {
		t.Fatalf("state %+v", got)
	}
	if !instance.Closed || instance.Instance != "axlr-session-"+string(state.ID) || instance.Turns != 3 || instance.Fence != "" || instance.OperationID != "" || !instance.ClaimedAt.IsZero() || instance.Reports != 0 || instance.ClaimIncarnation != "" || instance.KMPAgent != "" || instance.KMPContext != "" || instance.LastRecord != `{"turn":3}` {
		t.Fatalf("instance %+v", instance)
	}
	if len(instance.Pending) != 2 {
		t.Fatalf("pending %v", instance.Pending)
	}
	kept, _ := decodeEntry(instance.Pending[0])
	handoff, _ := decodeEntry(instance.Pending[1])
	if kept.ContextID != "" || kept.Summary != "pending" {
		t.Fatalf("kept %+v", kept)
	}
	if handoff.About != "ws:"+string(state.ID) || handoff.Kind != "observation" || handoff.Summary != "Session imported from laptop (exported 2026-10-13T20:00:00Z) and continued on gx10." || handoff.Evidence[0] != "axlr-tui --import /tmp/b.json" || handoff.Labels["event"][0] != "import" {
		t.Fatalf("handoff %+v", handoff)
	}
	if label := labels.labels[state.ID]; string(label.Title) != "Parser UTC" || label.About != "project:Repo-B" {
		t.Fatalf("labels %+v", label)
	}
	if _, _, err := u.Execute(context.Background(), []byte("x")); err == nil || !strings.Contains(err.Error(), "saved in this console already") {
		t.Fatalf("second import: %v", err)
	}
}

func TestRefAboutReadsTheAboutOfARef(t *testing.T) {
	about, err := RefAbout("project:Repo-B:entry:decision:las-fechas-166c94f5031adc46")
	if err != nil || about != "project:Repo-B" {
		t.Fatal(about, err)
	}
	for _, bad := range []string{"", "project:Repo-B", ":entry:x:y"} {
		if _, err := RefAbout(bad); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
}
