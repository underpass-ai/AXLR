package domain

import "errors"

// PluginPurpose describes the capability a registered plugin contributes.
type PluginPurpose string

const (
	PluginPurposeTools    PluginPurpose = "tools"
	PluginPurposeMemory   PluginPurpose = "memory"
	PluginPurposeCeremony PluginPurpose = "ceremony"
)

func (p PluginPurpose) Validate() error {
	switch p {
	case PluginPurposeTools, PluginPurposeMemory, PluginPurposeCeremony:
		return nil
	}
	return errors.New("invalid plugin purpose")
}
