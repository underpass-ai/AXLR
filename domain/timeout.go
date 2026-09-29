package domain

import (
	"errors"
	"time"
)

type Timeout time.Duration

func NewTimeout(n time.Duration, maximum time.Duration) (Timeout, error) {
	if n < time.Millisecond || n > maximum {
		return 0, errors.New("timeout exceeds profile")
	}
	return Timeout(n), nil
}
