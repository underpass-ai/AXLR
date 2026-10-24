package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	"github.com/underpass-ai/AXLR/tui/domain"
	"github.com/underpass-ai/AXLR/tui/dto"
)

// MaxExecutionPolicyBytes bounds the policy object a surface reads.
const MaxExecutionPolicyBytes = 64 << 10

// ParseExecutionPolicy reads the contract's policy object, strictly: an
// unknown field or a malformed rule refuses the whole policy, as the
// service refuses unknown fields. The object may come wrapped as
// {"policy": {...}}, the way CreateSession carries it.
func ParseExecutionPolicy(raw []byte) (domain.ExecutionPolicy, error) {
	if len(raw) > MaxExecutionPolicyBytes {
		return domain.ExecutionPolicy{}, errors.New("the policy exceeds 64 KiB")
	}
	var wrapped struct {
		Policy json.RawMessage `json:"policy"`
	}
	if decoder := strictDecoder(raw); decoder.Decode(&wrapped) == nil && wrapped.Policy != nil {
		raw = wrapped.Policy
	}
	var wire dto.ExecutionPolicy
	decoder := strictDecoder(raw)
	if err := decoder.Decode(&wire); err != nil {
		return domain.ExecutionPolicy{}, errors.New("the policy must be the contract's object: level and tools.allow/tools.deny, with no other field")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return domain.ExecutionPolicy{}, errors.New("trailing data after the policy")
	}
	return DomainExecutionPolicy(wire)
}

func strictDecoder(raw []byte) *json.Decoder {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder
}

// DomainExecutionPolicy validates a wire policy into the domain's.
func DomainExecutionPolicy(wire dto.ExecutionPolicy) (domain.ExecutionPolicy, error) {
	level, err := domain.ParsePolicyLevel(wire.Level)
	if err != nil {
		return domain.ExecutionPolicy{}, err
	}
	rules := func(in []dto.ExecutionPolicyRule) ([]domain.PolicyRule, error) {
		out := make([]domain.PolicyRule, 0, len(in))
		for _, rule := range in {
			parsed, err := domain.NewPolicyRule(rule.Name, rule.Arguments)
			if err != nil {
				return nil, err
			}
			out = append(out, parsed)
		}
		return out, nil
	}
	allow, err := rules(wire.Tools.Allow)
	if err != nil {
		return domain.ExecutionPolicy{}, err
	}
	deny, err := rules(wire.Tools.Deny)
	if err != nil {
		return domain.ExecutionPolicy{}, err
	}
	return domain.NewExecutionPolicy(level, allow, deny)
}

// ExecutionPolicy is the wire form of a policy, as GetSession shows it.
func ExecutionPolicy(policy domain.ExecutionPolicy) dto.ExecutionPolicy {
	rules := func(in []domain.PolicyRule) []dto.ExecutionPolicyRule {
		if len(in) == 0 {
			return nil
		}
		out := make([]dto.ExecutionPolicyRule, 0, len(in))
		for _, rule := range in {
			out = append(out, dto.ExecutionPolicyRule{Name: rule.Name, Arguments: rule.Arguments})
		}
		return out
	}
	return dto.ExecutionPolicy{Level: string(policy.Level()), Tools: dto.ExecutionPolicyTools{Allow: rules(policy.Allow()), Deny: rules(policy.Deny())}}
}
