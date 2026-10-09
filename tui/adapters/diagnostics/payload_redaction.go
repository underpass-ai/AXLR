package diagnostics

import (
	"bytes"
	"encoding/json"
	"io"
	"regexp"
	"sort"
	"strconv"
)

var redactedMarker = []byte("[redacted]")

// credentialRule finds one family of credentials. hint is a cheap test a
// payload must pass before the pattern scans it; group is the submatch that
// holds the secret, 0 for the whole match.
type credentialRule struct {
	hint    func([]byte) bool
	pattern *regexp.Regexp
	group   int
}

func containsAny(literals ...string) func([]byte) bool {
	return func(body []byte) bool {
		for _, literal := range literals {
			if bytes.Contains(body, []byte(literal)) {
				return true
			}
		}
		return false
	}
}

var credentialRules = []credentialRule{
	// Authorization values and OpenAI-style keys. Both begin with b/B or the
	// Unicode case-fold set s/S/ſ.
	{
		hint:    func(body []byte) bool { return bytes.IndexAny(body, "bBsS") >= 0 || bytes.Contains(body, []byte("ſ")) },
		pattern: regexp.MustCompile(`(?i)(bearer\s+)[a-z0-9._~+/=-]+|\bsk-[a-z0-9_-]{16,}`),
	},
	// GitHub personal, OAuth, user-to-server, server-to-server and refresh
	// tokens, and fine-grained personal access tokens.
	{
		hint:    containsAny("ghp_", "gho_", "ghs_", "ghu_", "ghr_", "github_pat_"),
		pattern: regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{20,}`),
	},
	// AWS access key ids, long-term and temporary.
	{
		hint:    containsAny("AKIA", "ASIA"),
		pattern: regexp.MustCompile(`(?:AKIA|ASIA)[A-Z0-9]{16}`),
	},
	// AWS secret access keys and session tokens have no prefix: they are
	// found by the name they are assigned to in an environment, a
	// credentials file or JSON (escaped quotes included), and only the
	// value is redacted.
	{
		hint:    containsAny("ecret_access_key", "ECRET_ACCESS_KEY", "ecretAccessKey", "ession_token", "ESSION_TOKEN", "essionToken"),
		pattern: regexp.MustCompile(`(?i)(?:secret_?access_?key|session_?token)\\?["']?\s*[:=]\s*\\?["']?([A-Za-z0-9/+=]{40,})`),
		group:   1,
	},
	// Slack bot, user, app-level, refresh and configuration tokens.
	{
		hint:    containsAny("xox"),
		pattern: regexp.MustCompile(`xox[abeprs]-[A-Za-z0-9-]{10,}`),
	},
	// PEM and OpenSSH private keys, through their END line or, when the key
	// was cut short, to the end of the JSON string that holds it.
	{
		hint:    containsAny("PRIVATE KEY"),
		pattern: regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY[A-Z ]*-----(?:(?:[^"\\]|\\.)*?-----END [A-Z0-9 ]*PRIVATE KEY[A-Z ]*-----|(?:[^"\\]|\\.)*)`),
	},
}

// secretSpans returns the byte ranges of the configured secrets and the
// recognised credentials in text, sorted, with overlapping ranges merged.
func (p *PayloadRecorder) secretSpans(text []byte) [][2]int {
	var spans [][2]int
	for _, secret := range p.secrets {
		if secret == "" {
			continue
		}
		for offset := 0; ; {
			index := bytes.Index(text[offset:], []byte(secret))
			if index < 0 {
				break
			}
			start := offset + index
			offset = start + len(secret)
			spans = append(spans, [2]int{start, offset})
		}
	}
	for _, rule := range credentialRules {
		if !rule.hint(text) {
			continue
		}
		if rule.group == 0 {
			for _, match := range rule.pattern.FindAllIndex(text, -1) {
				spans = append(spans, [2]int{match[0], match[1]})
			}
			continue
		}
		for _, match := range rule.pattern.FindAllSubmatchIndex(text, -1) {
			if start, end := match[2*rule.group], match[2*rule.group+1]; start >= 0 && end > start {
				spans = append(spans, [2]int{start, end})
			}
		}
	}
	if len(spans) == 0 {
		return nil
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i][0] < spans[j][0] })
	merged := spans[:1]
	for _, span := range spans[1:] {
		last := &merged[len(merged)-1]
		if span[0] < last[1] {
			last[1] = max(last[1], span[1])
			continue
		}
		merged = append(merged, span)
	}
	return merged
}

// redact replaces every secret span in body with the marker.
func (p *PayloadRecorder) redact(body []byte) []byte {
	spans := p.secretSpans(body)
	if len(spans) == 0 {
		return body
	}
	out := make([]byte, 0, len(body))
	last := 0
	for _, span := range spans {
		out = append(out, body[last:span[0]]...)
		out = append(out, redactedMarker...)
		last = span[1]
	}
	return append(out, body[last:]...)
}

// streamFrame is one decoded SSE data line of a capture.
type streamFrame struct {
	start, end int // the JSON payload's bytes in the capture
	value      map[string]any
	edits      map[int][]streamEdit // by leaf ordinal
}

type streamEdit struct {
	start, end int
	text       string
}

// streamLeaf is one string a frame contributes to a channel's joined text.
type streamLeaf struct{ frame, ordinal, start, end int }

type streamChannel struct {
	text   []byte
	leaves []streamLeaf
}

// redactStream removes secrets that a stream splits across chunks, such as
// a configured key the model echoes as "sk-or" in one delta and the rest in
// the next, where no single chunk matches. Each string under a frame's
// choices (content, reasoning, tool call arguments) is joined with the same
// field of the other frames in wire order, keyed by choice and tool call
// index, and the joined text is searched. Every frame holding part of a
// secret loses that part: the first part becomes the marker, later parts
// become empty. Frames without a secret keep their bytes; edited frames are
// re-encoded, so their field order may change. A data line that is not one
// JSON object (such as [DONE]) is left to the byte-level pass.
func (p *PayloadRecorder) redactStream(body []byte) []byte {
	var frames []streamFrame
	channels := map[string]*streamChannel{}
	for lineStart := 0; lineStart < len(body); {
		lineEnd := bytes.IndexByte(body[lineStart:], '\n')
		next := lineStart + lineEnd + 1
		if lineEnd < 0 {
			lineEnd, next = len(body)-lineStart, len(body)
		}
		line := bytes.TrimSuffix(body[lineStart:lineStart+lineEnd], []byte("\r"))
		if payload, ok := bytes.CutPrefix(line, []byte("data:")); ok {
			start := lineStart + len(line) - len(payload)
			if bytes.HasPrefix(payload, []byte(" ")) {
				payload, start = payload[1:], start+1
			}
			if value, ok := decodeStreamFrame(payload); ok {
				index := len(frames)
				frames = append(frames, streamFrame{start: start, end: start + len(payload), value: value})
				ordinal := 0
				walkStreamStrings(value["choices"], "choices", func(path, text string) string {
					channel := channels[path]
					if channel == nil {
						channel = &streamChannel{}
						channels[path] = channel
					}
					channel.leaves = append(channel.leaves, streamLeaf{frame: index, ordinal: ordinal, start: len(channel.text), end: len(channel.text) + len(text)})
					channel.text = append(channel.text, text...)
					ordinal++
					return text
				})
			}
		}
		lineStart = next
	}
	edited := false
	for _, channel := range channels {
		next := 0
		for _, span := range p.secretSpans(channel.text) {
			for next < len(channel.leaves) && channel.leaves[next].end <= span[0] {
				next++
			}
			first := true
			for _, leaf := range channel.leaves[next:] {
				if leaf.start >= span[1] {
					break
				}
				if leaf.end <= span[0] || leaf.start == leaf.end {
					continue
				}
				replacement := ""
				if first {
					replacement, first = string(redactedMarker), false
				}
				frame := &frames[leaf.frame]
				if frame.edits == nil {
					frame.edits = map[int][]streamEdit{}
				}
				frame.edits[leaf.ordinal] = append(frame.edits[leaf.ordinal], streamEdit{start: max(span[0], leaf.start) - leaf.start, end: min(span[1], leaf.end) - leaf.start, text: replacement})
				edited = true
			}
		}
	}
	if !edited {
		return body
	}
	out := make([]byte, 0, len(body))
	last := 0
	for _, frame := range frames {
		if frame.edits == nil {
			continue
		}
		ordinal := 0
		frame.value["choices"] = walkStreamStrings(frame.value["choices"], "choices", func(_, text string) string {
			edits := frame.edits[ordinal]
			ordinal++
			if len(edits) == 0 {
				return text
			}
			var b []byte
			cursor := 0
			for _, edit := range edits {
				b = append(b, text[cursor:edit.start]...)
				b = append(b, edit.text...)
				cursor = edit.end
			}
			return string(append(b, text[cursor:]...))
		})
		var encoded bytes.Buffer
		encoder := json.NewEncoder(&encoded)
		encoder.SetEscapeHTML(false)
		if encoder.Encode(frame.value) != nil {
			// Unreachable for decoded JSON; drop the frame rather than keep
			// a secret.
			encoded.Reset()
			encoded.Write(redactedMarker)
		}
		out = append(out, body[last:frame.start]...)
		out = append(out, bytes.TrimSuffix(encoded.Bytes(), []byte("\n"))...)
		last = frame.end
	}
	return append(out, body[last:]...)
}

func decodeStreamFrame(payload []byte) (map[string]any, bool) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var value map[string]any
	if decoder.Decode(&value) != nil || value == nil {
		return nil, false
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, false
	}
	return value, true
}

// walkStreamStrings visits every string under value in a fixed order and
// stores what visit returns. The path names an array element by its
// "index" field when it has one, so a choice's or a tool call's fragments
// share a path across frames whatever their position.
func walkStreamStrings(value any, path string, visit func(path, text string) string) any {
	switch v := value.(type) {
	case string:
		return visit(path, v)
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			v[key] = walkStreamStrings(v[key], path+"."+key, visit)
		}
	case []any:
		for i, item := range v {
			key := strconv.Itoa(i)
			if object, ok := item.(map[string]any); ok {
				if index, ok := object["index"].(json.Number); ok {
					key = index.String()
				}
			}
			v[i] = walkStreamStrings(item, path+"["+key+"]", visit)
		}
	}
	return value
}
