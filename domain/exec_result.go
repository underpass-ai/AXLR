package domain

type ExecResult struct {
	ExitCode       int
	Stdout         string
	Stderr         string
	CapturedBytes  int
	DiscardedBytes int64
	Truncated      bool
}
