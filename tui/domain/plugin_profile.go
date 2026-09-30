package domain

import root "github.com/underpass-ai/AXLR/domain"

// PluginProfile contains user-visible metadata, never process environment values.
type PluginProfile struct {
	ID          root.PluginID
	Name        root.Text
	Description root.Text
	Purpose     PluginPurpose
	Approval    ApprovalMode
}

func (p PluginProfile) Validate() error {
	if _, err := root.NewPluginID(p.ID.String()); err != nil {
		return err
	}
	if _, err := root.NewText(string(p.Name)); err != nil {
		return err
	}
	if _, err := root.NewText(string(p.Description)); err != nil {
		return err
	}
	if err := p.Purpose.Validate(); err != nil {
		return err
	}
	return p.Approval.Validate()
}
