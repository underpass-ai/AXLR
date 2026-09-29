package domain

import "errors"

// ContextWindow is a model's maximum context size in tokens. Zero means unknown.
type ContextWindow int

func NewContextWindow(tokens int) (ContextWindow, error) {
	if tokens <= 0 {
		return 0, errors.New("context window must be positive")
	}
	return ContextWindow(tokens), nil
}

func (c ContextWindow) Tokens() int { return int(c) }
