package domain

import (
	"errors"
	"fmt"
	"regexp"
)

// ExecutionSkills pins the catalog skills of one execution
// (docs/runtime-contract.md, "Skills per execution"): with them, the
// execution's catalog skills are exactly these, at these versions; a
// session without them reads every installed catalog skill at its newest
// installed version. Built-in and plugin skills are not pinned.
type ExecutionSkills struct {
	Pins []SkillPin
}

// SkillPin is one catalog skill at one version, resolved against the
// installed skills when the execution began: Catalog is never empty and
// BundleSHA256 is the digest of the bundle that was installed.
type SkillPin struct {
	Catalog, Name, Version, BundleSHA256 string
}

// MaxSkillPins bounds an execution's skills.
const MaxSkillPins = 32

var (
	skillPinCatalog = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
	skillPinName    = regexp.MustCompile(`^[a-z][a-z0-9_]{1,47}$`)
	skillPinVersion = regexp.MustCompile(`^(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})$`)
	skillPinDigest  = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

func (p SkillPin) Validate() error {
	if !skillPinCatalog.MatchString(p.Catalog) {
		return fmt.Errorf("skill pin %s: catalog must be a configured catalog's name", p.Name)
	}
	if !skillPinName.MatchString(p.Name) {
		return fmt.Errorf("skill pin %q: name must be 2-48 lowercase letters, digits or underscores, starting with a letter", p.Name)
	}
	if !skillPinVersion.MatchString(p.Version) {
		return fmt.Errorf("skill pin %s: version must be major.minor.patch, such as 1.0.0", p.Name)
	}
	if !skillPinDigest.MatchString(p.BundleSHA256) {
		return fmt.Errorf("skill pin %s: the bundle digest must be 64 lowercase hex digits", p.Name)
	}
	return nil
}

// Validate refuses more than MaxSkillPins pins and a skill pinned twice
// in one catalog; no pin at all is valid: an execution with no catalog
// skill.
func (s ExecutionSkills) Validate() error {
	if len(s.Pins) > MaxSkillPins {
		return fmt.Errorf("an execution pins at most %d skills", MaxSkillPins)
	}
	seen := map[string]bool{}
	for _, pin := range s.Pins {
		if err := pin.Validate(); err != nil {
			return err
		}
		key := pin.Catalog + "/" + pin.Name
		if seen[key] {
			return fmt.Errorf("skill %s of catalog %s is pinned twice", pin.Name, pin.Catalog)
		}
		seen[key] = true
	}
	return nil
}

// Clone copies the pins.
func (s ExecutionSkills) Clone() ExecutionSkills {
	return ExecutionSkills{Pins: append([]SkillPin(nil), s.Pins...)}
}

// Pin is the pinned version of a catalog's skill, if the execution pins it.
func (s ExecutionSkills) Pin(catalog, name string) (SkillPin, bool) {
	for _, pin := range s.Pins {
		if pin.Catalog == catalog && pin.Name == name {
			return pin, true
		}
	}
	return SkillPin{}, false
}

// Skills is the execution's pinned skills, if the session has them.
func (s Session) Skills() (ExecutionSkills, bool) {
	if s.state.Skills == nil {
		return ExecutionSkills{}, false
	}
	return s.state.Skills.Clone(), true
}

// SetSkills pins the session's catalog skills from now on: this
// execution's and every later turn's or decision's.
func (s *Session) SetSkills(skills ExecutionSkills) error {
	if err := skills.Validate(); err != nil {
		return err
	}
	clone := skills.Clone()
	if clone.Pins == nil {
		// No pin is a pin: the execution reads no catalog skill.
		clone.Pins = []SkillPin{}
	}
	s.state.Skills = &clone
	return nil
}

// ClearSkills lifts the pins: the newest installed versions again.
func (s *Session) ClearSkills() { s.state.Skills = nil }

// ErrSkillsUnavailable wraps a pin this host cannot serve.
var ErrSkillsUnavailable = errors.New("the execution's skills are unavailable here")
