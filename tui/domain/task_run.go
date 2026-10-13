package domain

// TaskRun is what a task ceremony carries: the task's contract and the
// digests the console compares against. The console owns it.
type TaskRun struct {
	Plan, Task string
	Scope      []string
	Protect    []string
	Check      CheckCommand
	TestFirst  bool
	// Start holds the SHA-256 of every scope, protected and already dirty
	// file when the task started; "" for a file that did not exist.
	Start map[string]string
	// Frozen holds the digests of the test files red named; green must not
	// change them.
	Frozen map[string]string
	// Git is false when the workspace is not a repository: only the digests
	// are enforced and the hand-back says so.
	Git bool
}

// Clone copies the task run, maps included.
func (t TaskRun) Clone() TaskRun {
	t.Scope = append([]string(nil), t.Scope...)
	t.Protect = append([]string(nil), t.Protect...)
	t.Check.Args = append([]string(nil), t.Check.Args...)
	t.Start = cloneDigests(t.Start)
	t.Frozen = cloneDigests(t.Frozen)
	return t
}

func cloneDigests(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
