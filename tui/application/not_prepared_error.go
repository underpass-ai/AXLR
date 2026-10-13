package application

import (
	"errors"
	"strings"
)

// ErrCeremonyNotPrepared means the pinned 2.0 definition is not published in
// the connected MADE store; only the explicit /mcp → P action publishes it.
var ErrCeremonyNotPrepared = errors.New("prepare MADE first: open /mcp, select MADE and press p")

// notPreparedError names the missing definitions and the one-time action; it
// matches ErrCeremonyNotPrepared with errors.Is.
type notPreparedError struct {
	missing []MissingDefinition
}

// NotPreparedError returns ErrCeremonyNotPrepared's failure naming each missing
// definition with its version and the exact action to publish it.
func NotPreparedError(missing ...MissingDefinition) error {
	return notPreparedError{missing: missing}
}

func (e notPreparedError) Error() string {
	names := make([]string, 0, len(e.missing))
	for _, m := range e.missing {
		names = append(names, m.Name+" "+m.Version)
	}
	list := strings.Join(names, ", ")
	if list == "" {
		list = "the MADE definition"
	}
	// Where the person's prompt went is the console's to say: this text
	// also reaches plan tasks and the model's tool results.
	return "MADE definition not published: " + list +
		". Preparing MADE is a one-time setup: open /mcp, select MADE and press p."
}

func (e notPreparedError) Is(target error) bool {
	return target == ErrCeremonyNotPrepared
}
