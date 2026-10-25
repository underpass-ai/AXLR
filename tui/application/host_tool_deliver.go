package application

import (
	"context"
	"errors"
	"fmt"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// HostDeliverName hands a deliverable over to whoever runs the execution
// (the runtime contract's artifacts): the master plan's Phase 1 bots end a
// task with a report the control plane fetches and checks.
const HostDeliverName = domain.DeliverToolName

// DeliverablesPort reads and writes deliverables in a workspace. DeliverFile
// hashes a workspace file delivered as it is, refusing one outside the
// workspace, through a link out of it, not regular or past
// domain.MaxDeliverableFileBytes; WriteDeliverable writes inline content at
// target, a workspace-relative path under domain.DeliverablesDirectory,
// replacing an earlier delivery of the same name.
type DeliverablesPort interface {
	DeliverFile(ctx context.Context, workspace, path string) (DeliveredFile, error)
	WriteDeliverable(ctx context.Context, workspace, target string, content []byte) (DeliveredFile, error)
}

// DeliverTool is axlr_deliver, offered when the console can deliver.
func DeliverTool() domain.AvailableTool {
	identity, _ := domain.NewHostToolIdentity(domain.HostOperationDeliver)
	schema, _ := root.NewJSONObject([]byte(`{"type":"object","properties":{"name":{"type":"string","pattern":"^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$"},"media_type":{"type":"string","pattern":"^[a-z0-9][a-z0-9!#$&^_.+-]{0,62}/[a-z0-9][a-z0-9!#$&^_.+-]{0,62}$"},"description":{"type":"string","minLength":1,"maxLength":300},"path":{"type":"string","minLength":1},"content":{"type":"string","minLength":1,"maxLength":262144}},"required":["name","media_type","description"],"additionalProperties":false}`))
	schema = requiredWhy(schema)
	return domain.AvailableTool{Identity: identity, Definition: root.ToolDefinition{
		Name:        HostDeliverName,
		Description: "Hand over a deliverable of this task (a report, a plan, a diff, a data file) to the person or program running you, once it is final. Give path to deliver a workspace file as it is, or content (UTF-8 text, at most 256 KiB) to have it written to .axlr/deliverables/<session>/<name>; never both. name is its file name, media_type its type (text/markdown, application/json, text/x-diff), description one line on what it is. Delivering a path reads it; delivering content writes a file and is approved as a write. The result names the artifact's reference, size and sha256, not its content.",
		Parameters:  schema,
	}}
}

// deliverArgumentError checks axlr_deliver's arguments before any card, so
// a call that cannot deliver is refused with the reason instead of asked.
func deliverArgumentError(tool domain.AvailableTool, arguments root.JSONValue) error {
	if tool.Definition.Name != HostDeliverName {
		return nil
	}
	if _, err := domain.ParseDeliverable(arguments); err != nil {
		return fmt.Errorf("%s: %w", HostDeliverName, err)
	}
	return nil
}

// hostDeliver delivers the file or writes the content and returns the
// artifact, which the result reads back from this call's outcome.
func (u HostToolUseCase) hostDeliver(ctx context.Context, session domain.Session, arguments root.JSONValue) (any, error) {
	if u.Deliverables == nil {
		return nil, errors.New("this console does not deliver artifacts")
	}
	deliverable, err := domain.ParseDeliverable(arguments)
	if err != nil {
		return nil, err
	}
	workspace := string(session.Export().Workspace)
	target := deliverable.Target(session.ID())
	source := domain.ArtifactFile
	var file DeliveredFile
	if deliverable.Inline() {
		source = domain.ArtifactInline
		file, err = u.Deliverables.WriteDeliverable(ctx, workspace, target, []byte(deliverable.Content))
	} else {
		file, err = u.Deliverables.DeliverFile(ctx, workspace, target)
	}
	if err != nil {
		return nil, err
	}
	return domain.DeliveredArtifact{Artifact: domain.DeliveredArtifactFields{Name: deliverable.Name, MediaType: deliverable.MediaType, Description: deliverable.Description, Size: file.Size, SHA256: file.SHA256, Reference: target, Source: source}}, nil
}
