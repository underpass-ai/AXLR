package provider

type TransportError struct {
	Cause error
	// Provider names a non-OpenRouter endpoint; empty means OpenRouter.
	Provider string
}

func (e *TransportError) Error() string {
	if e.Provider != "" && e.Provider != DefaultProviderName {
		return e.Provider + " transport failure"
	}
	return DefaultProviderName + " transport failure"
}
func (e *TransportError) Unwrap() error { return e.Cause }
