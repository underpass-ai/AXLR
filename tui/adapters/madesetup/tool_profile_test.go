package madesetup

import (
	"slices"
	"testing"
)

func TestWithToolProfileAddsTheFullProfileAndKeepsAnOperatorsChoice(t *testing.T) {
	env := []string{"HOME=/home/test"}
	got := WithToolProfile(env)
	if !slices.Equal(got, []string{"HOME=/home/test", "MADE_MCP_TOOL_PROFILE=full"}) {
		t.Fatalf("%v", got)
	}
	if len(env) != 1 {
		t.Fatalf("the caller's environment changed: %v", env)
	}
	chosen := []string{"MADE_MCP_TOOL_PROFILE=core+ceremony_execution", "HOME=/home/test"}
	if got := WithToolProfile(chosen); !slices.Equal(got, chosen) {
		t.Fatalf("an operator's profile was replaced: %v", got)
	}
}
