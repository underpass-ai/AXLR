package domain

type UIPreferences struct {
	Theme        ThemeID
	Icons        IconProfile
	ReduceMotion bool
	// NaturalWriting types pastes into the composer at a human pace, for
	// recording demos (/natural-writing).
	NaturalWriting bool
}

func DefaultUIPreferences() UIPreferences {
	return UIPreferences{Theme: ThemeAuto, Icons: IconsSafe}
}

func (p UIPreferences) Validate() error {
	if err := p.Theme.Validate(); err != nil {
		return err
	}
	return p.Icons.Validate()
}
