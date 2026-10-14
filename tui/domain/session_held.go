package domain

import "errors"

// ErrSessionHeld refuses to open a session another console is writing.
var ErrSessionHeld = errors.New("session already has a writer")
