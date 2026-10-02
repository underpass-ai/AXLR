package application

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
)

// projectToolContent removes transport duplication while keeping protocol state.
// A clipped result is a valid JSON object with explicit original provenance; its
// excerpt is a string, never a fragment represented as if it were valid data.
func projectToolContent(raw string, index, limit int) (string, error) {
	return projectToolContentIn(raw, index, limit, excerptHistorical)
}

// excerptPlace says where a clipped result sits, which decides how the model
// can get the rest of it.
type excerptPlace int

const (
	// excerptHistorical is a closed turn: axlr_history returns the original.
	excerptHistorical excerptPlace = iota
	// excerptCurrentTurn is the turn in progress: re-reading it through
	// axlr_history would refill the very context it was clipped to save.
	excerptCurrentTurn
	// excerptCompacted is the turn in progress shrunk further because the
	// turn alone no longer fits the context budget.
	excerptCompacted
)

func projectToolContentIn(raw string, index, limit int, place excerptPlace) (string, error) {
	original, decodeErr := decodeContextJSON([]byte(raw))
	if decodeErr != nil {
		if contentJSONBytes(raw) <= limit-256 {
			return raw, nil
		}
		original = raw
	}
	semantic := original
	if envelope, ok := original.(map[string]any); ok {
		if _, runtimeEnvelope := envelope["protocol_version"]; runtimeEnvelope {
			if output, ok := envelope["output"].(map[string]any); ok {
				semantic = projectMCPOutput(output)
				if result, ok := semantic.(map[string]any); ok {
					for _, key := range []string{"status", "error", "uncertain"} {
						if value, exists := envelope[key]; exists && value != nil {
							if current, exists := result[key]; exists && !reflect.DeepEqual(current, value) {
								result["runtime_"+key] = value
							} else {
								result[key] = value
							}
						}
					}
					result["protocol_version"] = envelope["protocol_version"]
				}
			}
		}
	}
	encoded, _ := json.Marshal(semantic)
	if contentJSONBytes(string(encoded)) <= limit-256 {
		return string(encoded), nil
	}
	bounded := map[string]any{
		"kind": "axlr_tool_result_excerpt", "lossy": true,
		"message_index": index, "original_bytes": len(raw),
		"retrieval": fmt.Sprintf("axlr_history({message_index: %d, offset_bytes: 0}); continue with next_offset_bytes for the exact original result.", index),
	}
	switch place {
	case excerptCurrentTurn:
		bounded["retrieval"] = "This result belongs to the current turn and axlr_history will not re-read it. If you need more of it, repeat the original call with a narrower query, filter or page."
	case excerptCompacted:
		bounded["retrieval"] = "This turn no longer fits the model context, so its earlier results were shortened. Answer with what you have, or ask the user before gathering more."
	}
	if controls := protocolControls(original); controls != nil {
		bounded["protocol_controls"] = controls
	}
	if blocks := nonTextMetadata(semantic); len(blocks) > 0 {
		bounded["non_text_content"] = blocks
	}
	minimum, _ := json.Marshal(bounded)
	if contentJSONBytes(string(minimum)) > limit-256 {
		// Control-named fields inside record collections are data. Dropping them
		// is explicit and the exact original stays retrievable through retrieval.
		delete(bounded, "protocol_controls")
		if controls := envelopeProtocolControls(original); controls != nil {
			bounded["protocol_controls"] = controls
		}
		bounded["record_controls_omitted"] = true
		minimum, _ = json.Marshal(bounded)
	}
	if contentJSONBytes(string(minimum)) > limit-256 {
		return "", fmt.Errorf("active protocol controls cannot be clipped: %w", ErrContextBudgetExceeded)
	}
	available := limit - contentJSONBytes(string(minimum)) - 512
	if available < 0 {
		available = 0
	}
	for {
		bounded["excerpt_start"] = utf8Prefix(string(encoded), available/2)
		bounded["excerpt_end"] = utf8Suffix(string(encoded), available/2)
		bounded["omitted_bytes"] = len(encoded) - len(bounded["excerpt_start"].(string)) - len(bounded["excerpt_end"].(string))
		result, _ := json.Marshal(bounded)
		if contentJSONBytes(string(result)) <= limit-256 {
			return string(result), nil
		}
		if available == 0 {
			return "", ErrContextBudgetExceeded
		}
		available /= 2
	}
}

func contentJSONBytes(text string) int {
	encoded, _ := json.Marshal(text)
	return len(encoded)
}

func utf8Suffix(text string, maximum int) string {
	if len(text) <= maximum {
		return text
	}
	if maximum <= 0 {
		return ""
	}
	start := len(text) - maximum
	for start < len(text) && text[start]&0xc0 == 0x80 {
		start++
	}
	return text[start:]
}

func projectMCPOutput(output map[string]any) any {
	blocks, hasContent := output["content"].([]any)
	if !hasContent {
		return output // Local output has its own domain shape.
	}
	result := map[string]any{}
	for key, value := range output {
		if key != "content" && key != "structured_content" && key != "structuredContent" {
			result[key] = value
		}
	}
	if len(blocks) == 0 {
		if structured, exists := output["structured_content"]; exists {
			result["content"] = structured
			if other, exists := output["structuredContent"]; exists && !reflect.DeepEqual(other, structured) {
				result["structuredContent"] = other
			}
		} else if structured, exists := output["structuredContent"]; exists {
			result["content"] = structured
		} else {
			result["content"] = []any{}
		}
		return result
	}
	content := make([]any, 0, len(blocks))
	for _, value := range blocks {
		block, ok := value.(map[string]any)
		if !ok || block["type"] != "text" {
			content = append(content, value) // Resource/image metadata stays exact.
			continue
		}
		text, ok := block["text"].(string)
		object, err := decodeContextJSON([]byte(text))
		if ok && len(block) == 2 && err == nil {
			content = append(content, object)
		} else {
			content = append(content, value)
		}
	}
	result["content"] = content
	// Remove structured data only after proving semantic JSON equality with a
	// textual block. Brief prose and a distinct structured evidence object are
	// both meaningful. Oversized unique data is clipped explicitly with lookup.
	for _, key := range []string{"structured_content", "structuredContent"} {
		if structured, exists := output[key]; exists {
			duplicate := false
			for _, block := range content {
				if reflect.DeepEqual(block, structured) {
					duplicate = true
					break
				}
			}
			if !duplicate {
				result[key] = structured
			}
		}
	}
	return result
}

func nonTextMetadata(value any) []any {
	object, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	blocks, ok := object["content"].([]any)
	if !ok {
		return nil
	}
	result := []any{}
	for _, value := range blocks {
		block, ok := value.(map[string]any)
		if !ok || block["type"] == nil || block["type"] == "text" {
			continue
		}
		metadata := map[string]any{}
		for key, value := range block {
			if key == "data" {
				metadata["data_omitted"] = true
				continue
			}
			if key == "resource" {
				if resource, ok := value.(map[string]any); ok {
					selected := map[string]any{}
					for field, payload := range resource {
						if field == "text" || field == "blob" {
							selected[field+"_omitted"] = true
						} else {
							selected[field] = payload
						}
					}
					metadata[key] = selected
					continue
				}
			}
			metadata[key] = value
		}
		result = append(result, metadata)
	}
	return result
}

// protocolControls preserves nested paths, including bound arguments on returned
// continuation actions. Free prose is omitted; control state is never shortened.
func protocolControls(value any) any { return collectProtocolControls(value, true) }

// envelopeProtocolControls skips record collections such as a server's tool
// catalogue, whose schemas reuse control names like status or scope as data.
// MCP content blocks remain reachable; control-valued keys are kept whole.
func envelopeProtocolControls(value any) any { return collectProtocolControls(value, false) }

func collectProtocolControls(value any, records bool) any {
	switch object := value.(type) {
	case map[string]any:
		out := map[string]any{}
		for key, nested := range object {
			switch key {
			case "protocol_version", "context_id", "agent_id", "continuation", "scope", "current_about", "about", "abouts", "as_of", "axis", "interval", "is_error", "isError", "uncertain", "status", "error", "error_code", "next_action", "next_actions", "has_more", "next_cursor", "guide_revision", "guide_revision_seen", "next_offset_bytes", "offset_bytes", "total_bytes", "message_index":
				out[key] = nested
			case "agent":
				if agent, ok := nested.(map[string]any); ok {
					if id, exists := agent["id"]; exists {
						out[key] = map[string]any{"id": id}
					}
				}
			case "text":
				if text, ok := nested.(string); ok {
					if parsed, err := decodeContextJSON([]byte(text)); err == nil {
						if controls := collectProtocolControls(parsed, records); controls != nil {
							out["text_controls"] = controls
						}
					}
				}
			default:
				if _, collection := nested.([]any); collection && !records && key != "content" {
					continue
				}
				if controls := collectProtocolControls(nested, records); controls != nil {
					out[key] = controls
				}
			}
		}
		if len(out) > 0 {
			return out
		}
	case []any:
		out := []any{}
		for _, nested := range object {
			if controls := collectProtocolControls(nested, records); controls != nil {
				out = append(out, controls)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

// Decode numeric literals exactly. Re-emitting an MCP result must not round its
// sequence IDs, timestamps or proof values through float64, nor confuse two
// unequal structured values during duplicate detection.
func decodeContextJSON(raw []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("extra JSON data")
		}
		return nil, err
	}
	return value, nil
}

func protocolIdentities(value any) any {
	switch object := value.(type) {
	case map[string]any:
		out := map[string]any{}
		for key, nested := range object {
			switch key {
			case "context_id", "agent_id":
				out[key] = nested
			case "agent":
				if agent, ok := nested.(map[string]any); ok {
					if id, exists := agent["id"]; exists {
						out[key] = map[string]any{"id": id}
					}
				}
			default:
				if identities := protocolIdentities(nested); identities != nil {
					out[key] = identities
				}
			}
		}
		if len(out) > 0 {
			return out
		}
	case []any:
		out := []any{}
		for _, nested := range object {
			if identities := protocolIdentities(nested); identities != nil {
				out = append(out, identities)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}
