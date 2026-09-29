package domain

type PluginTool struct {
	Ref          PluginRef
	Description  string
	InputSchema  JSONValue
	OutputSchema *JSONValue
}
