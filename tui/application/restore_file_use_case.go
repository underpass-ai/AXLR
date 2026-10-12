package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// RestoreFileUseCase puts files back to the snapshots the session recorded
// of its writes and edits: one file to before one step (Ctrl+D), or every
// file the last turn changed (undo). It reads and writes through the
// session's own local tools, so the workspace root and the sandbox rules
// hold, and it refuses unless every file still holds what the session
// wrote: a file changed since, by the person or another program, is never
// overwritten. Each write is a replace guarded by the digest of that
// content, so a change between the check and the write is refused too. The
// restore is recorded as a console line that names the steps, whose
// snapshots give its diff; the model's history does not change.
type RestoreFileUseCase struct {
	Tools ToolExecutionPort
	Store SessionStorePort
	// Workspace is the root Tools work in; a session of another workspace
	// is refused. Empty checks nothing.
	Workspace domain.Workspace
}

// restoreTarget is one file to put back: it must hold current, the content
// step from wrote, and receives content, what it held before step to.
type restoreTarget struct {
	path     root.RelativePath
	from, to root.ToolCallID
	current  string
	content  string
}

// Restore puts the file step changed back to what it held before the step.
func (u RestoreFileUseCase) Restore(ctx context.Context, session *domain.Session, step root.ToolCallID) (domain.RestoredFile, error) {
	if err := u.ready(session); err != nil {
		return domain.RestoredFile{}, err
	}
	change, reason := session.Export().RecordedChange(step)
	if reason != "" {
		return domain.RestoredFile{}, fmt.Errorf("cannot restore step %s: %s", step, reason)
	}
	if change.Created {
		return domain.RestoredFile{}, fmt.Errorf("cannot restore %s: the step created it, and AXLR does not delete files; remove it yourself", change.Path)
	}
	restored, err := u.apply(ctx, session, false, []restoreTarget{{path: change.Path, from: step, to: step, current: string(change.After), content: string(change.Before)}})
	if err != nil {
		return domain.RestoredFile{}, err
	}
	return restored[0], nil
}

// UndoLastTurn puts every file the last turn changed back to what it held
// before the turn, newest step first. It writes nothing unless every file
// can be restored.
func (u RestoreFileUseCase) UndoLastTurn(ctx context.Context, session *domain.Session) ([]domain.RestoredFile, error) {
	if err := u.ready(session); err != nil {
		return nil, err
	}
	changes := session.Export().LastTurnChanges()
	if len(changes) == 0 {
		return nil, errors.New("the last turn changed no file")
	}
	var order []root.RelativePath
	byPath := map[root.RelativePath][]domain.RestorableChange{}
	for _, c := range changes {
		if _, seen := byPath[c.Change.Path]; !seen {
			order = append(order, c.Change.Path)
		}
		byPath[c.Change.Path] = append(byPath[c.Change.Path], c)
	}
	var targets []restoreTarget
	for _, path := range order {
		steps := byPath[path]
		for i, step := range steps {
			if step.Change.Unavailable != "" {
				return nil, fmt.Errorf("cannot undo the last turn: the snapshot of %s at step %s was not kept (%s)", path, step.Call, step.Change.Unavailable)
			}
			// Newest first: each step must have started from what the
			// step before it wrote, or something else changed the file
			// in between and its content would be lost.
			if i > 0 && steps[i-1].Change.Before != step.Change.After {
				return nil, fmt.Errorf("cannot undo the last turn: %s changed between steps %s and %s", path, step.Call, steps[i-1].Call)
			}
		}
		newest, oldest := steps[0], steps[len(steps)-1]
		if oldest.Change.Created {
			return nil, fmt.Errorf("cannot undo the last turn: it created %s, and AXLR does not delete files; remove it yourself or restore the other files from Ctrl+D", path)
		}
		targets = append(targets, restoreTarget{path: path, from: newest.Call, to: oldest.Call, current: string(newest.Change.After), content: string(oldest.Change.Before)})
	}
	return u.apply(ctx, session, true, targets)
}

func (u RestoreFileUseCase) ready(session *domain.Session) error {
	if session == nil || u.Tools == nil || u.Store == nil {
		return errors.New("restoring a file requires a session, tools and a store")
	}
	state := session.Export()
	if state.Status != domain.StatusIdle && state.Status != domain.StatusComplete && state.Status != domain.StatusInterrupted || len(session.Pending()) != 0 {
		return errors.New("wait for the turn to end before restoring a file")
	}
	if u.Workspace != "" && state.Workspace != u.Workspace {
		return fmt.Errorf("the session belongs to %s; restore its files from a console opened there", state.Workspace)
	}
	return nil
}

// apply checks every target, then writes them and records the restore.
func (u RestoreFileUseCase) apply(ctx context.Context, session *domain.Session, undo bool, targets []restoreTarget) ([]domain.RestoredFile, error) {
	for _, target := range targets {
		if err := u.unchanged(ctx, target); err != nil {
			return nil, err
		}
	}
	var restored []domain.RestoredFile
	var failure error
	for _, target := range targets {
		if err := u.write(ctx, target); err != nil {
			failure = fmt.Errorf("restore %s: %w", target.path, err)
			break
		}
		restored = append(restored, domain.RestoredFile{Path: target.path, From: target.from, To: target.to})
	}
	if len(restored) == 0 {
		return nil, failure
	}
	// What was written is recorded even when a later file failed, so the
	// transcript says which files went back.
	next := *session
	if err := next.AddRestore(domain.FileRestore{Undo: undo, Files: restored}); err != nil {
		return restored, errors.Join(failure, err)
	}
	if err := u.Store.Save(context.WithoutCancel(ctx), next); err != nil {
		return restored, errors.Join(failure, err)
	}
	*session = next
	return restored, failure
}

// unchanged refuses a target whose file no longer holds what the step
// wrote.
func (u RestoreFileUseCase) unchanged(ctx context.Context, target restoreTarget) error {
	output, err := u.run(ctx, "read", map[string]any{"path": string(target.path), "max_bytes": domain.MaxChangePreviewBytes})
	if err != nil {
		return fmt.Errorf("cannot restore %s: %w", target.path, err)
	}
	var read struct {
		Truncated     bool   `json:"truncated"`
		ContentSHA256 string `json:"content_sha256"`
	}
	if json.Unmarshal(output, &read) != nil || read.Truncated || read.ContentSHA256 != string(root.DigestOf([]byte(target.current))) {
		return fmt.Errorf("cannot restore %s: it changed after step %s; nothing was restored", target.path, target.from)
	}
	return nil
}

func (u RestoreFileUseCase) write(ctx context.Context, target restoreTarget) error {
	_, err := u.run(ctx, "write", map[string]any{"path": string(target.path), "content": target.content, "mode": "replace", "expected_sha256": string(root.DigestOf([]byte(target.current)))})
	return err
}

// run executes one local operation and returns its output, or the error
// the runtime reported.
func (u RestoreFileUseCase) run(ctx context.Context, operation string, arguments map[string]any) (json.RawMessage, error) {
	id, err := domain.NewLocalToolIdentity(operation)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(arguments)
	if err != nil {
		return nil, err
	}
	args, err := root.NewJSONObject(raw)
	if err != nil {
		return nil, err
	}
	outcome, err := u.Tools.Execute(ctx, id, args)
	if err != nil {
		return nil, err
	}
	var response struct {
		Status string          `json:"status"`
		Output json.RawMessage `json:"output"`
		Error  *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(outcome.Content), &response) != nil {
		return nil, errors.New("unreadable local_" + operation + " result")
	}
	if outcome.IsError || response.Status != "completed" {
		if response.Error != nil {
			return nil, fmt.Errorf("local_%s %s: %s", operation, response.Error.Code, strings.TrimSpace(response.Error.Message))
		}
		return nil, fmt.Errorf("local_%s %s", operation, response.Status)
	}
	return response.Output, nil
}
