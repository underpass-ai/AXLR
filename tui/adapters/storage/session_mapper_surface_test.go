package storage

import (
	"context"
	"encoding/json"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
	"github.com/underpass-ai/AXLR/tui/dto"
)

// The surface of a plugin tool is saved with the session's snapshot and
// read back, so a session paused mid-turn resumes with the protocol it
// ran with; a session file without surfaces still loads.
func TestSessionSnapshotKeepsEachToolsSurface(t *testing.T) {
	store, _ := openStore(t)
	s := fixture(t)
	ref := root.PluginRef{PluginID: "kmp", ToolName: "kmp_write_memory"}
	identity, err := domain.NewPluginToolIdentity(ref)
	must(t, err)
	params, err := root.NewJSONObject([]byte(`{"type":"object"}`))
	must(t, err)
	surface := domain.ToolSurface{Role: domain.PluginRoleMemory, ContextArgument: "context_id", MemoryWrite: true}
	tool := domain.AvailableTool{Identity: identity, Definition: root.ToolDefinition{Name: "kmp_write_memory", Description: "kmp/kmp_write_memory: write", Parameters: params}, Surface: surface}
	must(t, s.BeginTurn("hello", []domain.AvailableTool{tool}))
	must(t, store.Save(context.Background(), s))
	got, err := store.Load(context.Background(), s.Export().ID)
	must(t, err)
	if tools := got.ToolSnapshot(); len(tools) != 1 || tools[0].Surface != surface {
		t.Fatalf("snapshot %+v", tools)
	}
	encoded, err := snapshot(s)
	must(t, err)
	encoded.ToolSnapshot[0].Surface = nil
	data, _ := json.Marshal(encoded)
	var old dto.SessionSnapshot
	must(t, json.Unmarshal(data, &old))
	restored, err := restore(old)
	must(t, err)
	if tools := restored.ToolSnapshot(); len(tools) != 1 || tools[0].Surface != (domain.ToolSurface{}) {
		t.Fatalf("old file %+v", tools)
	}
}
