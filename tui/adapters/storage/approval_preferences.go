package storage

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/underpass-ai/AXLR/tui/domain"
)

type ApprovalPreferences struct {
	Autonomous bool                  `json:"autonomous"`
	Allowed    []domain.ToolIdentity `json:"allowed"`
	// AlwaysAsk names the destructive rules that keep a card under
	// autonomy (domain.DestructiveRules); absent means every rule.
	AlwaysAsk []string                   `json:"always_ask,omitempty"`
	Extra     map[string]json.RawMessage `json:"-"`
}

func (p *ApprovalPreferences) UnmarshalJSON(data []byte) error {
	type known ApprovalPreferences
	var value known
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return errors.New("approvals must be a JSON object")
	}
	for key := range fields {
		if strings.EqualFold(key, "autonomous") || strings.EqualFold(key, "allowed") {
			delete(fields, key)
		}
	}
	*p = ApprovalPreferences(value)
	p.Extra = fields
	return nil
}

func (p ApprovalPreferences) MarshalJSON() ([]byte, error) {
	type known ApprovalPreferences
	data, err := json.Marshal(known(p))
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	for key, value := range p.Extra {
		if _, owned := fields[key]; !owned {
			fields[key] = value
		}
	}
	return json.Marshal(fields)
}
