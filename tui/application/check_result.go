package application

import "strings"

// ConsoleOutputBytes is the output cap the console asks for when it parses a
// command's output itself: the runtime's hard limit (hardFileBytes in
// runtime/config.go). An output still cut there is reported by
// CheckResult.Truncated and refused, never parsed in part.
const ConsoleOutputBytes = 1 << 20

type CheckResult struct {
	// Ran is false when the program never started (not found, timed out,
	// refused); ExitCode is then meaningless.
	Ran      bool
	ExitCode int
	// Output is the bounded tail of stdout and stderr, for showing to the model.
	Output string
	// Stdout is the whole stdout within the runtime's output cap, for callers
	// that parse it: stderr may carry notices (gh's release notice, for one)
	// that are not part of the parsed value. Empty when the runner only has
	// Output.
	Stdout string
	// Truncated is true when the runtime's output cap, which stdout and
	// stderr share, cut the output: Stdout then lacks its end, so a caller
	// that parses it must refuse it.
	Truncated bool
}

// RanNoTests reports a Go test run that exited 0 without running a test: a
// -run pattern that matches nothing passes with "[no tests to run]" on every
// package line, or with "testing: warning: no tests to run" in a single
// directory. Such a check proves nothing, so it never passes, and before a
// change it shows the improvement missing. It is narrow on purpose: only
// those Go markers with exit 0, never when one package ran a test, and never
// on output whose head or end is gone.
func (r CheckResult) RanNoTests() bool {
	if !r.Ran || r.ExitCode != 0 || r.Truncated {
		return false
	}
	text := r.Stdout
	if text == "" {
		if strings.HasPrefix(r.Output, "…") {
			return false
		}
		text = r.Output
	}
	warned := strings.Contains(text, "testing: warning: no tests to run")
	if !warned && !strings.Contains(text, "[no tests to run]") {
		return false
	}
	if strings.Contains(text, "=== RUN") || strings.Contains(text, `"Action":"run"`) {
		return false // a test started (-v or -json)
	}
	packages, empty := 0, 0
	for _, line := range strings.Split(text, "\n") {
		if !strings.HasPrefix(line, "ok ") && !strings.HasPrefix(line, "ok\t") {
			continue
		}
		packages++
		if strings.HasSuffix(strings.TrimRight(line, " \r"), "[no tests to run]") {
			empty++
		}
	}
	return packages > 0 && empty == packages || warned && packages <= 1
}

// noTestsFeedback is what the model is told when its check ran no tests.
const noTestsFeedback = "the check ran no tests: go test matched no test (\"[no tests to run]\"), so its zero exit proves nothing; add the test or fix the -run pattern so the check runs it"
