package domain

type PluginResult struct {
	Content           []JSONValue
	StructuredContent *JSONValue
	IsError           bool
}
