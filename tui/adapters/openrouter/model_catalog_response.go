package openrouter

import (
	"encoding/json"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type modelCatalogResponse struct {
	Data json.RawMessage `json:"data"`
}

func (r modelCatalogResponse) models() ([]domain.AvailableModel, error) {
	var entries []json.RawMessage
	if len(r.Data) == 0 || string(r.Data) == "null" || json.Unmarshal(r.Data, &entries) != nil {
		return nil, errMalformedCatalog
	}
	models := make([]domain.AvailableModel, 0, len(entries))
	for _, entry := range entries {
		model, ok := mapCatalogModel(entry)
		if ok {
			models = append(models, model)
		}
	}
	return models, nil
}

func mapCatalogModel(entry json.RawMessage) (domain.AvailableModel, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(entry, &fields) != nil || fields == nil {
		return domain.AvailableModel{}, false
	}
	var id, name string
	if json.Unmarshal(fields["id"], &id) != nil || json.Unmarshal(fields["name"], &name) != nil {
		return domain.AvailableModel{}, false
	}
	var params []string
	if json.Unmarshal(fields["supported_parameters"], &params) != nil || params == nil || !containsCatalogValue(params, "tools") {
		return domain.AvailableModel{}, false
	}
	var architecture map[string]json.RawMessage
	if json.Unmarshal(fields["architecture"], &architecture) != nil || architecture == nil {
		return domain.AvailableModel{}, false
	}
	var outputs []string
	if json.Unmarshal(architecture["output_modalities"], &outputs) != nil || outputs == nil || !containsCatalogValue(outputs, "text") {
		return domain.AvailableModel{}, false
	}
	model := domain.AvailableModel{ID: root.ModelID(id), Name: root.Text(name), SupportsTools: true, TextOutput: true}
	if model.Validate() != nil {
		return domain.AvailableModel{}, false
	}
	var context int
	if json.Unmarshal(fields["context_length"], &context) == nil {
		if value, err := domain.NewContextWindow(context); err == nil {
			model.Context = value
		}
	}
	var pricing map[string]json.RawMessage
	if json.Unmarshal(fields["pricing"], &pricing) == nil {
		model.PromptRate = catalogRate(pricing["prompt"])
		model.CompletionRate = catalogRate(pricing["completion"])
	}
	return model, true
}

func containsCatalogValue(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func catalogRate(raw json.RawMessage) domain.ModelRate {
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return domain.ModelRate{}
	}
	rate, _ := domain.NewModelRate(value)
	return rate
}
