package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

type Digest string

func NewDigest(s string) (Digest, error) {
	if len(s) != 64 {
		return "", errors.New("invalid SHA-256 digest")
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return "", errors.New("invalid SHA-256 digest")
		}
	}
	return Digest(s), nil
}
func DigestOf(b []byte) Digest { h := sha256.Sum256(b); return Digest(hex.EncodeToString(h[:])) }
