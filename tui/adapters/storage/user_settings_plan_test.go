package storage

import "testing"

// The plan model defaults to the session's: the former default,
// z-ai/glm-5.3-flash, took 138 s for one decompose request.
func TestPlannerDefaultsToTheSessionModel(t *testing.T) {
	for _, tc := range []struct {
		plan *PlanSettings
		want string
	}{
		{nil, ""},
		{&PlanSettings{}, ""},
		{&PlanSettings{AutoApprove: true}, ""},
		{&PlanSettings{Model: "session"}, ""},
		{&PlanSettings{Model: "z-ai/glm-5.3-flash"}, "z-ai/glm-5.3-flash"},
	} {
		if got := (UserSettings{Plan: tc.plan}).Planner(); got != tc.want {
			t.Fatalf("plan %+v: planner = %q, want %q", tc.plan, got, tc.want)
		}
	}
}
