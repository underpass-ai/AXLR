package application

import (
	"context"
	"encoding/json"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// localListingRetries bounds how many times a page is asked again shorter.
const localListingRetries = 3

// boundedLocalListing runs a local_search or local_list whose page the
// projection keeps whole under a limit of limit bytes per tool result, as
// boundedLocalRead does for a file: an omitted max_bytes, or a larger one,
// becomes localReadPageBytes(limit), and a page whose JSON, escaped again
// by the projection, still overflows is asked again with a max_bytes
// shorter in proportion. The runtime then ends the page earlier and its
// next_offset is where the results continue, where an excerpt would have
// cut matches out of the middle. Searching and listing have no effect, so
// asking again is safe; arguments the runtime refuses are passed unchanged.
func boundedLocalListing(ctx context.Context, tools ToolExecutionPort, id domain.ToolIdentity, arguments root.JSONValue, limit int) (domain.ToolOutcome, error) {
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
	for attempt := 0; ; attempt++ {
		outcome, err := tools.Execute(ctx, id, args)
		if err != nil || outcome.IsError || attempt == localListingRetries || toolResultFits(string(outcome.Content), limit) {
			return outcome, err
		}
		_, _, whole := wholeToolContent(string(outcome.Content))
		shorter := page * (limit - localReadEnvelopeBytes) / contentJSONBytes(whole)
		if shorter < 1 || shorter >= page {
			return outcome, nil
		}
		page, args = shorter, withMaxBytes(call, shorter)
	}
}
