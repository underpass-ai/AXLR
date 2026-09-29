package domain

import "errors"

type ByteOffset int64

func NewByteOffset(n int64) (ByteOffset, error) {
	if n < 0 {
		return 0, errors.New("byte offset cannot be negative")
	}
	return ByteOffset(n), nil
}
