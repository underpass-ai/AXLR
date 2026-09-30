package domain

import "errors"

// EnsureHostTools upgrades a legacy frozen catalog with host-only control tools.
// It cannot add effectful operations or replace an existing alias.
func (s *Session) EnsureHostTools(tools []AvailableTool) error {
	next := s.ToolSnapshot()
	for _, tool := range tools {
		if tool.Identity.Kind != ToolKindHost {
			return errors.New("catalog upgrade accepts only host tools")
		}
		found := false
		for _, previous := range next {
			if previous.Definition.Name == tool.Definition.Name {
				if previous.Identity != tool.Identity {
					return errors.New("legacy catalog conflicts with a reserved host alias")
				}
				found = true
				break
			}
		}
		if !found {
			next = append(next, tool)
		}
	}
	if err := validateTools(s.state.Model, next); err != nil {
		return err
	}
	s.state.ToolSnapshot = next
	return nil
}
