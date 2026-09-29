package openrouter

type TransportError struct{ Cause error }

func (e *TransportError) Error() string { return "OpenRouter transport failure" }
func (e *TransportError) Unwrap() error { return e.Cause }
