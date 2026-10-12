package application

// RunLivenessPort tells whether the console launch that owns a run token is
// still running. Each launch holds its token for its lifetime, so a plan or
// a job recorded under another token belongs to a console that stopped
// only when that console no longer holds it: on 12 October 2026 a second
// console marked the plans and repairs of a first one, still running them,
// as interrupted, and offered to run them again.
type RunLivenessPort interface {
	Alive(token string) bool
}

// ownerAlive reports whether the launch that owns token still runs: this
// one, or another that still holds its token. Without a port every other
// launch is taken for stopped, as before tokens were held.
func ownerAlive(liveness RunLivenessPort, token, mine string) bool {
	if token == mine {
		return true
	}
	return liveness != nil && token != "" && liveness.Alive(token)
}
