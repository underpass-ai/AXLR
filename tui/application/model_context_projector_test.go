package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// scaled grows a size written for the original 96 KiB budget so tests keep
// exercising the same boundaries when the default budget changes.
func scaled(size int) int {
	return size * domain.DefaultContextBudget().MaximumBytes() / (96 * 1024)
}

func contextMessages(turns, size int) []root.Message {
	messages := []root.Message{{Role: root.RoleSystem, Content: "Trusted stable host instruction"}}
	for i := 0; i < turns; i++ {
		messages = append(messages, root.Message{Role: root.RoleUser, Content: root.Text(fmt.Sprintf("prompt %d: %s", i, strings.Repeat("u", size)))}, root.Message{Role: root.RoleAssistant, Content: root.Text(fmt.Sprintf("answer %d: %s", i, strings.Repeat("a", size)))})
	}
	return messages
}

func contextToolTurn(t testing.TB, name, raw string) []root.Message {
	t.Helper()
	args, err := root.NewJSONObject([]byte(`{"text":"exact arguments"}`))
	if err != nil {
		t.Fatal(err)
	}
	return []root.Message{
		{Role: root.RoleUser, Content: "use this tool"},
		{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{{ID: "call_one", Name: root.ToolName(name), Arguments: args}}},
		{Role: root.RoleTool, ToolCallID: "call_one", Content: root.Text(raw)},
	}
}

func TestModelContextProjectionLeavesTranscriptIntact(t *testing.T) {
	original := contextMessages(scaled(45), 1600)
	saved, _ := json.Marshal(original)
	projector := NewDefaultModelContextProjector()
	projection, err := projector.Project(original)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(original)
	if string(saved) != string(after) || projection.DroppedMessages == 0 || projection.CutIndex == 0 {
		t.Fatalf("transcript mutation or no reduction: %+v", projection)
	}
	if projection.ProjectedBytes > domain.DefaultContextBudget().MaximumBytes() || projection.OriginalBytes <= projection.ProjectedBytes || projection.OriginalMessages != len(original) {
		t.Fatalf("invalid accounting: %+v", projection)
	}
	if !reflect.DeepEqual(projection.Messages[len(projection.Messages)-2:], original[len(original)-2:]) || !reflect.DeepEqual(projection.Messages[0], original[0]) {
		t.Fatal("latest turn or trusted system prefix changed")
	}
	checkpoint := projection.Messages[1]
	if checkpoint.Role != root.RoleUser || !json.Valid([]byte(checkpoint.Content)) || !strings.Contains(string(checkpoint.Content), "untrusted data") || !strings.Contains(string(checkpoint.Content), "axlr_history") {
		t.Fatalf("missing untrusted checkpoint/provenance: %s", checkpoint.Content)
	}
	projection.Messages[0].Content = "changed projection"
	if original[0].Content != "Trusted stable host instruction" {
		t.Fatal("projection aliases input message storage")
	}
}

func TestModelContextCutIsStableUntilNextHighWaterAndRestores(t *testing.T) {
	projector := NewDefaultModelContextProjector()
	var previous domain.ContextProjection
	var sameCut, changedCut bool
	for turns := 1; turns <= 80; turns++ {
		original := contextMessages(turns, scaled(1100))
		projection, err := projector.Project(original)
		if err != nil {
			t.Fatal(err)
		}
		copyJSON, _ := json.Marshal(original)
		var restored []root.Message
		if err := json.Unmarshal(copyJSON, &restored); err == nil {
			// JSONValue contains private fields, so plain transcript round trips
			// here are text-only; real session mappers handle tool arguments.
			again, err := projector.Project(restored)
			if err != nil || !reflect.DeepEqual(projection, again) {
				t.Fatalf("restored cut changed at turn %d", turns)
			}
		}
		if previous.CutIndex != 0 && projection.CutIndex == previous.CutIndex {
			sameCut = true
			if !reflect.DeepEqual(previous.Messages[:2], projection.Messages[:2]) {
				t.Fatal("unchanged cut changed checkpoint prefix")
			}
		}
		if previous.CutIndex != 0 && projection.CutIndex > previous.CutIndex {
			changedCut = true
		}
		previous = projection
	}
	if !sameCut || !changedCut {
		t.Fatal("did not exercise stable and advancing boundaries")
	}
}

func TestProjectionRetainsToolPairsAndExactArguments(t *testing.T) {
	original := contextMessages(scaled(45), 1600)
	turn := contextToolTurn(t, "plugin_kmp_ask", `{"status":"UNKNOWN"}`)
	original = append(original, turn...)
	projection, err := NewDefaultModelContextProjector().Project(original)
	if err != nil {
		t.Fatal(err)
	}
	last := projection.Messages[len(projection.Messages)-3:]
	if !reflect.DeepEqual(last[0], turn[0]) {
		t.Fatal("user prompt changed")
	}
	if !reflect.DeepEqual(last[1], turn[1]) || last[2].ToolCallID != turn[2].ToolCallID {
		t.Fatal("tool call/result identity or exact arguments lost")
	}
	if err := (root.CompletionRequest{Model: "model", Messages: projection.Messages}).Validate(); err != nil {
		t.Fatalf("invalid projected call/result sequence: %v", err)
	}
}

func TestProjectionBoundsLargeToolResultWithoutInvalidJSON(t *testing.T) {
	raw := fmt.Sprintf(`{"protocol_version":1,"status":"completed","output":{"content":[{"type":"text","text":%q}],"structured_content":{"large":%q},"is_error":false}}`, strings.Repeat("memory 🔎 ñ \"", 10000), strings.Repeat("duplicates", 10000))
	original := contextToolTurn(t, "plugin_kmp_ask", raw)
	projection, err := NewDefaultModelContextProjector().Project(original)
	if err != nil {
		t.Fatal(err)
	}
	result := projection.Messages[2]
	if !utf8.ValidString(string(result.Content)) || !json.Valid([]byte(result.Content)) || modelMessageBytes(result) > domain.DefaultContextBudget().ToolResultBytes() || !strings.Contains(string(result.Content), `"message_index":2`) || !strings.Contains(string(result.Content), `"lossy":true`) {
		t.Fatalf("invalid bounded output, bytes=%d: %.300s", modelMessageBytes(result), result.Content)
	}
	if projection.DroppedMessages != 0 || projection.CutIndex != 0 {
		t.Fatal("active turn discarded")
	}
}

func TestMCPProjectionUsesSemanticContentOnceAndStructuredFallback(t *testing.T) {
	large := strings.Repeat("one-copy", 1000)
	text, _ := json.Marshal(map[string]any{"payload": large, "context_id": "ctx-1"})
	full, _ := json.Marshal(map[string]any{"protocol_version": 1, "status": "completed", "output": map[string]any{"content": []any{map[string]any{"type": "text", "text": string(text)}}, "structured_content": json.RawMessage(text), "is_error": false}})
	result, err := projectToolContent(string(full), 0, 32*1024)
	if err != nil || strings.Count(result, large) != 1 || strings.Contains(result, "started_at") {
		t.Fatalf("semantic content duplicated: %v, %s", err, result)
	}
	fallback, err := projectToolContent(`{"protocol_version":1,"status":"completed","output":{"content":[],"structured_content":{"answer":"fallback"}}}`, 0, 16384)
	if err != nil || !strings.Contains(fallback, `"answer":"fallback"`) {
		t.Fatalf("structured fallback lost: %v %s", err, fallback)
	}
}

func TestMCPProjectionRetainsUnequalStructuredEvidenceAndUnknownMetadata(t *testing.T) {
	raw := `{"protocol_version":1,"status":"completed","output":{"content":[{"type":"text","text":"short brief"}],"structured_content":{"evidence":{"claim":"different complete proof","ref":"proof:1"}},"usage":{"cached":24},"status":"domain_unknown"}}`
	result, err := projectToolContent(raw, 0, 16384)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"short brief", "different complete proof", "proof:1", `"cached":24`, `"status":"domain_unknown"`, `"runtime_status":"completed"`} {
		if !strings.Contains(result, expected) {
			t.Fatalf("silently lost distinct MCP evidence/metadata %q: %s", expected, result)
		}
	}
}

func TestMCPProjectionPreservesNumericPrecisionAndUnequalProofs(t *testing.T) {
	raw := `{"protocol_version":1,"status":"completed","output":{"content":[{"type":"text","text":"{\"sequence\":9007199254740993,\"fraction\":0.10000000000000000001}"}],"structured_content":{"sequence":9007199254740992,"fraction":0.10000000000000000001}}}`
	result, err := projectToolContent(raw, 0, 16384)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"sequence":9007199254740993`, `"sequence":9007199254740992`, `0.10000000000000000001`, `"structured_content"`} {
		if !strings.Contains(result, expected) {
			t.Fatalf("numeric proof rounded or unequal structured data deduplicated: %s", result)
		}
	}
	plain, err := projectToolContent(`{"sequence":9007199254740993,"fraction":0.10000000000000000001,"scientific":1e90}`, 1, 16384)
	if err != nil || !strings.Contains(plain, `9007199254740993`) || !strings.Contains(plain, `0.10000000000000000001`) || !strings.Contains(plain, `1e90`) {
		t.Fatalf("plain JSON numeric literals changed: %v %s", err, plain)
	}
	clipped, err := projectToolContent(`{"scope":{"sequence":9007199254740993},"body":"`+strings.Repeat("x", 50000)+`"}`, 2, 16384)
	if err != nil || !strings.Contains(clipped, `"sequence":9007199254740993`) {
		t.Fatalf("active control number rounded during clipping: %v %s", err, clipped)
	}
	if _, err := decodeContextJSON([]byte(`{} {}`)); err == nil {
		t.Fatal("extra JSON document silently ignored")
	}
	if _, err := decodeContextJSON([]byte(`{} broken`)); err == nil {
		t.Fatal("trailing malformed data silently ignored")
	}
}

func TestProjectionKeepsBinaryAndEmbeddedResourceMetadataDuringClipping(t *testing.T) {
	result := map[string]any{"protocol_version": 1, "status": "completed", "output": map[string]any{"content": []any{
		map[string]any{"type": "image", "mimeType": "image/png", "data": strings.Repeat("A", 70000)},
		map[string]any{"type": "resource", "resource": map[string]any{"uri": "memory://embedded", "mimeType": "text/plain", "text": strings.Repeat("data", 20000)}},
	}}}
	raw, _ := json.Marshal(result)
	projected, err := projectToolContent(string(raw), 42, 16384)
	if err != nil || !json.Valid([]byte(projected)) {
		t.Fatalf("binary result failed: %v", err)
	}
	for _, expected := range []string{`"non_text_content"`, `"mimeType":"image/png"`, `"data_omitted":true`, "memory://embedded", `"text_omitted":true`, `"message_index":42`} {
		if !strings.Contains(projected, expected) {
			t.Fatalf("lost resource metadata %q", expected)
		}
	}
}

func TestProjectionClipsCatalogueWhoseRecordsReuseControlNames(t *testing.T) {
	tools := make([]any, 0, 400)
	for i := 0; i < 400; i++ {
		tools = append(tools, map[string]any{"name": fmt.Sprintf("tool_%d", i), "inputSchema": map[string]any{"properties": map[string]any{
			"status": map[string]any{"type": "string", "description": "Lifecycle status of the record."},
			"scope":  map[string]any{"kind": "global"},
		}}})
	}
	catalogue, _ := json.Marshal(map[string]any{"server": "made", "tools": tools})
	raw, _ := json.Marshal(map[string]any{"protocol_version": 1, "status": "completed", "output": map[string]any{
		"content": []any{map[string]any{"type": "text", "text": string(catalogue)}},
	}})
	projected, err := projectToolContent(string(raw), 26, 16384)
	if err != nil || !json.Valid([]byte(projected)) {
		t.Fatalf("catalogue result aborted the turn: %v", err)
	}
	for _, expected := range []string{`"record_controls_omitted":true`, `"message_index":26`, `"status":"completed"`} {
		if !strings.Contains(projected, expected) {
			t.Fatalf("missing %q in %s", expected, projected[:200])
		}
	}
}

func TestProjectionRefusesToSilentlyTruncateOversizedProtocolControls(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{"context_id": strings.Repeat("identity", 10000)})
	_, err := projectToolContent(string(raw), 42, 16384)
	if !errors.Is(err, ErrContextBudgetExceeded) {
		t.Fatalf("protocol identity silently clipped: %v", err)
	}
	guide := contextToolTurn(t, "kmp_guide", string(raw))
	original := append(guide, contextMessages(50, 1500)[1:]...)
	projection, err := NewDefaultModelContextProjector().Project(original)
	if err != nil || !strings.Contains(string(projection.Messages[0].Content), `"identities_omitted":true`) {
		t.Fatalf("closed oversized identity must be recoverable, never truncated: %v", err)
	}
}

func TestClosedOversizedProtocolForcesRecoverableWholeTurnCut(t *testing.T) {
	for _, field := range []string{"error", "scope"} {
		t.Run(field, func(t *testing.T) {
			raw, _ := json.Marshal(map[string]any{"context_id": "context-original", field: strings.Repeat("x", 3*domain.DefaultContextBudget().ToolResultBytes())})
			closed := append(contextToolTurn(t, "legacy_kmp_call", string(raw)), root.Message{Role: root.RoleAssistant, Content: "finished"})
			if _, err := NewDefaultModelContextProjector().Project(closed); !errors.Is(err, ErrContextBudgetExceeded) {
				t.Fatalf("active oversized control must fail explicitly: %v", err)
			}
			session, err := domain.NewSession("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", domain.Workspace(t.TempDir()), "test/model")
			if err != nil {
				t.Fatal(err)
			}
			identity, _ := domain.NewPluginToolIdentity(root.PluginRef{PluginID: "kmp", ToolName: "kmp_ask"})
			parameters, _ := root.NewJSONObject([]byte(`{"type":"object"}`))
			tools := []domain.AvailableTool{{Identity: identity, Definition: root.ToolDefinition{Name: "legacy_kmp_call", Description: "memory", Parameters: parameters}}}
			if err := session.BeginTurn(closed[0].Content, tools); err != nil {
				t.Fatal(err)
			}
			if err := session.CompleteAssistant(root.CompletionResult{Message: closed[1]}); err != nil {
				t.Fatal(err)
			}
			if err := session.RecordToolOutcome(closed[2].ToolCallID, domain.DecisionAutoApprove, domain.ToolOutcome{Content: closed[2].Content}); err != nil {
				t.Fatal(err)
			}
			if err := session.CompleteAssistant(root.CompletionResult{Message: closed[3]}); err != nil {
				t.Fatal(err)
			}
			if err := session.BeginTurn("next small question", tools); err != nil {
				t.Fatal(err)
			}
			original := session.Messages()
			saved, _ := json.Marshal(original)
			projector := NewDefaultModelContextProjector()
			projection, err := projector.Project(original)
			if err != nil {
				t.Fatal(err)
			}
			if projection.CutIndex != len(closed) || projection.DroppedMessages != len(closed) || len(projection.Messages) != 2 || !reflect.DeepEqual(projection.Messages[1], original[len(closed)]) {
				t.Fatalf("whole old turn or current input changed: %+v", projection)
			}
			var checkpoint map[string]any
			if err := json.Unmarshal([]byte(projection.Messages[0].Content), &checkpoint); err != nil {
				t.Fatal(err)
			}
			protocol := checkpoint["latest_memory_protocol"].(map[string]any)
			controls := protocol["controls"].(map[string]any)
			if protocol["message_index"] != float64(2) || protocol["protocol_controls_omitted"] != true || controls["context_id"] != "context-original" || controls[field] != nil || protocol["retrieval"] == nil {
				t.Fatalf("missing explicit exact identity/recovery: %v", protocol)
			}
			if checkpoint["omitted_until_exclusive"] != float64(len(closed)) {
				t.Fatal("checkpoint provenance changed")
			}
			after, _ := json.Marshal(original)
			if string(saved) != string(after) {
				t.Fatal("original transcript changed")
			}
			// Replaying after a cold restore and appending another small turn must
			// retain the same epoch and exact checkpoint prefix.
			restored, err := domain.RestoreSession(session.Export())
			if err != nil {
				t.Fatal(err)
			}
			again, err := projector.Project(restored.Messages())
			if err != nil || !reflect.DeepEqual(projection, again) {
				t.Fatalf("cold restore changed forced cut: %v", err)
			}
			extended := append(original, root.Message{Role: root.RoleAssistant, Content: "answer"}, root.Message{Role: root.RoleUser, Content: "another question"})
			again, err = projector.Project(extended)
			if err != nil || again.CutIndex != projection.CutIndex || !reflect.DeepEqual(again.Messages[0], projection.Messages[0]) {
				t.Fatalf("stable checkpoint epoch changed: %v", err)
			}
		})
	}
}

func FuzzModelContextProjectionBoundsJSONAndUTF8(f *testing.F) {
	for _, seed := range []string{"hello", "ñ🔎\n\"", "{\"protocol_version\":1,\"output\":{\"content\":[]}}", ""} {
		f.Add(seed, uint8(4))
	}
	f.Fuzz(func(t *testing.T, seed string, multiplier uint8) {
		if !utf8.ValidString(seed) || len(seed) > 1000 {
			t.Skip()
		}
		raw, _ := json.Marshal(map[string]any{"data": strings.Repeat(seed, int(multiplier)+1)})
		messages := contextToolTurn(t, "plugin_memory", string(raw))
		projection, err := NewDefaultModelContextProjector().Project(messages)
		if err != nil {
			t.Fatal(err)
		}
		result := string(projection.Messages[2].Content)
		if !utf8.ValidString(result) || !json.Valid([]byte(result)) || modelMessageBytes(projection.Messages[2]) > domain.DefaultContextBudget().ToolResultBytes() || !reflect.DeepEqual(projection.Messages[1], messages[1]) {
			t.Fatal("clipping corrupted JSON, UTF8 or exact tool call")
		}
	})
}

func TestActiveProtocolStateSurvivesClipping(t *testing.T) {
	payload := map[string]any{"protocol_version": 1, "status": "failed", "error": map[string]any{"code": "transport_failure"}, "uncertain": true, "output": map[string]any{"content": []any{map[string]any{"type": "text", "text": strings.Repeat("large ", 10000)}, map[string]any{"type": "resource_link", "uri": "memory://test", "name": "source"}}, "structured_content": map[string]any{"context_id": "context-current", "agent": map[string]any{"id": "agent-current"}, "scope": map[string]any{"about": "project:AXLR"}, "projection": map[string]any{"next_action": map[string]any{"tool": "kmp_wake", "arguments": map[string]any{"continuation": "bound-continuation"}}, "page": map[string]any{"has_more": true}}}}}
	raw, _ := json.Marshal(payload)
	result, err := projectToolContent(string(raw), 3, 16*1024)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"context-current", "agent-current", "bound-continuation", "project:AXLR", "transport_failure", `"uncertain":true`, "memory://test", `"has_more":true`} {
		if !strings.Contains(result, expected) {
			t.Fatalf("lost protocol/resource state %s", expected)
		}
	}
}

func TestCurrentDiscoverySchemaIsKeptExactBeyondStandardToolCap(t *testing.T) {
	raw := fmt.Sprintf(`{"schema":{"description":%q}}`, strings.Repeat("schema ", 3000))
	projection, err := NewDefaultModelContextProjector().Project(contextToolTurn(t, "axlr_tools", raw))
	if err != nil || strings.Contains(string(projection.Messages[2].Content), "axlr_tool_result_excerpt") || !strings.Contains(string(projection.Messages[2].Content), strings.Repeat("schema ", 3000)) {
		t.Fatalf("exact schema was clipped: %v", err)
	}
	_, err = NewDefaultModelContextProjector().Project(contextToolTurn(t, "axlr_tools", fmt.Sprintf(`{"schema":%q}`, strings.Repeat("s", MaxHostResultBytes+8000))))
	if !errors.Is(err, ErrContextBudgetExceeded) {
		t.Fatalf("oversized exact schema did not fail explicitly: %v", err)
	}
}

func TestCurrentOversizedPromptAndArgumentsFailExplicitly(t *testing.T) {
	projector := NewDefaultModelContextProjector()
	_, err := projector.Project([]root.Message{{Role: root.RoleUser, Content: root.Text(strings.Repeat("p", scaled(100000)))}})
	if !errors.Is(err, ErrContextBudgetExceeded) {
		t.Fatalf("large prompt silently cut: %v", err)
	}
	args, _ := root.NewJSONObject([]byte(fmt.Sprintf(`{"exact":%q}`, strings.Repeat("arg", scaled(40000)))))
	_, err = projector.Project([]root.Message{{Role: root.RoleUser, Content: "use tool"}, {Role: root.RoleAssistant, ToolCalls: []root.ToolCall{{ID: "call_large", Name: "local_read", Arguments: args}}}})
	if !errors.Is(err, ErrContextBudgetExceeded) {
		t.Fatalf("large arguments silently cut: %v", err)
	}
}

func TestProviderMessageSizeCountsEscapedArgumentsAndUnicode(t *testing.T) {
	args, _ := root.NewJSONObject([]byte(`{"text":"\"ñ🔎<>&"}`))
	messages := []root.Message{{Role: root.RoleUser, Content: "ñ🔎<>&\n"}, {Role: root.RoleAssistant, ToolCalls: []root.ToolCall{{ID: "call_one", Name: "local_read", Arguments: args}}}, {Role: root.RoleTool, ToolCallID: "call_one", Content: "hello"}}
	wire := []map[string]any{
		{"role": "user", "content": "ñ🔎<>&\n"},
		{"role": "assistant", "content": nil, "tool_calls": []any{map[string]any{"id": "call_one", "type": "function", "function": map[string]any{"name": "local_read", "arguments": string(args.Bytes())}}}},
		{"role": "tool", "content": "hello", "tool_call_id": "call_one"},
	}
	encoded, _ := json.Marshal(wire)
	if ModelMessagesBytes(messages) != len(encoded) {
		t.Fatalf("size estimate=%d, real=%d", ModelMessagesBytes(messages), len(encoded))
	}
	for _, limit := range []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 100} {
		for _, excerpt := range []string{utf8Prefix("ñ🔎hello", limit), utf8Suffix("hello🔎ñ", limit)} {
			if !utf8.ValidString(excerpt) || len(excerpt) > limit {
				t.Fatalf("unsafe UTF8 limit=%d excerpt=%q", limit, excerpt)
			}
		}
	}
}

func TestCheckpointPreservesMemoryIdentityAndGuideLookup(t *testing.T) {
	guide := contextToolTurn(t, "mcp_opaque_legacy_alias", `{"protocol_version":1,"status":"completed","output":{"content":[],"structured_content":{"context_id":"old-context","guide_revision":"revision-1","scheme":[],"agent":{"id":"old-agent"}}}}`)
	// Later results mention the spelling context_id in prose without actually
	// carrying an identity. They must not hide the earlier registered identity.
	notIdentity := root.Message{Role: root.RoleTool, ToolCallID: "call_other", Content: `{"status":"completed","content":[{"type":"text","text":"reuse context_id from the guide"}]}`}
	original := append(append(guide, notIdentity), contextMessages(scaled(45), 1600)[1:]...)
	projection, err := NewDefaultModelContextProjector().Project(original)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := projection.Messages[0]
	for _, expected := range []string{"old-context", "old-agent", "latest_memory_guide", `"message_index":2`, "Do not register a second"} {
		if !strings.Contains(string(checkpoint.Content), expected) {
			t.Fatalf("lost guide/identity %q: %s", expected, checkpoint.Content)
		}
	}
}

func TestGuideRecoveryUsesPacketShapeNotWriteProseOrOpaqueAlias(t *testing.T) {
	guide := contextToolTurn(t, "mcp_8469fd8c2a67", `{"protocol_version":1,"output":{"structured_content":{"context_id":"context-legacy","agent":{"id":"agent-legacy"},"guide_revision":"rev-legacy","scheme":[]},"content":[]}}`)
	args, _ := root.NewJSONObject([]byte(`{"name":"kmp_write_memory","arguments":{"text":"Previously used kmp_guide"}}`))
	write := []root.Message{
		{Role: root.RoleUser, Content: "record an outcome"},
		{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{{ID: "write-two", Name: "axlr_call_tool", Arguments: args}}},
		{Role: root.RoleTool, ToolCallID: "write-two", Content: `{"status":"completed","context_id":"context-legacy","receipt":{"note":"kmp_guide was used"}}`},
	}
	original := append(append(guide, write...), contextMessages(scaled(45), 1600)[1:]...)
	projection, err := NewDefaultModelContextProjector().Project(original)
	if err != nil {
		t.Fatal(err)
	}
	var checkpoint struct {
		Guide struct {
			Index int `json:"message_index"`
		} `json:"latest_memory_guide"`
	}
	if err := json.Unmarshal([]byte(projection.Messages[0].Content), &checkpoint); err != nil || checkpoint.Guide.Index != 2 {
		t.Fatalf("guide lookup selected write prose rather than legacy guide result: %v %+v", err, checkpoint)
	}
	valid := map[string]any{"context_id": "context", "guide_revision": "revision", "scheme": []any{}, "agent": map[string]any{"id": "agent"}}
	text, _ := json.Marshal(valid)
	if memoryGuidePacket(map[string]any{"content": []any{map[string]any{"type": "text", "text": string(text)}}}) == nil {
		t.Fatal("textual MCP guide packet was missed")
	}
	if memoryGuidePacket(map[string]any{"payload": valid}) != nil || memoryGuidePacket(map[string]any{"context_id": "context", "guide_revision": "revision", "scheme": "not an array", "agent": map[string]any{"id": "agent"}}) != nil {
		t.Fatal("unrelated nested claim or malformed guide granted guide identity")
	}
}

func TestColdRestoreWithOpaqueGuideToolReproducesProjectionAndIdentity(t *testing.T) {
	session, err := domain.NewSession("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", domain.Workspace(t.TempDir()), "test/model")
	if err != nil {
		t.Fatal(err)
	}
	identity, _ := domain.NewPluginToolIdentity(root.PluginRef{PluginID: "kmp", ToolName: "kmp_guide"})
	parameters, _ := root.NewJSONObject([]byte(`{"type":"object"}`))
	tools := []domain.AvailableTool{{Identity: identity, Definition: root.ToolDefinition{Name: "mcp_opaque_legacy", Description: "brief guide", Parameters: parameters}}}
	if err := session.BeginTurn("retrieve the memory guide", tools); err != nil {
		t.Fatal(err)
	}
	args, _ := root.NewJSONObject([]byte(`{}`))
	call := root.ToolCall{ID: "restore-guide", Name: "mcp_opaque_legacy", Arguments: args}
	if err := session.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{call}}}); err != nil {
		t.Fatal(err)
	}
	if err := session.RecordToolOutcome(call.ID, domain.DecisionAutoApprove, domain.ToolOutcome{Content: `{"protocol_version":1,"output":{"content":[],"structured_content":{"context_id":"context-restored","agent":{"id":"agent-restored"},"guide_revision":"rev-restored","scheme":[]}}}`}); err != nil {
		t.Fatal(err)
	}
	if err := session.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "Guide registered."}}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < scaled(45); i++ {
		if err := session.BeginTurn(root.Text(fmt.Sprintf("short instruction %d", i)), tools); err != nil {
			t.Fatal(err)
		}
		if err := session.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: root.Text(strings.Repeat("assistant claim ", 400))}}); err != nil {
			t.Fatal(err)
		}
	}
	restored, err := domain.RestoreSession(session.Export())
	if err != nil {
		t.Fatal(err)
	}
	first, err := NewDefaultModelContextProjector().Project(session.Messages())
	if err != nil {
		t.Fatal(err)
	}
	cold, err := NewDefaultModelContextProjector().Project(restored.Messages())
	if err != nil || !reflect.DeepEqual(first, cold) {
		t.Fatalf("cold restore changed boundary or provider projection: %v", err)
	}
	if !strings.Contains(string(cold.Messages[0].Content), "context-restored") || !strings.Contains(string(cold.Messages[0].Content), `"message_index":2`) {
		t.Fatal("restored guide identity/recovery index lost")
	}
}

func TestClippedHistoryPreservesExactPagingControlFields(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{"message_index": 7, "offset_bytes": 4096, "next_offset_bytes": 8192, "total_bytes": 40000, "has_more": true, "text": strings.Repeat("large", 10000)})
	result, err := projectToolContent(string(raw), 12, 16384)
	if err != nil {
		t.Fatal(err)
	}
	var bounded struct {
		Controls map[string]any `json:"protocol_controls"`
	}
	if err := json.Unmarshal([]byte(result), &bounded); err != nil {
		t.Fatal(err)
	}
	for key, expected := range map[string]any{"message_index": float64(7), "offset_bytes": float64(4096), "next_offset_bytes": float64(8192), "total_bytes": float64(40000), "has_more": true} {
		if bounded.Controls[key] != expected {
			t.Fatalf("paging control %s changed: %v", key, bounded.Controls)
		}
	}
}

func TestCheckpointPrioritizesExactUserConstraintsBeforeAssistantClaims(t *testing.T) {
	original := []root.Message{{Role: root.RoleUser, Content: root.Text("Keep this constraint exactly: " + strings.Repeat("ñ", 400))}, {Role: root.RoleAssistant, Content: root.Text(strings.Repeat("assistant claim ", 2000))}}
	for i := 0; i < scaled(12); i++ {
		original = append(original, root.Message{Role: root.RoleUser, Content: root.Text(fmt.Sprintf("short instruction %d", i))}, root.Message{Role: root.RoleAssistant, Content: root.Text(strings.Repeat("other assistant claims ", 1000))})
	}
	projection, err := NewDefaultModelContextProjector().Project(original)
	if err != nil || projection.CutIndex == 0 {
		t.Fatalf("checkpoint did not trigger: %v", err)
	}
	var checkpoint struct {
		Users []struct {
			Index    int    `json:"message_index"`
			Complete bool   `json:"complete"`
			Content  string `json:"quoted_content"`
		} `json:"historical_user_inputs_newest_first"`
		Coverage struct {
			Exact         int `json:"exact"`
			Unrepresented int `json:"unrepresented"`
		} `json:"historical_user_input_coverage"`
	}
	if err := json.Unmarshal([]byte(projection.Messages[0].Content), &checkpoint); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, input := range checkpoint.Users {
		if input.Index == 0 && input.Complete && input.Content == string(original[0].Content) {
			found = true
		}
	}
	if !found || checkpoint.Coverage.Exact != projection.CutIndex/2 || checkpoint.Coverage.Unrepresented != 0 {
		t.Fatalf("old small user constraints lost while assistant claims consumed budget: %+v", checkpoint)
	}
	if modelMessageBytes(projection.Messages[0]) > domain.DefaultContextBudget().CheckpointBytes() {
		t.Fatal("checkpoint exceeded limit")
	}
}

func TestProjectorRejectsInvalidBudgetAndRetainsSmallTranscripts(t *testing.T) {
	if _, err := NewModelContextProjector(domain.ContextBudget{}); err == nil {
		t.Fatal("zero budget accepted")
	}
	if _, err := (ModelContextProjector{}).Project(nil); err == nil {
		t.Fatal("zero projector accepted")
	}
	projector := NewDefaultModelContextProjector()
	projection, err := projector.Project(nil)
	if err != nil || len(projection.Messages) != 0 || projection.ProjectedBytes != 2 {
		t.Fatal("empty transcript failed")
	}
	small := contextMessages(1, 20)
	projection, err = projector.Project(small)
	if err != nil || !reflect.DeepEqual(projection.Messages, small) || projection.CutIndex != 0 {
		t.Fatal("small transcript changed")
	}
	projection, err = projector.Project([]root.Message{{Role: root.RoleSystem, Content: "host"}})
	if err != nil || len(projection.Messages) != 1 {
		t.Fatal("system-only transcript failed")
	}
	_, err = projector.Project([]root.Message{{Role: root.RoleSystem, Content: root.Text(strings.Repeat("x", scaled(100000)))}})
	if !errors.Is(err, ErrContextBudgetExceeded) {
		t.Fatal("oversized system-only transcript accepted")
	}
}

func BenchmarkModelContextProjector(b *testing.B) {
	messages := contextMessages(80, 1600)
	if path := os.Getenv("AXLR_REPLAY_REQUEST"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			b.Fatal(err)
		}
		var request struct {
			Messages []struct {
				Role       string `json:"role"`
				Content    string `json:"content"`
				ToolCallID string `json:"tool_call_id"`
				ToolCalls  []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(data, &request); err != nil {
			b.Fatal(err)
		}
		messages = nil
		for _, wire := range request.Messages {
			message := root.Message{Role: root.MessageRole(wire.Role), Content: root.Text(wire.Content), ToolCallID: root.ToolCallID(wire.ToolCallID)}
			for _, call := range wire.ToolCalls {
				args, err := root.NewJSONObject([]byte(call.Function.Arguments))
				if err != nil {
					b.Fatal(err)
				}
				message.ToolCalls = append(message.ToolCalls, root.ToolCall{ID: root.ToolCallID(call.ID), Name: root.ToolName(call.Function.Name), Arguments: args})
			}
			messages = append(messages, message)
		}
	}
	projector := NewDefaultModelContextProjector()
	projection, err := projector.Project(messages)
	if err != nil {
		b.Fatal(err)
	}
	b.Logf("messages=%d projected=%d original_bytes=%d projected_bytes=%d cut=%d", len(messages), len(projection.Messages), projection.OriginalBytes, projection.ProjectedBytes, projection.CutIndex)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := projector.Project(messages); err != nil {
			b.Fatal(err)
		}
	}
}
