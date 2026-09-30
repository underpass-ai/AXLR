package dto

type MCPPluginConfig struct {
	Name        string            `json:"name,omitempty"`
	Description string            `json:"description,omitempty"`
	Purpose     string            `json:"purpose,omitempty"`
	Approval    string            `json:"approval,omitempty"`
	Manifest    string            `json:"manifest"`
	Env         map[string]string `json:"env,omitempty"`
	EnvFrom     map[string]string `json:"env_from,omitempty"`
}
