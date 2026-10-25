package domain

import "testing"

func TestEgressRulesReadHostsPortsWildcardsAndAddresses(t *testing.T) {
	for raw, want := range map[string]string{
		"API.GitHub.com:443": "api.github.com:443",
		"pypi.org":           "pypi.org",
		"*.Example.com.":     "*.example.com",
		"10.0.0.7:8080":      "10.0.0.7:8080",
		"[::1]:8080":         "[::1]:8080",
		"[2001:db8::1]":      "[2001:db8::1]",
		"localhost:11434":    "localhost:11434",
	} {
		rule, err := ParseEgressRule(raw)
		if err != nil || rule.String() != want {
			t.Errorf("%q: %q %v", raw, rule.String(), err)
		}
	}
	for _, raw := range []string{"", "host:", "host:0", "host:65536", "host:08", "::1", "[::1", "[::1]x", "[example.com]", "*.", "*", "a*.example.com", "exa_mple.com", "-bad.com", "http://example.com", "example.com/path", "a..b"} {
		if rule, err := ParseEgressRule(raw); err == nil {
			t.Errorf("accepted %q as %q", raw, rule.String())
		}
	}
}

func TestAnEgressRuleAllowsOnlyItsDestination(t *testing.T) {
	cases := []struct {
		rule, host string
		port       int
		allowed    bool
	}{
		{"api.github.com:443", "api.github.com", 443, true},
		{"api.github.com:443", "API.GITHUB.COM.", 443, true},
		{"api.github.com:443", "api.github.com", 80, false},
		{"api.github.com:443", "evil.api.github.com", 443, false},
		{"api.github.com:443", "github.com", 443, false},
		{"pypi.org", "pypi.org", 443, true},
		{"pypi.org", "pypi.org", 80, true},
		{"pypi.org", "pypi.org", 8080, false},
		{"*.example.com:443", "a.example.com", 443, true},
		{"*.example.com:443", "a.b.example.com", 443, true},
		{"*.example.com:443", "example.com", 443, false},
		{"*.example.com:443", "badexample.com", 443, false},
		{"127.0.0.1:8080", "127.0.0.1", 8080, true},
		{"127.0.0.1:8080", "localhost", 8080, false},
		{"[::1]:8080", "[::1]", 8080, true},
		{"[::1]:8080", "0:0:0:0:0:0:0:1", 8080, true},
	}
	for _, c := range cases {
		rule, err := ParseEgressRule(c.rule)
		if err != nil {
			t.Fatal(err)
		}
		if got := rule.Allows(c.host, c.port); got != c.allowed {
			t.Errorf("%s allows %s:%d = %v", c.rule, c.host, c.port, got)
		}
	}
}
