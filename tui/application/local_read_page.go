package application

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// localReadEnvelopeBytes is what a local_read page takes in the projection
// besides its text: the projection's 256-byte margin, and the result's
// offsets, status and digest, which encode to less than 256 bytes.
const localReadEnvelopeBytes = 512

// localReadPageBytes is the largest local_read page whose result fits a
// limit of limit bytes per tool result when its text needs no JSON escaping.
func localReadPageBytes(limit int) int {
	return max(limit-localReadEnvelopeBytes, 1)
}

// boundedLocalRead runs a local_read whose page the projection keeps whole
// under a limit of limit bytes per tool result. An omitted max_bytes, or a
// larger one, becomes localReadPageBytes(limit), and a page whose escaped
// JSON still overflows is read again shorter, so the next_offset_bytes the
// model receives is where the file continues. Measured with
// z-ai/glm-5.3-flash under the default prompt budget: a 30 KB file read
// without max_bytes came back as an excerpt whose next_offset_bytes was the
// end of the file, and the model spent several requests re-reading
// overlapping ranges. A read has no effect, so reading again is safe;
// arguments the runtime refuses are passed unchanged.
func boundedLocalRead(ctx context.Context, tools ToolExecutionPort, id domain.ToolIdentity, arguments root.JSONValue, limit int) (domain.ToolOutcome, error) {
	var call map[string]json.RawMessage
	if json.Unmarshal(arguments.Bytes(), &call) != nil {
		return tools.Execute(ctx, id, arguments)
	}
	page, requested := localReadPageBytes(limit), 0
	if raw, ok := call["max_bytes"]; ok && json.Unmarshal(raw, &requested) != nil {
		return tools.Execute(ctx, id, arguments)
	}
	args := arguments
	switch {
	case requested < 0:
		return tools.Execute(ctx, id, arguments)
	case requested > 0 && requested <= page:
		page = requested
	default:
		args = withMaxBytes(call, page)
	}
	for {
		outcome, err := tools.Execute(ctx, id, args)
		if err != nil || outcome.IsError || toolResultFits(string(outcome.Content), limit) {
			return outcome, err
		}
		_, semantic, whole := wholeToolContent(string(outcome.Content))
		read, ok := localReadPageOf(semantic)
		if !ok {
			return outcome, nil
		}
		// Each byte of text takes at least one byte of the result, so a page
		// shorter by the overflow fits. When escaping is even, a page shorter
		// in proportion fits as well and is longer; text escaped to several
		// times its size leaves only that one.
		measured := contentJSONBytes(whole)
		shorter := len(read.content) * localReadPageBytes(limit) / measured
		if exact := len(read.content) - (measured - (limit - 256)); exact > shorter {
			shorter = exact
		}
		if shorter < 1 || shorter >= page {
			return outcome, nil
		}
		page, args = shorter, withMaxBytes(call, shorter)
	}
}

func withMaxBytes(call map[string]json.RawMessage, page int) root.JSONValue {
	paged := make(map[string]json.RawMessage, len(call)+1)
	for key, value := range call {
		paged[key] = value
	}
	paged["max_bytes"], _ = json.Marshal(page)
	encoded, _ := json.Marshal(paged)
	value, _ := root.NewJSONObject(encoded)
	return value
}

// localReadPage is the text a completed local_read returned and the file
// offset it starts at.
type localReadPage struct {
	start   int64
	content string
}

// localReadPageOf finds the page in the projection's view of a result.
func localReadPageOf(semantic any) (localReadPage, bool) {
	result, ok := semantic.(map[string]any)
	if !ok || result["status"] != "completed" {
		return localReadPage{}, false
	}
	content, isText := result["content"].(string)
	start, isNumber := result["start_offset_bytes"].(json.Number)
	if !isText || !isNumber {
		return localReadPage{}, false
	}
	offset, err := start.Int64()
	if err != nil || offset < 0 {
		return localReadPage{}, false
	}
	return localReadPage{start: offset, content: content}, true
}

// end is the file offset after the page.
func (p localReadPage) end() int64 { return p.start + int64(len(p.content)) }

// resume is the first file byte that excerpt, a prefix of the page's JSON,
// does not show whole. That JSON starts with the text, since its keys are
// sorted, and escapes it as json.Marshal escapes a string.
func (p localReadPage) resume(excerpt string) int64 {
	const prefix = `{"content":`
	if !strings.HasPrefix(excerpt, prefix+`"`) {
		return p.start
	}
	// The largest character boundary whose escaped text, without its
	// closing quote, still fits in the excerpt.
	boundary := func(k int) int {
		for k > 0 && k < len(p.content) && !utf8.RuneStart(p.content[k]) {
			k--
		}
		return k
	}
	beyond := sort.Search(len(p.content)+1, func(k int) bool {
		escaped, _ := json.Marshal(p.content[:boundary(k)])
		return len(prefix)+len(escaped)-1 > len(excerpt)
	})
	return p.start + int64(boundary(beyond-1))
}

// localReadRetrieval tells the model how to read what an excerpted
// local_read page omits. local_read has no query or filter: the rest is read
// from the file, from offset, in pages the projection keeps whole. The text
// depends only on the result and the limit, so it reads the same whether its
// turn is open or closed.
func localReadRetrieval(offset int64, limit int) string {
	return fmt.Sprintf("This local_read page is larger than the model context keeps per tool result, and its next_offset_bytes follows the whole page, not this excerpt. To read what the excerpt omits, call local_read again with the same path, offset_bytes: %d and max_bytes of at most %d; continue with the next_offset_bytes that call returns.", offset, localReadPageBytes(limit))
}
