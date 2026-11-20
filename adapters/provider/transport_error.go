package provider

import (
	"context"
	"errors"
	"io"
	"net"
	"syscall"
)

type TransportError struct {
	Cause error
	// Provider names a non-OpenRouter endpoint; empty means OpenRouter.
	Provider string
}

func (e *TransportError) Error() string {
	name := DefaultProviderName
	if e.Provider != "" && e.Provider != DefaultProviderName {
		name = e.Provider
	}
	if kind := transportKind(e.Cause); kind != "" {
		return name + " transport failure: " + kind
	}
	return name + " transport failure"
}
func (e *TransportError) Unwrap() error { return e.Cause }

// Transient reports that asking again may succeed: the connection failed,
// not the provider's verdict on the request.
func (e *TransportError) Transient() bool { return true }

// transportKind names how the connection failed without quoting the cause,
// which may hold an address or a server's words. Before 20 November 2026 a
// stopped llama.cpp read only "transport failure", which did not say the
// server was down.
func transportKind(err error) string {
	var dns *net.DNSError
	var netErr net.Error
	switch {
	case err == nil:
		return ""
	case errors.Is(err, syscall.ECONNREFUSED):
		return "connection refused (is the server running?)"
	case errors.Is(err, syscall.ECONNRESET), errors.Is(err, syscall.EPIPE):
		return "connection reset by the server"
	case errors.As(err, &dns):
		return "host not found"
	case errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, io.EOF):
		return "connection closed by the server"
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		return "timed out"
	case errors.Is(err, syscall.EHOSTUNREACH), errors.Is(err, syscall.ENETUNREACH):
		return "host unreachable"
	}
	return ""
}
