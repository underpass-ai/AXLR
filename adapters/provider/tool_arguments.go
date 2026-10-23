package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/underpass-ai/AXLR/domain"
)

// OutputLimitNote ends an answer the output limit cut short, so neither the
// person nor the model takes it for complete.
const OutputLimitNote = "[AXLR] The model's output limit cut this answer short."

// MalformedArgumentsKey names the object that stands in for a call's
// arguments when they are not a JSON object. The console's tool resolution
// (tui/application/host_tool_resolution.go) refuses a call that carries it
// with the error inside, so the model is told and can send the call again;
// keep the two names in step.
const MalformedArgumentsKey = "axlr_malformed_arguments"

// malformedArgumentsPrefixBytes bounds the part of unreadable arguments kept
// to show the model which call it was.
const malformedArgumentsPrefixBytes = 512

type malformedArgumentsDTO struct {
	Error            string `json:"error"`
	RawPrefix        string `json:"raw_prefix"`
	CutByOutputLimit bool   `json:"cut_by_output_limit,omitempty"`
}

// ToolArguments reads a call's arguments. Empty arguments are the empty
// object of a call without parameters. Arguments that are not a JSON object,
// cut short by the output limit or mistyped by the model, would fail the
// whole operation; they are kept instead as a valid object that names the
// error, for the turn to answer the call with a tool error.
func ToolArguments(raw string, cutByOutputLimit bool) (domain.JSONValue, error) {
	if strings.TrimSpace(raw) == "" {
		return domain.NewJSONObject([]byte("{}"))
	}
	if arguments, err := domain.NewJSONObject([]byte(raw)); err == nil {
		return arguments, nil
	}
	malformed := malformedArgumentsDTO{Error: "arguments must be a JSON object", RawPrefix: utf8Prefix(raw, malformedArgumentsPrefixBytes), CutByOutputLimit: cutByOutputLimit}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		malformed.Error = err.Error()
		// The offset and the text around it say what to correct: on 16
		// October 2026 claude-haiku-5.5, told only "invalid character 'a'
		// looking for beginning of value", sent the same broken call nine
		// times; the break was a bare "evidence": text past the prefix.
		var syntax *json.SyntaxError
		if errors.As(err, &syntax) && int(syntax.Offset) < len(raw) {
			malformed.Error += fmt.Sprintf(" at byte %d, near `%s`; every string value needs double quotes", syntax.Offset, malformedContext(raw, int(syntax.Offset)))
		}
	}
	encoded, err := json.Marshal(map[string]malformedArgumentsDTO{MalformedArgumentsKey: malformed})
	if err != nil {
		return domain.JSONValue{}, err
	}
	return domain.NewJSONObject(encoded)
}

// malformedContextBytes bounds the text kept on each side of the byte where
// unreadable arguments break.
const malformedContextBytes = 40

// malformedContext is the text of raw around offset, on rune boundaries.
func malformedContext(raw string, offset int) string {
	start, end := max(0, offset-malformedContextBytes), min(len(raw), offset+malformedContextBytes)
	for start > 0 && !utf8.RuneStart(raw[start]) {
		start--
	}
	for end < len(raw) && !utf8.RuneStart(raw[end]) {
		end++
	}
	return raw[start:end]
}

// utf8Prefix keeps at most maximum bytes of text on a rune boundary.
func utf8Prefix(text string, maximum int) string {
	if len(text) <= maximum {
		return text
	}
	for maximum > 0 && !utf8.RuneStart(text[maximum]) {
		maximum--
	}
	return text[:maximum]
}
