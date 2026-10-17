package catalog

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// Minisign (https://jedisct1.github.io/minisign/) signs a file with an
// Ed25519 key. The public key is one base64 line, "Ed", an 8-byte key id
// and the 32-byte key, optionally under an "untrusted comment:" line. The
// signature file is four lines: an untrusted comment, the base64 signature
// (the algorithm, "Ed" for a signature over the file or "ED" over its
// BLAKE2b-512 digest, the key id and 64 bytes of Ed25519 signature), the
// trusted comment, and the base64 global signature over the signature and
// the trusted comment. Verify checks all of it; Sign writes it, for the
// tests and for a catalog that signs with a key of its own.

// PublicKey is a parsed minisign public key.
type PublicKey struct {
	ID  [8]byte
	Key ed25519.PublicKey
}

const (
	minisignAlgLegacy    = "Ed"
	minisignAlgPrehashed = "ED"
	trustedCommentPrefix = "trusted comment: "
)

// ParsePublicKey reads a minisign public key, the base64 line alone or the
// key file with its comment.
func ParsePublicKey(text string) (PublicKey, error) {
	line := ""
	for _, candidate := range strings.Split(text, "\n") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || strings.HasPrefix(candidate, "untrusted comment:") {
			continue
		}
		if line != "" {
			return PublicKey{}, errors.New("minisign public key has more than one key line")
		}
		line = candidate
	}
	raw, err := base64.StdEncoding.DecodeString(line)
	if err != nil || len(raw) != 2+8+ed25519.PublicKeySize {
		return PublicKey{}, errors.New("minisign public key must be base64 of Ed, an 8-byte key id and a 32-byte Ed25519 key")
	}
	if string(raw[:2]) != minisignAlgLegacy {
		return PublicKey{}, fmt.Errorf("minisign public key algorithm %q is not Ed", raw[:2])
	}
	var key PublicKey
	copy(key.ID[:], raw[2:10])
	key.Key = ed25519.PublicKey(append([]byte(nil), raw[10:]...))
	return key, nil
}

// String is the key's base64 line, as settings carry it.
func (k PublicKey) String() string {
	raw := append([]byte(minisignAlgLegacy), k.ID[:]...)
	return base64.StdEncoding.EncodeToString(append(raw, k.Key...))
}

// Verify checks signature, the contents of a .minisig file, over message
// with key: the key id, the signature (over the message or its BLAKE2b
// digest, as the signature says) and the global signature over the
// signature and the trusted comment.
func Verify(key PublicKey, message, signature []byte) error {
	lines := strings.Split(strings.ReplaceAll(string(signature), "\r\n", "\n"), "\n")
	var kept []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			kept = append(kept, line)
		}
	}
	if len(kept) != 4 || !strings.HasPrefix(kept[0], "untrusted comment:") || !strings.HasPrefix(kept[2], trustedCommentPrefix) {
		return errors.New("minisign signature must be an untrusted comment, the signature, the trusted comment and the global signature")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(kept[1]))
	if err != nil || len(raw) != 2+8+ed25519.SignatureSize {
		return errors.New("minisign signature line is not base64 of the algorithm, the key id and a 64-byte signature")
	}
	global, err := base64.StdEncoding.DecodeString(strings.TrimSpace(kept[3]))
	if err != nil || len(global) != ed25519.SignatureSize {
		return errors.New("minisign global signature is not a base64 64-byte signature")
	}
	if !bytes.Equal(raw[2:10], key.ID[:]) {
		return errors.New("minisign signature was made with another key: its key id differs from the catalog's public key")
	}
	signed := message
	switch string(raw[:2]) {
	case minisignAlgLegacy:
	case minisignAlgPrehashed:
		digest := blake2b512(message)
		signed = digest[:]
	default:
		return fmt.Errorf("minisign signature algorithm %q is neither Ed nor ED", raw[:2])
	}
	sig := raw[10:]
	if !ed25519.Verify(key.Key, signed, sig) {
		return errors.New("minisign signature does not verify: the index is not the one the catalog signed")
	}
	trusted := strings.TrimPrefix(kept[2], trustedCommentPrefix)
	if !ed25519.Verify(key.Key, append(append([]byte(nil), sig...), []byte(trusted)...), global) {
		return errors.New("minisign global signature does not verify: the trusted comment was changed")
	}
	return nil
}

// Sign writes the .minisig contents for message with a private key whose
// public half has id: prehashed signs the BLAKE2b-512 digest (what the
// minisign tool writes), otherwise the message itself.
func Sign(private ed25519.PrivateKey, id [8]byte, message []byte, trustedComment string, prehashed bool) []byte {
	alg, signed := minisignAlgLegacy, message
	if prehashed {
		digest := blake2b512(message)
		alg, signed = minisignAlgPrehashed, digest[:]
	}
	sig := ed25519.Sign(private, signed)
	raw := append(append([]byte(alg), id[:]...), sig...)
	global := ed25519.Sign(private, append(append([]byte(nil), sig...), []byte(trustedComment)...))
	return []byte("untrusted comment: signature from AXLR catalog tooling\n" + base64.StdEncoding.EncodeToString(raw) + "\n" + trustedCommentPrefix + trustedComment + "\n" + base64.StdEncoding.EncodeToString(global) + "\n")
}
