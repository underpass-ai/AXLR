# Work Modes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the user switch AXLR into a working mode (`/review`, `/writer`, `/research`, back with `/normal`). Each mode changes the model guidance, which tools the host lets the model use, and the status line.

**Architecture:** The mode is a domain value carried by the session. It is kept outside the versioned snapshot in an `<id>.mode` sidecar, so older consoles still read every session. The application layer enforces it in three places: the tools sent to the model, the existing rejection choke point (`rejectUnknown`) for denied calls, and automatic approval for calls that need a human. A mode is a policy, not a MADE ceremony; ceremony modes (`/delivery`, `/debug`, `/resume`) come in a separate plan that builds on this one.

**Tech Stack:** Go (module `github.com/underpass-ai/AXLR/tui` in a `go.work`), Bubble Tea v2 TUI, no new dependencies.

**Spec:** the ceremony review published at https://claude.ai/artifact/ScnDk7CV34Lt5e1iCUjNeH and Tirso's decisions of 1 Oct 2026: the host drives MADE; the catalogue keeps only delivery/debug 2.0; review and research become modes; all modes ship in the first round.

## Global Constraints

- No new dependencies.
- The session snapshot format (`tui/dto/session_snapshot.go`, `snapshotVersion = 2`, decoded with `DisallowUnknownFields`) must not change. Session metadata lives in sibling files, as `<id>.times` does.
- Every user-facing string exists in both `enMessages` and `esMessages` (`tui/adapters/terminal/i18n.go`); `TestCatalogsHaveTheSameLabeledMessages` and `TestLiteralTranslationLabelsExist` must pass.
- Mode identifiers: `normal`, `review`, `writer`, `research`. Commands: `/normal`, `/review`, `/writer`, `/research`; Spanish aliases `/revisar`, `/escritor`, `/investigar`.
- Document paths that writer and research modes may write: extensions `.md`, `.mdx`, `.txt`, `.rst`, or anything under `docs/`.
- A mode can change only between turns: never while streaming, awaiting approval, or with pending calls.
- Run all tests with `cd tui && go test ./...`; also `go vet ./...` and `gofmt -l .` (must print nothing).
- One branch and one PR for this plan, from `origin/main`. Tirso merges.

## File Map

| File | Responsibility |
|:--|:--|
| `tui/domain/work_mode.go` (new) | `WorkMode` value, parsing, validation |
| `tui/domain/work_mode_policy.go` (new) | `ModeVerdict` and `WorkMode.Judge`: what a mode allows for one tool call |
| `tui/domain/session_state.go`, `session.go` | `SessionState.Mode`, `Session.Mode()`, `Session.SetMode`, restore validation |
| `tui/adapters/storage/session_mode.go` (new) | `<id>.mode` sidecar read/write |
| `tui/adapters/storage/session_sidecar.go` (new) | atomic sidecar write shared by times and mode |
| `tui/adapters/storage/session_times.go`, `session_store.go`, `session_mapper.go` | use the shared writer; read and apply the mode |
| `tui/application/mode_denial.go` (new) | `ModeDenial` error with a clean model-visible message |
| `tui/application/agent_turn_use_case.go`, `resolve_tool_use_case.go`, `unknown_tool_rejection.go`, `automatic_tool_policy.go` | enforcement |
| `tui/application/host_tool_definitions.go`, `continue_turn_use_case.go`, `model_context.go` | tools sent and guidance per mode |
| `tui/adapters/terminal/slash_commands.go`, `app_model.go`, `footer.go`, `i18n.go`, `work_mode.go` (new) | commands, switch, badge, strings |

---

### Task 1: The mode and its tool policy (domain)

**Files:**
- Create: `tui/domain/work_mode.go`
- Create: `tui/domain/work_mode_policy.go`
- Test: `tui/domain/work_mode_test.go`

**Interfaces:**
- Produces:
  - `type WorkMode string`; constants `ModeNormal`, `ModeReview`, `ModeWriter`, `ModeResearch`
  - `func ParseWorkMode(raw string) (WorkMode, error)`; `func (m WorkMode) Validate() error`
  - `type ModeVerdict int`; constants `VerdictAllow`, `VerdictAsk`, `VerdictDeny`
  - `func (m WorkMode) Judge(id ToolIdentity, arguments axlr.JSONValue) (ModeVerdict, string)`
  - `func (m WorkMode) HidesWriteTools() bool`

- [ ] **Step 1: Write the failing test**

```go
package domain

import (
	"testing"

	axlr "github.com/underpass-ai/AXLR/domain"
)

func jsonArgs(t *testing.T, raw string) axlr.JSONValue {
	t.Helper()
	value, err := axlr.NewJSONObject([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func localTool(t *testing.T, operation string) ToolIdentity {
	t.Helper()
	id, err := NewLocalToolIdentity(operation)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestParseWorkModeAcceptsOnlyKnownModes(t *testing.T) {
	for _, raw := range []string{"normal", "review", "writer", "research"} {
		if mode, err := ParseWorkMode(raw); err != nil || string(mode) != raw {
			t.Fatalf("%s: %v", raw, err)
		}
	}
	for _, raw := range []string{"", "Writer", "debug", "review "} {
		if _, err := ParseWorkMode(raw); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
}

func TestModesJudgeWorkspaceChanges(t *testing.T) {
	write, edit := localTool(t, "write"), localTool(t, "edit")
	exec, read := localTool(t, "exec"), localTool(t, "read")
	plugin, _ := NewPluginToolIdentity(axlr.PluginRef{PluginID: "kmp", ToolName: "kmp_write_memory"})
	doc := jsonArgs(t, `{"path":"docs/guide.go","content":"x"}`)
	readme := jsonArgs(t, `{"path":"README.md","content":"x"}`)
	code := jsonArgs(t, `{"path":"wc.py","content":"x"}`)
	sneaky := jsonArgs(t, `{"path":"docs/../wc.py","content":"x"}`)
	cases := []struct {
		mode WorkMode
		id   ToolIdentity
		args axlr.JSONValue
		want ModeVerdict
	}{
		{ModeNormal, write, code, VerdictAllow},
		{ModeNormal, exec, jsonArgs(t, `{}`), VerdictAllow},
		{ModeReview, write, readme, VerdictDeny},
		{ModeReview, edit, readme, VerdictDeny},
		{ModeReview, exec, jsonArgs(t, `{}`), VerdictAsk},
		{ModeReview, read, code, VerdictAllow},
		{ModeReview, plugin, jsonArgs(t, `{}`), VerdictAllow},
		{ModeWriter, write, readme, VerdictAllow},
		{ModeWriter, edit, doc, VerdictAllow},
		{ModeWriter, write, code, VerdictDeny},
		{ModeWriter, write, sneaky, VerdictDeny},
		{ModeWriter, exec, jsonArgs(t, `{}`), VerdictAsk},
		{ModeResearch, write, readme, VerdictAllow},
		{ModeResearch, edit, code, VerdictDeny},
		{ModeResearch, exec, jsonArgs(t, `{}`), VerdictAsk},
	}
	for _, c := range cases {
		got, why := c.mode.Judge(c.id, c.args)
		if got != c.want {
			t.Fatalf("%s %s %s: got %d want %d (%s)", c.mode, c.id.LocalOperation, c.args.Bytes(), got, c.want, why)
		}
		if got == VerdictDeny && why == "" {
			t.Fatalf("%s denial has no reason", c.mode)
		}
	}
}

func TestOnlyReviewHidesWriteTools(t *testing.T) {
	if !ModeReview.HidesWriteTools() || ModeWriter.HidesWriteTools() || ModeNormal.HidesWriteTools() || ModeResearch.HidesWriteTools() {
		t.Fatal("unexpected write-tool visibility")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd tui && go test ./domain -run 'WorkMode|ModesJudge|HidesWrite'`
Expected: FAIL to compile with `undefined: ParseWorkMode`.

- [ ] **Step 3: Write minimal implementation**

`tui/domain/work_mode.go`:

```go
package domain

import "errors"

// WorkMode is how AXLR works in a session: a policy over guidance and tools,
// not a ceremony. The zero value reads as ModeNormal.
type WorkMode string

const (
	ModeNormal   WorkMode = "normal"
	ModeReview   WorkMode = "review"
	ModeWriter   WorkMode = "writer"
	ModeResearch WorkMode = "research"
)

func ParseWorkMode(raw string) (WorkMode, error) {
	mode := WorkMode(raw)
	return mode, mode.Validate()
}

func (m WorkMode) Validate() error {
	switch m {
	case ModeNormal, ModeReview, ModeWriter, ModeResearch:
		return nil
	}
	return errors.New("unknown work mode")
}
```

`tui/domain/work_mode_policy.go`:

```go
package domain

import (
	"encoding/json"
	"path"
	"path/filepath"
	"strings"

	axlr "github.com/underpass-ai/AXLR/domain"
)

// ModeVerdict is what a work mode allows for one resolved tool call.
type ModeVerdict int

const (
	VerdictAllow ModeVerdict = iota
	// VerdictAsk requires the user's decision even when autonomy is on.
	VerdictAsk
	VerdictDeny
)

var documentExtensions = map[string]bool{".md": true, ".mdx": true, ".txt": true, ".rst": true}

// Judge applies the mode to a resolved call. Plugin and host tools are not
// workspace changes and stay under their own approval policy.
func (m WorkMode) Judge(id ToolIdentity, arguments axlr.JSONValue) (ModeVerdict, string) {
	if m == "" || m == ModeNormal || id.Kind != ToolKindLocal {
		return VerdictAllow, ""
	}
	switch id.LocalOperation {
	case "exec":
		return VerdictAsk, ""
	case "write", "edit":
		if m == ModeReview {
			return VerdictDeny, "review mode does not change files"
		}
		var target struct {
			Path string `json:"path"`
		}
		if json.Unmarshal(arguments.Bytes(), &target) != nil || !documentPath(target.Path) {
			return VerdictDeny, string(m) + " mode writes only documents: .md, .mdx, .txt, .rst or files under docs/"
		}
	}
	return VerdictAllow, ""
}

// HidesWriteTools reports modes whose model never sees write tools at all.
func (m WorkMode) HidesWriteTools() bool { return m == ModeReview }

func documentPath(raw string) bool {
	relative, err := axlr.NewRelativePath(raw)
	if err != nil {
		return false
	}
	clean := path.Clean(filepath.ToSlash(string(relative)))
	if strings.HasPrefix(clean, "../") || clean == ".." {
		return false
	}
	return documentExtensions[strings.ToLower(path.Ext(clean))] || strings.HasPrefix(clean, "docs/")
}
```

If `axlr.NewRelativePath` already rejects `docs/../wc.py`, the `path.Clean` check is redundant but harmless. If it accepts it, `path.Clean` turns it into `wc.py`, which then fails both document checks. Either way the `sneaky` case is denied.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd tui && go test ./domain -run 'WorkMode|ModesJudge|HidesWrite'`
Expected: PASS. The root-module constructors are `axlr.NewJSONObject(raw []byte)` and `axlr.NewRelativePath(s string)`.

- [ ] **Step 5: Commit**

```bash
git add tui/domain/work_mode.go tui/domain/work_mode_policy.go tui/domain/work_mode_test.go
git commit -m "Add work modes and their tool policy"
```

---

### Task 2: The session carries its mode (domain)

**Files:**
- Modify: `tui/domain/session_state.go` (struct `SessionState`, `restoreSession`)
- Modify: `tui/domain/session.go` (new methods after `ChangeModel`)
- Test: `tui/domain/session_mode_test.go`

**Interfaces:**
- Consumes: `WorkMode`, `ModeNormal`, `Validate` (Task 1)
- Produces: `SessionState.Mode WorkMode`; `func (s Session) Mode() WorkMode`; `func (s *Session) SetMode(m WorkMode) error`

- [ ] **Step 1: Write the failing test**

```go
package domain

import (
	"testing"

	axlr "github.com/underpass-ai/AXLR/domain"
)

func idleSession(t *testing.T) Session {
	t.Helper()
	s, err := NewSession("0123456789abcdef0123456789abcdef", Workspace(t.TempDir()), "test/model")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSessionModeDefaultsToNormalAndSurvivesRestore(t *testing.T) {
	s := idleSession(t)
	if s.Mode() != ModeNormal {
		t.Fatalf("new session mode %q", s.Mode())
	}
	if err := s.SetMode(ModeWriter); err != nil {
		t.Fatal(err)
	}
	restored, err := RestoreSession(s.Export())
	if err != nil || restored.Mode() != ModeWriter {
		t.Fatalf("%v %q", err, restored.Mode())
	}
	state := s.Export()
	state.Mode = "debug"
	if _, err := RestoreSession(state); err == nil {
		t.Fatal("restored an unknown mode")
	}
}

func TestSessionModeChangesOnlyBetweenTurns(t *testing.T) {
	s := idleSession(t)
	if err := s.SetMode("loud"); err == nil {
		t.Fatal("accepted an unknown mode")
	}
	if err := s.BeginTurn(axlr.Text("hola"), nil); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMode(ModeReview); err == nil {
		t.Fatal("mode changed during a streaming turn")
	}
}
```

Session IDs are 32 lowercase hex characters. If `BeginTurn` with a nil tool list is rejected, pass the tool list those tests use.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd tui && go test ./domain -run SessionMode`
Expected: FAIL to compile with `s.Mode undefined`.

- [ ] **Step 3: Write minimal implementation**

In `tui/domain/session_state.go`, add the field after `MessageTimes`:

```go
	// Mode is the session's work mode; empty reads as ModeNormal. It is kept
	// outside the snapshot so the snapshot format does not change.
	Mode WorkMode
```

In `restoreSession`, directly after the `axlr.NewText(string(state.Draft))` check, add:

```go
	if state.Mode != "" {
		if err = state.Mode.Validate(); err != nil {
			return Session{}, err
		}
	}
```

In `tui/domain/session.go`, after `ChangeModel`:

```go
func (s Session) Mode() WorkMode {
	if s.state.Mode == "" {
		return ModeNormal
	}
	return s.state.Mode
}

// SetMode changes how the next turn works. A turn in progress keeps the mode
// it started with.
func (s *Session) SetMode(mode WorkMode) error {
	if err := mode.Validate(); err != nil {
		return err
	}
	if s.Status() != StatusIdle && s.Status() != StatusComplete && s.Status() != StatusInterrupted {
		return errors.New("cannot change mode while a turn is active")
	}
	if len(s.Pending()) != 0 {
		return errors.New("pending calls must be resolved before changing mode")
	}
	s.state.Mode = mode
	return nil
}
```

`cloneState` copies the field already, because it is a value.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd tui && go test ./domain`
Expected: PASS, including existing tests.

- [ ] **Step 5: Commit**

```bash
git add tui/domain/session_state.go tui/domain/session.go tui/domain/session_mode_test.go
git commit -m "Carry the work mode on the session"
```

---

### Task 3: Persist the mode beside the snapshot (storage)

**Files:**
- Create: `tui/adapters/storage/session_sidecar.go`
- Create: `tui/adapters/storage/session_mode.go`
- Modify: `tui/adapters/storage/session_times.go` (`writeTimes` uses the shared writer)
- Modify: `tui/adapters/storage/session_store.go` (`Save` writes the mode; `read` passes it)
- Modify: `tui/adapters/storage/session_mapper.go` (`restoreSnapshot` takes the mode)
- Test: `tui/adapters/storage/session_mode_test.go`

**Interfaces:**
- Consumes: `SessionState.Mode`, `Session.Mode()`, `Session.SetMode` (Task 2)
- Produces: `func (s *SessionStore) readMode(id domain.SessionID) domain.WorkMode`; `func (s *SessionStore) writeMode(id domain.SessionID, mode domain.WorkMode) error`; `restoreSnapshot(d dto.SessionSnapshot, preserveActive bool, times []time.Time, mode domain.WorkMode)`

- [ ] **Step 1: Write the failing test**

```go
package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestSessionModeIsKeptBesideTheSnapshot(t *testing.T) {
	store, dir := openStore(t)
	const id = "0123456789abcdef0123456789abcdef"
	s, err := domain.NewSession(id, domain.Workspace(t.TempDir()), "test/model")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetMode(domain.ModeWriter); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	snapshot, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if err != nil || strings.Contains(string(snapshot), "writer") {
		t.Fatalf("mode leaked into the snapshot: %s %v", snapshot, err)
	}
	loaded, err := store.Load(context.Background(), id)
	if err != nil || loaded.Mode() != domain.ModeWriter {
		t.Fatalf("%v %q", err, loaded.Mode())
	}
	if err := loaded.SetMode(domain.ModeNormal); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), loaded); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, id+".mode")); !os.IsNotExist(err) {
		t.Fatal("normal mode left a sidecar behind")
	}
}

func TestUnreadableModeSidecarLoadsAsNormal(t *testing.T) {
	store, dir := openStore(t)
	const id = "fedcba9876543210fedcba9876543210"
	s, _ := domain.NewSession(id, domain.Workspace(t.TempDir()), "test/model")
	if err := store.Save(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, id+".mode"), []byte(`{"version":1,"mode":"loud"}`), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(context.Background(), id)
	if err != nil || loaded.Mode() != domain.ModeNormal {
		t.Fatalf("%v %q", err, loaded.Mode())
	}
}
```

`openStore(t)` is the existing helper in `session_store_test.go`; it returns the store and its directory.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd tui && go test ./adapters/storage -run Mode`
Expected: FAIL. The loaded mode is `normal` instead of `writer`.

- [ ] **Step 3: Write minimal implementation**

`tui/adapters/storage/session_sidecar.go`. This is the temp-file block from `writeTimes`, moved:

```go
package storage

import "os"

// writeSidecar atomically replaces one private metadata file beside a snapshot.
func (s *SessionStore) writeSidecar(pattern, target string, data []byte) error {
	file, err := os.CreateTemp(s.dir, pattern)
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return s.replace(file.Name(), target)
}
```

In `session_times.go`, replace everything in `writeTimes` from `file, err := os.CreateTemp(s.dir, ".times-*")` to the final `return s.replace(...)` with:

```go
	return s.writeSidecar(".times-*", s.timesPath(id), data)
```

`tui/adapters/storage/session_mode.go`:

```go
package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/underpass-ai/AXLR/tui/domain"
)

const sessionModeVersion = 1
const maxSessionModeBytes = 4 << 10

// sessionMode is kept in <id>.mode for the same reason as <id>.times: the
// snapshot rejects unknown fields, and older consoles must keep loading.
type sessionMode struct {
	Version int    `json:"version"`
	Mode    string `json:"mode"`
}

func (s *SessionStore) modePath(id domain.SessionID) string {
	return filepath.Join(s.dir, string(id)+".mode")
}

// readMode returns ModeNormal when the file is missing or unreadable: a mode
// is a preference and never blocks loading a session.
func (s *SessionStore) readMode(id domain.SessionID) domain.WorkMode {
	file, err := openNoFollow(s.modePath(id), os.O_RDONLY, 0)
	if err != nil {
		return domain.ModeNormal
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxSessionModeBytes+1))
	if err != nil || len(data) > maxSessionModeBytes {
		return domain.ModeNormal
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var record sessionMode
	if decoder.Decode(&record) != nil || record.Version != sessionModeVersion {
		return domain.ModeNormal
	}
	mode, err := domain.ParseWorkMode(record.Mode)
	if err != nil {
		return domain.ModeNormal
	}
	return mode
}

func (s *SessionStore) writeMode(id domain.SessionID, mode domain.WorkMode) error {
	if mode == "" || mode == domain.ModeNormal {
		if err := os.Remove(s.modePath(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	data, err := json.Marshal(sessionMode{Version: sessionModeVersion, Mode: string(mode)})
	if err != nil {
		return err
	}
	return s.writeSidecar(".mode-*", s.modePath(id), data)
}
```

In `session_store.go` `Save`, after the `writeTimes` call:

```go
	if e = s.writeMode(session.Export().ID, session.Mode()); e != nil {
		return e
	}
```

In `read`, change the last line to:

```go
	return restoreSnapshot(record, s.preserveActive, s.readTimes(id), s.readMode(id))
```

In `session_mapper.go`, change the signature to `restoreSnapshot(d dto.SessionSnapshot, preserveActive bool, times []time.Time, mode domain.WorkMode)`. Set `s.Mode = mode` next to each `s.MessageTimes = times`, and change `restore` to `return restoreSnapshot(d, false, nil, domain.ModeNormal)`. Run `git grep -n "restoreSnapshot(" tui` and update every caller.

If the store deletes sibling files when a session is deleted or archived, delete `.mode` in the same place: `grep -n '".times"\|timesPath' tui/adapters/storage/*.go`.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd tui && go test ./adapters/storage`
Expected: PASS, including the existing times tests, which now go through `writeSidecar`.

- [ ] **Step 5: Commit**

```bash
git add tui/adapters/storage
git commit -m "Keep each session's work mode in an <id>.mode sidecar"
```

---

### Task 4: Enforce the mode in the agent loop (application)

**Files:**
- Create: `tui/application/mode_denial.go`
- Modify: `tui/application/unknown_tool_rejection.go` (reason text)
- Modify: `tui/application/agent_turn_use_case.go` (`rejectUnknown`, auto-approval check)
- Modify: `tui/application/resolve_tool_use_case.go` (`resolveOne` after `ResolveToolCall`)
- Modify: `tui/application/automatic_tool_policy.go`
- Modify: `tui/application/host_tool_definitions.go` (`ModeTools`)
- Modify: `tui/application/continue_turn_use_case.go:75` (use `ModeTools`)
- Modify: `tui/application/model_context.go` (mode guidance)
- Test: `tui/application/work_mode_test.go`

**Interfaces:**
- Consumes: `Session.Mode()`, `WorkMode.Judge`, `VerdictAsk`, `VerdictDeny`, `WorkMode.HidesWriteTools` (Tasks 1–2)
- Produces:
  - `type ModeDenial struct{ Mode domain.WorkMode; Reason string }` (implements `error`)
  - `func ModeTools(mode domain.WorkMode, snapshot []domain.AvailableTool) []root.ToolDefinition`
  - `func approvesInMode(policy ToolApprovalPolicyPort, mode domain.WorkMode, id domain.ToolIdentity, arguments root.JSONValue) bool`

- [ ] **Step 1: Write the failing test**

Reuse the helpers the package tests already have. `turnSession(t)` and `hostPlugin(...)` exist in `model_context_test.go` and `host_tool_resolution_test.go`. Find a helper that builds a session with one pending local call and a store: `grep -n "func .*pending\|memoryStore\|fakeStore" tui/application/*_test.go`. The test below assumes two helpers you write in this file if none fit: `pendingCall(t, mode, name, args)`, which returns a session in `StatusApproval` with that single call pending and its tool in the snapshot, and an in-memory `SessionStorePort`.

```go
package application

import (
	"context"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type allowEverything struct{}

func (allowEverything) AutoApproves(domain.ToolIdentity) bool { return true }

func TestReviewModeRejectsWritesWithAModelVisibleReason(t *testing.T) {
	s, store := pendingCall(t, domain.ModeReview, "local_write", `{"path":"wc.py","content":"x"}`)
	if err := rejectUnknown(context.Background(), &s, store, nil, nil); err != nil {
		t.Fatal(err)
	}
	activity := s.Export().Activity
	outcome := activity[len(activity)-1].Outcome
	if outcome == nil || !outcome.IsError || !strings.HasPrefix(string(outcome.Content), "denied by review mode: ") {
		t.Fatalf("write was not refused by mode: %+v", outcome)
	}
}

func TestWriterModeLetsDocumentsThrough(t *testing.T) {
	s, store := pendingCall(t, domain.ModeWriter, "local_write", `{"path":"docs/usage.md","content":"x"}`)
	if err := rejectUnknown(context.Background(), &s, store, nil, nil); err != nil {
		t.Fatal(err)
	}
	if len(s.Pending()) != 1 {
		t.Fatal("a document write was refused")
	}
}

func TestModesKeepExecUnderHumanApprovalEvenWithAutonomy(t *testing.T) {
	exec, _ := domain.NewLocalToolIdentity("exec")
	args, _ := root.NewJSONObject([]byte(`{"program":"ls"}`))
	if !approvesInMode(allowEverything{}, domain.ModeNormal, exec, args) {
		t.Fatal("normal mode lost autonomy")
	}
	for _, mode := range []domain.WorkMode{domain.ModeReview, domain.ModeWriter, domain.ModeResearch} {
		if approvesInMode(allowEverything{}, mode, exec, args) {
			t.Fatalf("%s auto-approved exec", mode)
		}
	}
}

func TestReviewModeHidesWriteToolsFromTheModel(t *testing.T) {
	snapshot := HostTools()
	for _, op := range []string{"read", "write", "edit", "exec"} {
		id, _ := domain.NewLocalToolIdentity(op)
		snapshot = append(snapshot, domain.AvailableTool{Definition: root.ToolDefinition{Name: root.ToolName("local_" + op), Description: "x", Parameters: mustObject(t, `{"type":"object"}`)}, Identity: id})
	}
	names := func(mode domain.WorkMode) string {
		var out []string
		for _, tool := range ModeTools(mode, snapshot) {
			out = append(out, string(tool.Name))
		}
		return strings.Join(out, ",")
	}
	if strings.Contains(names(domain.ModeReview), "local_write") || strings.Contains(names(domain.ModeReview), "local_edit") {
		t.Fatal("review mode exposes write tools")
	}
	if !strings.Contains(names(domain.ModeWriter), "local_write") || !strings.Contains(names(domain.ModeReview), "local_read") {
		t.Fatal("mode removed the wrong tools")
	}
}

func TestGuidanceDescribesTheActiveMode(t *testing.T) {
	s := turnSession(t)
	if err := s.SetMode(domain.ModeWriter); err != nil {
		t.Fatal(err)
	}
	text := string(modelHostGuidance(&s).Content)
	if !strings.Contains(text, "Mode: writer.") || strings.Contains(text, "axlr-ceremonies") {
		t.Fatalf("writer guidance missing or still routing to ceremonies: %s", text)
	}
}

func mustObject(t *testing.T, raw string) root.JSONValue {
	t.Helper()
	value, err := root.NewJSONObject([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return value
}
```

`turnSession(t)` (in `start_turn_use_case_test.go`) returns an idle session, so `SetMode` works on it.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd tui && go test ./application -run 'Mode|Writer|Review'`
Expected: FAIL to compile with `undefined: approvesInMode`, `ModeTools`.

- [ ] **Step 3: Write minimal implementation**

`tui/application/mode_denial.go`:

```go
package application

import "github.com/underpass-ai/AXLR/tui/domain"

// ModeDenial is a call the session's work mode refuses. Its text reaches the
// model as the tool result so it can change course.
type ModeDenial struct {
	Mode   domain.WorkMode
	Reason string
}

func (d ModeDenial) Error() string { return "denied by " + string(d.Mode) + " mode: " + d.Reason }
```

`unknown_tool_rejection.go`. Replace the `if len(invalid) > 0 && invalid[0] != nil {` block with:

```go
	if len(invalid) > 0 && invalid[0] != nil {
		reason = "invalid tool invocation rejected: " + invalid[0].Error()
		var denial ModeDenial
		if errors.As(invalid[0], &denial) {
			reason = denial.Error()
		}
		if len(reason) > 1024 {
			reason = reason[:1024]
		}
	}
```

`agent_turn_use_case.go` `rejectUnknown` loop body becomes:

```go
		p := s.Pending()[0]
		tool, args, known, resolveErr := ResolveToolCall(s.ToolSnapshot(), p.Call)
		if known && resolveErr == nil {
			verdict, reason := s.Mode().Judge(tool.Identity, args)
			if verdict != domain.VerdictDeny {
				return nil
			}
			resolveErr = ModeDenial{Mode: s.Mode(), Reason: reason}
		}
		if err := rejectUnknownCall(ctx, s, store, trace, p, resolveErr); err != nil {
			return err
		}
		if err := emitTool(s, p.Call.ID, emit); err != nil {
			return err
		}
```

In `AgentTurnUseCase.Execute`, change `tool, _, known, resolveErr := ResolveToolCall(...)` to `tool, args, known, resolveErr := ...`, and change `automaticallyApproves(u.Approval, tool.Identity)` to `approvesInMode(u.Approval, s.Mode(), tool.Identity, args)`.

`resolve_tool_use_case.go` `resolveOne`. Directly after the `if !known || resolveErr != nil { return rejectUnknown(...) }` block, insert:

```go
	if verdict, _ := s.Mode().Judge(tool.Identity, toolArgs); verdict == domain.VerdictDeny {
		return rejectUnknown(ctx, s, u.Store, emit, u.Diagnostics)
	}
```

Change `!automaticallyApproves(u.Approval, tool.Identity)` to `!approvesInMode(u.Approval, s.Mode(), tool.Identity, toolArgs)`.

`automatic_tool_policy.go`. Append:

```go
// approvesInMode adds the work mode: a call the mode puts under the user's
// decision is never approved automatically, autonomy included.
func approvesInMode(policy ToolApprovalPolicyPort, mode domain.WorkMode, id domain.ToolIdentity, arguments root.JSONValue) bool {
	if verdict, _ := mode.Judge(id, arguments); verdict != domain.VerdictAllow {
		return false
	}
	return automaticallyApproves(policy, id)
}
```

Add the `root "github.com/underpass-ai/AXLR/domain"` import.

`host_tool_definitions.go`. Append:

```go
// ModeTools is the model's tool surface under a work mode. Hidden tools are
// also refused by the mode if a model calls them from memory.
func ModeTools(mode domain.WorkMode, snapshot []domain.AvailableTool) []root.ToolDefinition {
	tools := ModelTools(snapshot)
	if !mode.HidesWriteTools() {
		return tools
	}
	out := tools[:0:0]
	for _, tool := range tools {
		if tool.Name != "local_write" && tool.Name != "local_edit" {
			out = append(out, tool)
		}
	}
	return out
}
```

`continue_turn_use_case.go:75`. Change `Tools: ModelTools(snapshot)` to `Tools: ModeTools(session.Mode(), snapshot)`.

`model_context.go`. In `modelHostGuidance`, write the ceremony paragraph (the second `WriteString`) only when `s.Mode() == domain.ModeNormal`. Then, after the fourth fixed paragraph, add:

```go
	if text, ok := modeGuidance[s.Mode()]; ok {
		guidance.WriteString(text)
	}
```

At file level:

```go
var modeGuidance = map[domain.WorkMode]string{
	domain.ModeReview: "Mode: review. You review; you do not change the workspace, and the host refuses file writes. Every local_exec needs the user's approval, so run only the checks that matter. Report findings by severity, each with location, a concrete failure scenario, impact and evidence; separate blockers, suggestions and uncertainties. Use no ceremony.\n",
	domain.ModeWriter: "Mode: writer. You write documents: plans, READMEs, release notes, articles and notes. The host lets you write or edit only .md, .mdx, .txt and .rst files or files under docs/, and every local_exec needs approval. Work in order: settle the brief (audience, purpose, length) unless the user gave it, outline, draft, critique the draft against the brief, then revise at most twice. When KMP is connected, recall the project's voice, glossary and earlier style decisions before drafting, and record a new style decision with its reason once the user accepts it. Use no ceremony.\n",
	domain.ModeResearch: "Mode: research. You answer a question with evidence and leave a decision behind. When KMP is connected, ask project memory first with kmp_ask and say what it already knows. Then read primary sources: repository files, and public pages through local_exec with curl, which needs approval. Attribute every claim to its source and separate observation from inference. Deliver a recommendation with alternatives, confidence and open questions, as a Markdown file when the user wants a document; the host lets you write only documents. When KMP is connected, record the decision with its sources and why. Use no ceremony.\n",
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd tui && go test ./application`
Expected: PASS, including existing tests. `TestModelHostGuidanceBoundsManifestAndKeepsDiscovery` must still be under 13 KiB.

- [ ] **Step 5: Commit**

```bash
git add tui/application
git commit -m "Enforce the work mode on tools, approval and guidance"
```

---

### Task 5: Switch modes from the composer and show the badge (TUI)

**Files:**
- Create: `tui/adapters/terminal/work_mode.go`
- Modify: `tui/adapters/terminal/slash_commands.go` (commands and aliases)
- Modify: `tui/adapters/terminal/app_model.go` (send handler, next to `/autonomy`)
- Modify: `tui/adapters/terminal/footer.go` (`footerStatus`)
- Modify: `tui/adapters/terminal/i18n.go` (en and es)
- Test: `tui/adapters/terminal/work_mode_test.go`

**Interfaces:**
- Consumes: `Session.SetMode`, `Session.Mode()`, `SessionState.Mode`, `domain.ParseWorkMode` (Tasks 1–3); `m.deps.Session *domain.Session`; `m.deps.Store application.SessionStorePort`
- Produces: `func (m AppModel) switchMode(mode domain.WorkMode) (AppModel, tea.Cmd)`; `var slashModes map[string]domain.WorkMode`

- [ ] **Step 1: Write the failing test**

`sized()`, `update()` and `ControlIntent("send")` are the helpers used by `engine_updates_test.go`.

```go
package terminal

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func send(t *testing.T, m AppModel, draft string) AppModel {
	t.Helper()
	m.Composer.Input.SetValue(draft)
	next, _ := m.Update(ControlIntent("send"))
	return next.(AppModel)
}

func TestModeCommandsSwitchPersistAndShowABadge(t *testing.T) {
	m := sized()
	defer m.Close()
	m = send(t, m, "/escritor")
	if m.deps.Session.Mode() != domain.ModeWriter || m.Composer.Input.Value() != "" || m.Status.Error != "" {
		t.Fatalf("alias did not switch: %q %q", m.deps.Session.Mode(), m.Status.Error)
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "writer mode") {
		t.Fatal("footer has no mode badge")
	}
	m = send(t, m, "/normal")
	if m.deps.Session.Mode() != domain.ModeNormal || strings.Contains(ansi.Strip(m.View().Content), "mode") {
		t.Fatal("normal mode still shows a badge")
	}
}

func TestModeCannotChangeDuringATurn(t *testing.T) {
	m := sized()
	defer m.Close()
	m.Busy = true
	m = send(t, m, "/review")
	if m.deps.Session.Mode() != domain.ModeNormal || m.Status.Error == "" || m.Composer.Input.Value() != "/review" {
		t.Fatal("mode changed while busy or the draft was lost")
	}
}
```

If `sized()` does not wire `deps.Store`, set it in the test to the in-memory store the other terminal tests use: `grep -n "Store:" tui/adapters/terminal/*_test.go`. The second assertion of the first test checks `"mode"`; if some other footer string contains "mode", narrow it to `"writer mode"`.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd tui && go test ./adapters/terminal -run Mode`
Expected: FAIL. `/escritor` is reported as an unknown command.

- [ ] **Step 3: Write minimal implementation**

`slash_commands.go`. Append to `slashCommands`, before `/exit`:

```go
	{"/normal", "slash.normal"},
	{"/review", "slash.review"},
	{"/writer", "slash.writer"},
	{"/research", "slash.research"},
```

Extend `slashAliases` with `"/revisar": "/review", "/escritor": "/writer", "/investigar": "/research"`.

`tui/adapters/terminal/work_mode.go`:

```go
package terminal

import (
	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// slashModes maps a mode command to the mode it selects.
var slashModes = map[string]domain.WorkMode{
	"/normal": domain.ModeNormal, "/review": domain.ModeReview, "/writer": domain.ModeWriter, "/research": domain.ModeResearch,
}

// switchMode changes how the next turn works and saves it with the session.
// The draft stays when the change is refused, so nothing typed is lost.
func (m AppModel) switchMode(mode domain.WorkMode) (AppModel, tea.Cmd) {
	if m.Busy || m.knownPending() {
		m.Status.Error = m.Theme.T("mode.busy")
		return m, nil
	}
	if m.deps.Session == nil || m.deps.Store == nil {
		m.Status.Error = m.Theme.T("mode.unavailable")
		return m, nil
	}
	next := *m.deps.Session
	if err := next.SetMode(mode); err != nil {
		m.Status.Error = err.Error()
		return m, nil
	}
	if err := m.deps.Store.Save(m.lifetime.ctx, next); err != nil {
		m.Status.Error = err.Error()
		return m, nil
	}
	*m.deps.Session = next
	m.Header.State = next.Export()
	m.Status.Error = ""
	m.Composer.Input.Reset()
	return m, nil
}
```

`app_model.go`. Directly before the `if command == "/autonomy on" ...` block:

```go
			if mode, ok := slashModes[command]; ok {
				return m.switchMode(mode)
			}
```

`footer.go` `footerStatus`. Before `if status.Autonomous {`:

```go
	if mode := m.Header.State.Mode; mode != "" && mode != domain.ModeNormal {
		parts = append(parts, m.Theme.T("mode."+string(mode)))
	}
```

`i18n.go`. Add to `enMessages`:

```go
	"slash.normal":     "Work normally: no mode limits",
	"slash.review":     "Review mode: read and report, no file changes",
	"slash.writer":     "Writer mode: write documents only",
	"slash.research":   "Research mode: memory first, sources, a decision",
	"mode.review":      "review mode",
	"mode.writer":      "writer mode",
	"mode.research":    "research mode",
	"mode.busy":        "Finish or cancel the current task before changing mode",
	"mode.unavailable": "Modes are unavailable in this session",
```

and to `esMessages`:

```go
	"slash.normal":     "Trabajo normal: sin límites de modo",
	"slash.review":     "Modo revisión: leer e informar, sin cambiar ficheros",
	"slash.writer":     "Modo escritor: sólo documentos",
	"slash.research":   "Modo investigación: memoria primero, fuentes y una decisión",
	"mode.review":      "modo revisión",
	"mode.writer":      "modo escritor",
	"mode.research":    "modo investigación",
	"mode.busy":        "Termina o cancela la tarea actual antes de cambiar de modo",
	"mode.unavailable": "Los modos no están disponibles en esta sesión",
```

`"mode."+string(mode)` is not a literal label, so `TestLiteralTranslationLabelsExist` does not see the three badge keys. The parity test covers them.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd tui && go test ./adapters/terminal`
Expected: PASS, including the i18n parity tests and the existing slash-suggestion tests. If `slashSuggestionLimit = 6` breaks a suggestion test because more commands now match `/`, update that test's expected list. Do not raise the limit.

- [ ] **Step 5: Commit**

```bash
git add tui/adapters/terminal
git commit -m "Switch work modes from the composer and show the active mode"
```

---

### Task 6: Verify in the installed console

**Files:** none changed. Evidence goes in the PR description.

- [ ] **Step 1: Full checks**

Run: `cd tui && gofmt -l . && go vet ./... && go test ./...`
Expected: no gofmt output and all packages `ok`.

- [ ] **Step 2: Build and launch in an isolated lab**

Use a lab directory with its own `XDG_CONFIG_HOME` and `XDG_STATE_HOME` (both mode 0700), a copy of `settings.json` with autonomy on, and a toy Git repo with `wc.py` and `test_wc.py`. Launch in tmux with `TERM=xterm-256color COLORTERM=truecolor TMUX=` and the OpenRouter key from `~/.hermes/.env`:

```bash
go build -o "$LAB/axlr-tui" ./cmd/axlr-tui
tmux new-session -d -s modes -x 160 -y 45 "$LAB/run.sh"
```

- [ ] **Step 3: Exercise each mode with the real model and record what happens**

1. `/escritor`. The footer shows `modo escritor`. Ask: «Corrige el bug de wc.py y documenta el uso en docs/usage.md». Expected: the `wc.py` write is refused with `denied by writer mode: …`, `docs/usage.md` is created, and `git diff --stat` touches only `docs/`.
2. `/revisar`. Ask: «Revisa wc.py y arregla lo que veas». Expected: the model offers no write tool, and any `local_exec` shows the approval card although autonomy is on. Close the console, reopen the same session from the session picker, and confirm the footer still says `modo revisión`.
3. `/investigar`. Ask: «¿Deberíamos usar re.findall o str.split para contar palabras?». Expected: with KMP connected it calls `kmp_ask` first, and any document it writes is Markdown.
4. `/normal`. The badge disappears and a code edit goes through again.

- [ ] **Step 4: Commit nothing; open the PR**

Push the branch and open the PR with the commands and observed results from Step 3, including any mode instruction the model ignored, since that feeds the guidance wording.
