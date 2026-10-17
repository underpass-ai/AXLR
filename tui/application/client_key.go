package application

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
)

// A client key (axlr-registry docs/client-identity.md, §1) is the one
// Ed25519 key pair of this console: it signs the bundles it publishes and
// every request it makes to a registry. The console keeps it in its
// private state, never in a workspace; the registry only ever sees the
// public key.

// ClientKeyPort keeps the console's client key.
type ClientKeyPort interface {
	// Load returns the key, or nil when none was created yet.
	Load(ctx context.Context) (*ClientKey, error)
	// Create makes a new key and stores it; it refuses to replace one, since
	// a key the registry knows is not replaced in passing.
	Create(ctx context.Context) (*ClientKey, error)
}

// ClientKey is an Ed25519 key pair.
type ClientKey struct {
	Public  ed25519.PublicKey
	private ed25519.PrivateKey
}

// NewClientKey wraps a private key.
func NewClientKey(private ed25519.PrivateKey) (*ClientKey, error) {
	if len(private) != ed25519.PrivateKeySize {
		return nil, errors.New("client key must be a 64-byte Ed25519 private key")
	}
	return &ClientKey{Public: private.Public().(ed25519.PublicKey), private: private}, nil
}

// Seed is the 32-byte seed the key file keeps.
func (k *ClientKey) Seed() []byte { return k.private.Seed() }

// Sign signs a message.
func (k *ClientKey) Sign(message []byte) []byte { return ed25519.Sign(k.private, message) }

// ID is the key id: the first 16 hex characters of SHA-256 over the public
// key.
func (k *ClientKey) ID() string { return KeyIDOf(k.Public) }

// PublicBase64 is the public key as the registry receives it.
func (k *ClientKey) PublicBase64() string { return base64.StdEncoding.EncodeToString(k.Public) }

// Identity is what /tools identity shows.
func (k *ClientKey) Identity() ClientIdentity {
	return ClientIdentity{KeyID: k.ID(), PublicKey: k.PublicBase64()}
}

// KeyIDOf derives a key id from a public key.
func KeyIDOf(public ed25519.PublicKey) string {
	sum := sha256.Sum256(public)
	return hex.EncodeToString(sum[:8])
}

// ClientIdentity is the public side of the client key.
type ClientIdentity struct {
	KeyID     string
	PublicKey string
}

// Identity shows the client key; created is false when none exists yet.
func (u CatalogUseCase) Identity(ctx context.Context) (ClientIdentity, bool, error) {
	if u.Keys == nil {
		return ClientIdentity{}, false, errors.New("the registry client is not enabled in this console")
	}
	key, err := u.Keys.Load(ctx)
	if err != nil || key == nil {
		return ClientIdentity{}, false, err
	}
	return key.Identity(), true, nil
}

// Keygen creates the client key; it refuses when one exists.
func (u CatalogUseCase) Keygen(ctx context.Context) (ClientIdentity, error) {
	if u.Keys == nil {
		return ClientIdentity{}, errors.New("the registry client is not enabled in this console")
	}
	key, err := u.Keys.Create(ctx)
	if err != nil {
		return ClientIdentity{}, err
	}
	return key.Identity(), nil
}

// clientKey loads the key, creating it when there is none: publishing
// and enrolling need one.
func (u CatalogUseCase) clientKey(ctx context.Context) (*ClientKey, bool, error) {
	if u.Keys == nil {
		return nil, false, errors.New("the registry client is not enabled in this console")
	}
	key, err := u.Keys.Load(ctx)
	if err != nil {
		return nil, false, err
	}
	if key != nil {
		return key, false, nil
	}
	key, err = u.Keys.Create(ctx)
	if err != nil {
		return nil, false, err
	}
	return key, true, nil
}
