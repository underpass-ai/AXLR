package domain

import (
	"errors"
	"unicode/utf8"
)

type RequestID string

func NewRequestID(s string) (RequestID, error) {
	if len(s) == 0 || len(s) > 128 || !utf8.ValidString(s) {
		return "", errors.New("invalid request_id")
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return "", errors.New("invalid request_id")
		}
	}
	return RequestID(s), nil
}
