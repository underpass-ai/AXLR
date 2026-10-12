package application

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// NewRandomSessionID draws a new session's ID: 16 random bytes in hex.
func NewRandomSessionID() (domain.SessionID, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return domain.NewSessionID(hex.EncodeToString(raw[:]))
}
