package application

// DiagnosticEvent carries numeric measurements and allowlisted labels only.
// In particular, never add raw text, errors, model IDs, paths, or tool names.
type DiagnosticEvent struct {
	Stage               DiagnosticStage      `json:"stage"`
	OperationID         uint64               `json:"operation_id,omitempty"`
	Chunks              int                  `json:"chunks,omitempty"`
	Bytes               int                  `json:"bytes,omitempty"`
	ElapsedMilliseconds int64                `json:"elapsed_ms,omitempty"`
	Width               int                  `json:"width,omitempty"`
	Height              int                  `json:"height,omitempty"`
	ErrorClass          DiagnosticErrorClass `json:"error_class,omitempty"`
}
