package dto

type MCPPluginConfig struct {
	Manifest string            `json:"manifest"`
	Env      map[string]string `json:"env,omitempty"`
	EnvFrom  map[string]string `json:"env_from,omitempty"`
}
