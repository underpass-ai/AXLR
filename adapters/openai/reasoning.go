package openai

import "encoding/json"

// reasoningConfig asks for readable summaries of the reasoning, so the
// console can show it as it streams; the effort stays the model's default.
var reasoningConfig = json.RawMessage(`{"summary":"auto"}`)

// encryptedReasoning asks for the reasoning items encrypted, the only form a
// stateless request can send back: without it a replayed item refers to
// state the server did not keep.
var encryptedReasoning = []string{"reasoning.encrypted_content"}
