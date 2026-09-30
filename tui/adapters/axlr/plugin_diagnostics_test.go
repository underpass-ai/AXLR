package axlr

import (
	"context"
	"encoding/json"
	"errors"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/adapters/diagnostics"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func pluginTraceEvents(t *testing.T, path string) []application.DiagnosticEvent {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var events []application.DiagnosticEvent
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var e application.DiagnosticEvent
		if err = json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatal(err)
		}
		events = append(events, e)
	}
	return events
}

func TestPluginDiscoveryOrdinalsStayConsistentBetweenPanelAndToolCatalog(t *testing.T) {
	raw := testManager(t, "")
	profiles := []domain.PluginProfile{pluginProfile("beta", domain.ApprovalManual), pluginProfile("alpha", domain.ApprovalAuto)}
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	trace, err := diagnostics.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer trace.Close()
	var persistedSpan uint64
	manager := NewPluginManager(raw, profiles, func(ctx context.Context, id root.PluginID, mode domain.ApprovalMode) error {
		persistedSpan = application.CurrentDiagnosticSpan(ctx)
		if id != "beta" || mode != domain.ApprovalAuto {
			t.Fatal("wrong policy persisted")
		}
		return nil
	})
	manager.Diagnostics = trace
	ctx, parent := application.StartDiagnosticSpan(context.Background(), trace, application.DiagnosticActionOperation, application.DiagnosticEvent{})
	parentID := application.CurrentDiagnosticSpan(ctx)
	states, err := manager.List(ctx)
	if err != nil || len(states) != 2 || states[0].Profile.ID != "beta" || states[1].Profile.ID != "alpha" {
		t.Fatal("panel order or discovery changed", states, err)
	}
	catalogCtx, catalogSpan := application.StartDiagnosticSpan(ctx, trace, application.DiagnosticActionTools, application.DiagnosticEvent{})
	snapshot, err := (ToolCatalog{Plugins: raw, Diagnostics: trace, Profiles: manager.Profiles}).Snapshot(catalogCtx)
	catalogSpan.End(application.DiagnosticErrorNone)
	pluginSnapshot := pluginTools(snapshot)
	if err != nil || len(pluginSnapshot) != 2 || pluginSnapshot[0].Identity.Plugin.PluginID != "beta" || pluginSnapshot[1].Identity.Plugin.PluginID != "alpha" {
		t.Fatal("catalog discovery changed profile order", snapshot, err)
	}
	if profiles[0].ID != "beta" || manager.Profiles()[0].ID != "beta" {
		t.Fatal("sorting changed configuration")
	}
	if err = manager.SetApproval(ctx, "beta", domain.ApprovalAuto); err != nil {
		t.Fatal(err)
	}
	parent.End(application.DiagnosticErrorNone)
	events := pluginTraceEvents(t, path)
	starts := map[uint64]application.DiagnosticEvent{}
	ends := map[uint64]application.DiagnosticEvent{}
	var discoveries []application.DiagnosticEvent
	for _, e := range events {
		if e.Stage == application.DiagnosticActionStart {
			starts[e.SpanID] = e
			if e.Action == application.DiagnosticActionPluginDiscovery {
				discoveries = append(discoveries, e)
			}
		}
		if e.Stage == application.DiagnosticActionEnd {
			ends[e.SpanID] = e
		}
	}
	if len(starts) != len(ends) || len(discoveries) != 4 {
		t.Fatal("plugin lifecycles unbalanced", events)
	}
	for id, a := range starts {
		b, ok := ends[id]
		if !ok || a.Action != b.Action || a.ParentSpanID != b.ParentSpanID || b.ElapsedMicroseconds < 0 || b.ErrorClass != application.DiagnosticErrorNone {
			t.Fatal("correlation lost", a, b)
		}
	}
	for i, want := range []int{2, 1, 2, 1} {
		if discoveries[i].PluginOrdinal != want {
			t.Fatal("ordinal does not identify same server", discoveries)
		}
	}
	if discoveries[0].ParentSpanID != discoveries[1].ParentSpanID || discoveries[2].ParentSpanID != application.CurrentDiagnosticSpan(catalogCtx) || discoveries[3].ParentSpanID != application.CurrentDiagnosticSpan(catalogCtx) {
		t.Fatal("discovery not nested", discoveries)
	}
	policy := starts[persistedSpan]
	if policy.Action != application.DiagnosticActionPluginPolicy || policy.ParentSpanID != parentID || policy.PluginOrdinal != 2 {
		t.Fatal("policy persistence lost context", policy)
	}
}

func TestPluginDiscoveryAndPolicyRecordFailuresWithoutRawContent(t *testing.T) {
	raw := testManager(t, "")
	_ = raw.Close()
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	trace, err := diagnostics.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer trace.Close()
	privateError := errors.New("PRIVATE_SECRET_PATH=/hidden")
	manager := NewPluginManager(raw, []domain.PluginProfile{pluginProfile("alpha", domain.ApprovalManual)}, func(context.Context, root.PluginID, domain.ApprovalMode) error { return privateError })
	manager.Diagnostics = trace
	states, err := manager.List(context.Background())
	if err != nil || len(states) != 1 || states[0].Error == "" {
		t.Fatal("safe discovery state lost", states, err)
	}
	if err = manager.SetApproval(context.Background(), "alpha", domain.ApprovalAuto); !errors.Is(err, privateError) {
		t.Fatal("persistence failure changed", err)
	}
	if manager.AutoApproves(pluginIdentity("alpha", "echo")) {
		t.Fatal("failed persistence granted policy")
	}
	events := pluginTraceEvents(t, path)
	classes := map[application.DiagnosticAction]application.DiagnosticErrorClass{}
	for _, e := range events {
		if e.Stage == application.DiagnosticActionEnd {
			classes[e.Action] = e.ErrorClass
			if e.Action != application.DiagnosticActionTools && e.PluginOrdinal != 1 {
				t.Fatal("ordinal lost", e)
			}
		}
	}
	if classes[application.DiagnosticActionTools] != application.DiagnosticErrorTool || classes[application.DiagnosticActionPluginDiscovery] != application.DiagnosticErrorTool || classes[application.DiagnosticActionPluginPolicy] != application.DiagnosticErrorStorage {
		t.Fatal("failure classes absent", classes)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), privateError.Error()) || strings.Contains(string(data), "alpha") {
		t.Fatal("raw names or secrets leaked")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = manager.List(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err = manager.SetApproval(ctx, "alpha", domain.ApprovalAuto); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	events = pluginTraceEvents(t, path)
	last := events[len(events)-1]
	if last.Stage != application.DiagnosticActionEnd || last.Action != application.DiagnosticActionPluginPolicy || last.ErrorClass != application.DiagnosticErrorCancelled {
		t.Fatal("cancelled policy unbalanced", last)
	}
}

func TestProfileCatalogClosesServerFailure(t *testing.T) {
	raw := testManager(t, "")
	_ = raw.Close()
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	trace, err := diagnostics.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer trace.Close()
	_, err = (ToolCatalog{Plugins: raw, Diagnostics: trace, Profiles: func() []domain.PluginProfile {
		return []domain.PluginProfile{pluginProfile("alpha", domain.ApprovalManual)}
	}}).Snapshot(context.Background())
	if err == nil {
		t.Fatal("failed catalog accepted")
	}
	events := pluginTraceEvents(t, path)
	if len(events) != 2 || events[0].Stage != application.DiagnosticActionStart || events[1].Stage != application.DiagnosticActionEnd || events[0].SpanID != events[1].SpanID || events[1].ErrorClass != application.DiagnosticErrorTool {
		t.Fatal("failed discovery unbalanced", events)
	}
	if pluginOrdinal(nil, "missing") != 0 || pluginDiagnosticError(context.DeadlineExceeded) != application.DiagnosticErrorTimeout {
		t.Fatal("ordinal or deadline classification")
	}
}
