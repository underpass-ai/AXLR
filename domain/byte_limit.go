package domain

import "errors"

type ByteLimit int

func NewByteLimit(n, maximum int) (ByteLimit, error) {
	if n < 1 || n > maximum {
		return 0, errors.New("byte limit exceeds profile")
	}
	return ByteLimit(n), nil
}
