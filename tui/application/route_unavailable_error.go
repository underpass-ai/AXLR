package application

import "fmt"

// RouteUnavailableError refuses a request no candidate of the execution's
// routing may take: each was excluded by a constraint, or the one asked
// failed with no other to fall back on. The request is not sent anywhere
// else, never to the session's own model.
type RouteUnavailableError struct {
	Summary string
}

func (e *RouteUnavailableError) Error() string {
	return fmt.Sprintf("no candidate model of the execution's routing can take this request: %s", e.Summary)
}
