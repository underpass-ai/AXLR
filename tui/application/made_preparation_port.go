package application

import "context"

// MADEPreparationPort prepares the configured local MADE engine so AXLR works
// under its own least-privilege identity. It is an explicit setup operation:
// it may act as the store's trusted host, which the work agent never is.
type MADEPreparationPort interface {
	Prepare(context.Context) (MADEPreparation, error)
	// Publish validates and publishes one definition the person wrote
	// (/ceremony publish), through the same temporary install grant as the
	// console's own definitions, and returns "name version".
	Publish(ctx context.Context, definitionYAML string) (string, error)
}

type MADEPreparation struct {
	Status       string // ready, granted, remote, unsupported, missing, conflict
	WorkIdentity string
	// ApproverIdentity grants human guards when the person decides at the
	// console; the work identity cannot.
	ApproverIdentity string
	GrantID          string
	RestartRequired  bool
	Detail           string
	// Published lists the ceremony definitions this preparation published.
	Published []string
}
