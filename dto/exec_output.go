package dto

type ExecOutput struct {
	ExitCode       int    `json:"exit_code"`
	Stdout         string `json:"stdout"`
	Stderr         string `json:"stderr"`
	CapturedBytes  int    `json:"captured_bytes"`
	DiscardedBytes int64  `json:"discarded_bytes"`
	Truncated      bool   `json:"truncated"`
}
