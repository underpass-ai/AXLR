package storage

// sessionFailure is a turn that failed: the error and the kind of hint
// the console words for the person.
type sessionFailure struct {
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}
