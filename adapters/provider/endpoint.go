package provider

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

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

// ValidateEndpoint accepts an absolute http(s) URL without credentials,
// query or fragment. Plain http is accepted only for loopback hosts so a key
// never crosses the network unencrypted.
func ValidateEndpoint(raw string) error {
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

// EndpointName labels errors with the host of an endpoint, so a server's
// failure is not reported as OpenRouter's.
func EndpointName(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "model endpoint"
	}
	return "model endpoint " + parsed.Host
}

// EndpointHost is the host of an endpoint, "" when raw is not a URL.
func EndpointHost(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return parsed.Host
}
