package application

// DiagnosticEvent carries numeric measurements and allowlisted labels only.
// In particular, never add raw text, errors, model IDs, paths, or tool names.
type DiagnosticEvent struct {
	OriginalMessages    int                  `json:"original_messages,omitempty"`
	DroppedMessages     int                  `json:"dropped_messages,omitempty"`
	OriginalBytes       int                  `json:"original_bytes,omitempty"`
	ProjectedBytes      int                  `json:"projected_bytes,omitempty"`
	ContextCutIndex     int                  `json:"context_cut_index,omitempty"`
	MessageBytes        int                  `json:"message_bytes,omitempty"`
	ToolSchemaBytes     int                  `json:"tool_schema_bytes,omitempty"`
	Endpoint            DiagnosticEndpoint   `json:"endpoint,omitempty"`
	ToolOrdinal         int                  `json:"tool_ordinal,omitempty"`
	PluginOrdinal       int                  `json:"plugin_ordinal,omitempty"`
	Action              DiagnosticAction     `json:"action,omitempty"`
	SpanID              uint64               `json:"span_id,omitempty"`
	ParentSpanID        uint64               `json:"parent_span_id,omitempty"`
	ElapsedMicroseconds int64                `json:"elapsed_us,omitempty"`
	RequestID           uint64               `json:"request_id,omitempty"`
	Messages            int                  `json:"messages,omitempty"`
	Tools               int                  `json:"tools,omitempty"`
	HTTPStatus          int                  `json:"http_status,omitempty"`
	Stage               DiagnosticStage      `json:"stage"`
	OperationID         uint64               `json:"operation_id,omitempty"`
	Chunks              int                  `json:"chunks,omitempty"`
	Bytes               int                  `json:"bytes,omitempty"`
	ElapsedMilliseconds int64                `json:"elapsed_ms,omitempty"`
	Width               int                  `json:"width,omitempty"`
	Height              int                  `json:"height,omitempty"`
	ErrorClass          DiagnosticErrorClass `json:"error_class,omitempty"`
}
