package application

import "github.com/underpass-ai/AXLR/tui/domain"

// SessionBundleCodec encodes a session with its sidecars and decodes it
// back; the storage adapter owns the format.
type SessionBundleCodec interface {
	Encode(session domain.Session, meta SessionBundleMeta) ([]byte, error)
	Decode(data []byte) (domain.Session, SessionBundleMeta, error)
}
