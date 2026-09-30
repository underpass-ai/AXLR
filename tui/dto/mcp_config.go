package dto

type MCPConfig struct {
	Version int               `json:"version"`
	Plugins []MCPPluginConfig `json:"plugins"`
}
