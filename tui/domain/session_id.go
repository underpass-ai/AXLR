package domain

import (
	"errors"
	"regexp"
)

type SessionID string

var sessionIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func NewSessionID(value string) (SessionID, error) {
	if !sessionIDPattern.MatchString(value) {
		return "", errors.New("session ID must be 32 lowercase hexadecimal characters")
	}
	return SessionID(value), nil
}
