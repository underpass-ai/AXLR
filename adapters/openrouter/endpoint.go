package openrouter

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

const providerName = "OpenRouter"

// IsLoopbackEndpoint reports whether raw is an absolute http(s) URL whose host
// is localhost or a loopback address.
func IsLoopbackEndpoint(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return false
	}
	host := parsed.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// validateEndpoint accepts an absolute http(s) URL without credentials,
// query or fragment. Plain http is accepted only for loopback hosts so a key
// never crosses the network unencrypted.
func validateEndpoint(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("model endpoint must be an absolute http(s) URL without credentials, query or fragment")
	}
	switch parsed.Scheme {
	case "https":
		return nil
	case "http":
		if IsLoopbackEndpoint(raw) {
			return nil
		}
		return errors.New("model endpoint must use https unless it is a loopback address")
	default:
		return errors.New("model endpoint must be an absolute http(s) URL without credentials, query or fragment")
	}
}

// endpointName labels errors: OpenRouter for the default endpoint and the
// host otherwise, so a local server's failure is not reported as OpenRouter's.
func endpointName(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "model endpoint"
	}
	return "model endpoint " + parsed.Host
}
