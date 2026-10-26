package contract

import (
	"encoding/json"
	"errors"
	"io"

	"github.com/underpass-ai/AXLR/tui/domain"
	"github.com/underpass-ai/AXLR/tui/dto"
)

// MaxExecutionSkillsBytes bounds the skills a surface reads: at most
// domain.MaxSkillPins pins.
const MaxExecutionSkillsBytes = 16 << 10

// ParseExecutionSkills reads the contract's skills, strictly: a list of
// pins, each its name, version and optionally catalog, with no other
// field. The list may come wrapped as {"skills": [...]}. The pins are
// requests until they are resolved against the installed skills, which
// fills the catalog and the bundle digest.
func ParseExecutionSkills(raw []byte) ([]dto.ExecutionSkillPin, error) {
	if len(raw) > MaxExecutionSkillsBytes {
		return nil, errors.New("the skills exceed 16 KiB")
	}
	var wrapped struct {
		Skills json.RawMessage `json:"skills"`
	}
	if decoder := strictDecoder(raw); decoder.Decode(&wrapped) == nil && wrapped.Skills != nil {
		raw = wrapped.Skills
	}
	var wire []dto.ExecutionSkillPin
	decoder := strictDecoder(raw)
	if err := decoder.Decode(&wire); err != nil || wire == nil {
		return nil, errors.New("the skills must be the contract's list of pins: name, version and catalog, with no other field")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, errors.New("trailing data after the skills")
	}
	return CheckSkillRequests(wire)
}

// CheckSkillRequests validates requested pins before they are resolved:
// a name and a version each, no bundle digest (the install supplies it),
// at most domain.MaxSkillPins.
func CheckSkillRequests(wire []dto.ExecutionSkillPin) ([]dto.ExecutionSkillPin, error) {
	if len(wire) > domain.MaxSkillPins {
		return nil, errors.New("an execution pins at most 32 skills")
	}
	for _, pin := range wire {
		if pin.BundleSHA256 != "" {
			return nil, errors.New("a requested skill pin carries no bundle_sha256: the installed skill supplies it")
		}
		probe := domain.SkillPin{Catalog: pin.Catalog, Name: pin.Name, Version: pin.Version, BundleSHA256: zeroDigest}
		if probe.Catalog == "" {
			probe.Catalog = "any"
		}
		if err := probe.Validate(); err != nil {
			return nil, err
		}
	}
	return append([]dto.ExecutionSkillPin{}, wire...), nil
}

const zeroDigest = "0000000000000000000000000000000000000000000000000000000000000000"

// DomainExecutionSkills validates resolved wire pins, as a store kept
// them, into the domain's.
func DomainExecutionSkills(wire []dto.ExecutionSkillPin) (domain.ExecutionSkills, error) {
	skills := domain.ExecutionSkills{Pins: make([]domain.SkillPin, 0, len(wire))}
	for _, pin := range wire {
		skills.Pins = append(skills.Pins, domain.SkillPin{Catalog: pin.Catalog, Name: pin.Name, Version: pin.Version, BundleSHA256: pin.BundleSHA256})
	}
	return skills, skills.Validate()
}

// ExecutionSkills is the wire form of resolved pins, as GetSession shows
// them and the receipt digests them: never null.
func ExecutionSkills(skills domain.ExecutionSkills) []dto.ExecutionSkillPin {
	wire := make([]dto.ExecutionSkillPin, 0, len(skills.Pins))
	for _, pin := range skills.Pins {
		wire = append(wire, dto.ExecutionSkillPin{Name: pin.Name, Version: pin.Version, Catalog: pin.Catalog, BundleSHA256: pin.BundleSHA256})
	}
	return wire
}
