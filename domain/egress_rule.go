package domain

import (
	"errors"
	"net"
	"strconv"
	"strings"
)

// EgressRule is one destination a sandboxed command may reach through the
// egress proxy: a host name, "*.example.com" for the subdomains of one, or
// an IP address, with a port; no port allows 443 and 80.
type EgressRule struct {
	host     string
	wildcard bool
	port     int
}

// ParseEgressRule reads "host", "host:port", "*.example.com:443",
// "10.0.0.7:8080" or "[::1]:8080".
func ParseEgressRule(raw string) (EgressRule, error) {
	if raw == "" || len(raw) > 300 {
		return EgressRule{}, errors.New("an egress rule is a host, or host:port, of at most 300 characters")
	}
	host, port, err := splitEgressDestination(raw)
	if err != nil {
		return EgressRule{}, err
	}
	rule := EgressRule{port: port}
	if ip := net.ParseIP(host); ip != nil {
		rule.host = ip.String()
		return rule, nil
	}
	if strings.HasPrefix(host, "*.") {
		rule.wildcard = true
		host = host[2:]
	}
	if !validHostName(host) {
		return EgressRule{}, errors.New("egress rule " + strconv.Quote(raw) + ": the host must be a name, *.name or an IP address")
	}
	rule.host = host
	return rule, nil
}

// Allows reports whether the rule lets a command reach host:port.
func (r EgressRule) Allows(host string, port int) bool {
	host = normalHost(host)
	if r.port == 0 && port != 80 && port != 443 || r.port != 0 && port != r.port {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String() == r.host
	}
	if r.wildcard {
		return strings.HasSuffix(host, "."+r.host)
	}
	return host == r.host
}

// String is the rule as ParseEgressRule reads it.
func (r EgressRule) String() string {
	host := r.host
	if r.wildcard {
		host = "*." + host
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if r.port == 0 {
		return host
	}
	return host + ":" + strconv.Itoa(r.port)
}

func splitEgressDestination(raw string) (string, int, error) {
	host, rawPort := raw, ""
	switch {
	case strings.HasPrefix(raw, "["):
		end := strings.Index(raw, "]")
		if end < 0 {
			return "", 0, errors.New("egress rule " + strconv.Quote(raw) + ": an IPv6 address goes in brackets")
		}
		host, rawPort = raw[1:end], raw[end+1:]
		if rawPort != "" {
			if !strings.HasPrefix(rawPort, ":") {
				return "", 0, errors.New("egress rule " + strconv.Quote(raw) + ": the port follows the brackets after a colon")
			}
			rawPort = rawPort[1:]
		}
		if net.ParseIP(host) == nil {
			return "", 0, errors.New("egress rule " + strconv.Quote(raw) + ": the brackets hold an IPv6 address")
		}
	case strings.Count(raw, ":") == 1:
		host, rawPort, _ = strings.Cut(raw, ":")
	case strings.Count(raw, ":") > 1:
		return "", 0, errors.New("egress rule " + strconv.Quote(raw) + ": an IPv6 address goes in brackets")
	}
	host = normalHost(host)
	if host == "" {
		return "", 0, errors.New("egress rule " + strconv.Quote(raw) + " names no host")
	}
	if rawPort == "" {
		if strings.HasSuffix(raw, ":") {
			return "", 0, errors.New("egress rule " + strconv.Quote(raw) + ": the port is empty")
		}
		return host, 0, nil
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil || port < 1 || port > 65535 || strconv.Itoa(port) != rawPort {
		return "", 0, errors.New("egress rule " + strconv.Quote(raw) + ": the port must be 1 to 65535")
	}
	return host, port, nil
}

// normalHost lowers a host name and drops a trailing dot and brackets.
func normalHost(host string) string {
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	return strings.TrimSuffix(strings.ToLower(host), ".")
}

func validHostName(host string) bool {
	if host == "" || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	return true
}
