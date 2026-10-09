package application

import "testing"

// The outputs are go test's own, captured with Go 1.26 in a module with
// packages x (no test files), x/a (TestOld) and x/b (TestNew).
func TestRanNoTestsKnowsOnlyGoTestRunsThatMatchedNothing(t *testing.T) {
	const (
		packagesNone = "ok  \tx\t0.003s [no tests to run]\nok  \tx/a\t0.003s [no tests to run]\nok  \tx/b\t0.002s [no tests to run]\n"
		packagesSome = "ok  \tx\t0.003s [no tests to run]\nok  \tx/a\t0.003s [no tests to run]\nok  \tx/b\t0.003s\n"
		directory    = "testing: warning: no tests to run\nPASS\nok  \tx/a\t0.003s\n"
		verboseNone  = "testing: warning: no tests to run\nPASS\nok  \tx/a\t0.003s [no tests to run]\n"
		verboseSome  = "testing: warning: no tests to run\nPASS\nok  \tx/a\t0.003s [no tests to run]\n=== RUN   TestNew\n--- PASS: TestNew (0.00s)\nPASS\nok  \tx/b\t0.003s\n"
		cached       = "ok  \tx/a\t(cached) [no tests to run]\n"
		jsonSome     = `{"Action":"output","Package":"x/a","Output":"testing: warning: no tests to run\n"}` + "\n" + `{"Action":"run","Package":"x/b","Test":"TestNew"}` + "\n"
	)
	for name, c := range map[string]struct {
		result CheckResult
		want   bool
	}{
		"no package ran a test":         {CheckResult{Ran: true, Stdout: packagesNone, Output: packagesNone}, true},
		"one package ran the new test":  {CheckResult{Ran: true, Stdout: packagesSome, Output: packagesSome}, false},
		"a directory with no match":     {CheckResult{Ran: true, Stdout: directory, Output: directory}, true},
		"verbose with no match":         {CheckResult{Ran: true, Stdout: verboseNone, Output: verboseNone}, true},
		"verbose where one test ran":    {CheckResult{Ran: true, Stdout: verboseSome, Output: verboseSome}, false},
		"a cached empty run":            {CheckResult{Ran: true, Stdout: cached, Output: cached}, true},
		"json where one test ran":       {CheckResult{Ran: true, Stdout: jsonSome, Output: jsonSome}, false},
		"a failing run":                 {CheckResult{Ran: true, ExitCode: 1, Stdout: packagesNone, Output: packagesNone}, false},
		"a run the runtime cut":         {CheckResult{Ran: true, Stdout: packagesNone, Output: packagesNone, Truncated: true}, false},
		"a tail whose head is gone":     {CheckResult{Ran: true, Output: "…" + packagesNone}, false},
		"another test runner":           {CheckResult{Ran: true, Stdout: "Ran 0 tests in 0.000s\n\nOK\n", Output: "Ran 0 tests in 0.000s\n\nOK\n"}, false},
		"a program that never started":  {CheckResult{ExitCode: -1, Output: "[no tests to run]"}, false},
		"a test run that printed tests": {CheckResult{Ran: true, Stdout: "PASS\nok  \tx/a\t0.003s\n", Output: "PASS\nok  \tx/a\t0.003s\n"}, false},
	} {
		if got := c.result.RanNoTests(); got != c.want {
			t.Errorf("%s: RanNoTests() = %v, want %v", name, got, c.want)
		}
	}
}
