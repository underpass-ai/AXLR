package anthropic

import "encoding/json"

// thinkingBeta is the beta that lets a request say what to do with a
// thinking block whose conversation prefix changed: AXLR cuts its history
// at the context cap and drops the reasoning of earlier turns, which an
// account enforced on preserved thinking would otherwise refuse with
// HTTP 400 "Invalid `signature` in `thinking` block".
const thinkingBeta = "thinking-binding-controls-2026-08-01"

// thinkingConfig asks for adaptive thinking with readable summaries, so the
// console can show the reasoning as it streams (the default display leaves
// the thinking text empty), and drops a block the conversation no longer
// matches instead of failing the request. Models before Claude 4.6 do not
// accept adaptive thinking; their entries set thinking: false.
var thinkingConfig = json.RawMessage(`{"type":"adaptive","display":"summarized","block_binding":{"prefix_mismatch_behavior":"drop_block"}}`)
