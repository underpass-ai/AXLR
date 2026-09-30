package domain

import "testing"

func TestPluginProfileValidatesTypedPolicyAndCapabilityMetadata(t *testing.T) {
	profile := PluginProfile{ID: "kmp", Name: "KMP", Description: "Graph memory", Purpose: PluginPurposeMemory, Approval: ApprovalAuto}
	if err := profile.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, purpose := range []PluginPurpose{PluginPurposeTools, PluginPurposeMemory, PluginPurposeCeremony} {
		if err := purpose.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	for _, mode := range []ApprovalMode{ApprovalManual, ApprovalAuto} {
		if err := mode.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	for _, mutate := range []func(*PluginProfile){
		func(p *PluginProfile) { p.ID = "bad/id" },
		func(p *PluginProfile) { p.Name = "bad\x00name" },
		func(p *PluginProfile) { p.Description = "bad\x00description" },
		func(p *PluginProfile) { p.Purpose = "invalid" },
		func(p *PluginProfile) { p.Approval = "invalid" },
	} {
		invalid := profile
		mutate(&invalid)
		if invalid.Validate() == nil {
			t.Fatalf("invalid profile accepted: %+v", invalid)
		}
	}
}
