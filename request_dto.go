package axlr

import "encoding/json"

type Request struct {
	ProtocolVersion int             `json:"protocol_version"`
	RequestID       string          `json:"request_id"`
	Tool            string          `json:"tool"`
	Arguments       json.RawMessage `json:"arguments"`
}
