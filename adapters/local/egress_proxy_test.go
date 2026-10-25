//go:build !windows

package local

import (
	"bufio"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/domain"
)

// connectThrough opens a CONNECT tunnel through the proxy's socket and
// returns the proxy's status line and, on 200, the tunnel.
func connectThrough(t *testing.T, proxy *egressProxy, address string) (string, net.Conn) {
	t.Helper()
	conn, err := net.Dial("unix", proxy.socket())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(conn, "CONNECT "+address+" HTTP/1.1\r\nHost: "+address+"\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, &http.Request{Method: http.MethodConnect})
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		_ = conn.Close()
		return response.Status, nil
	}
	return response.Status, conn
}

func TestTheEgressProxyTunnelsOnlyToTheAllowList(t *testing.T) {
	allowed := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "secret-free") }))
	defer allowed.Close()
	address := strings.TrimPrefix(allowed.URL, "https://")
	rule, err := domain.ParseEgressRule(address)
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := startEgressProxy([]domain.EgressRule{rule})
	if err != nil {
		t.Fatal(err)
	}
	status, tunnel := connectThrough(t, proxy, address)
	if tunnel == nil {
		t.Fatalf("the allowed host: %s", status)
	}
	client := &http.Client{Transport: &http.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			conn := tls.Client(tunnel, &tls.Config{InsecureSkipVerify: true})
			return conn, conn.HandshakeContext(ctx)
		},
	}}
	response, err := client.Get(allowed.URL)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if string(body) != "secret-free" {
		t.Fatalf("through the tunnel: %q", body)
	}
	client.CloseIdleConnections()
	status, tunnel = connectThrough(t, proxy, "example.com:443")
	if tunnel != nil || !strings.HasPrefix(status, "403") {
		t.Fatalf("a host off the list: %s", status)
	}
	if note := proxy.close(); note != "axlr sandbox: egress refused to example.com:443 (not in the execution's allow list)\n" {
		t.Fatalf("note %q", note)
	}
}

func TestTheEgressProxyForwardsPlainHTTPAndRefusesRedirectsOffTheList(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "other") }))
	defer other.Close()
	allowed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/away" {
			http.Redirect(w, r, other.URL, http.StatusFound)
			return
		}
		if r.Header.Get("Proxy-Authorization") != "" {
			t.Errorf("a hop-by-hop header crossed the proxy")
		}
		_, _ = io.WriteString(w, "allowed")
	}))
	defer allowed.Close()
	rule, err := domain.ParseEgressRule(strings.TrimPrefix(allowed.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := startEgressProxy([]domain.EgressRule{rule})
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.close()
	proxyURL, _ := url.Parse("http://axlr-egress")
	client := &http.Client{Transport: &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return net.Dial("unix", proxy.socket())
		},
	}}
	request, _ := http.NewRequest(http.MethodGet, allowed.URL, nil)
	request.Header.Set("Proxy-Authorization", "Basic dGVzdA==")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || string(body) != "allowed" {
		t.Fatalf("the allowed host: %d %q", response.StatusCode, body)
	}
	// The client follows the redirect through the proxy, which refuses it.
	response, err = client.Get(allowed.URL + "/away")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("a redirect off the list: %d", response.StatusCode)
	}
}
