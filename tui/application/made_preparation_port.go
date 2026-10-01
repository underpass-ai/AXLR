package application

import "context"

// MADEPreparationPort prepares the configured local MADE engine so AXLR works
// under its own least-privilege identity. It is an explicit setup operation:
// it may act as the store's trusted host, which the work agent never is.
type MADEPreparationPort interface {
	Prepare(context.Context) (MADEPreparation, error)
}

type MADEPreparation struct {
	Status          string // ready, granted, remote, unsupported, missing, conflict
	WorkIdentity    string
	GrantID         string
	RestartRequired bool
	Detail          string
}
