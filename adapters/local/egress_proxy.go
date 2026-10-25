package local

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/underpass-ai/AXLR/domain"
)

// egressProxy is the only way out of a sandbox whose egress is an allow
// list: an HTTP proxy on a Unix socket that the sandbox mounts, which
// tunnels CONNECT host:port and forwards plain http:// requests to the
// destinations its rules allow and answers 403 to every other. It resolves
// names itself, outside the sandbox, which has no resolver. One runs for
// each command, and closes with it.
type egressProxy struct {
	allow    []domain.EgressRule
	dir      string
	listener net.Listener
	server   *http.Server
	dialer   net.Dialer
	mu       sync.Mutex
	refused  map[string]struct{}
	tunnels  sync.WaitGroup
	conns    map[net.Conn]struct{}
}

// egressDialTimeout bounds connecting to an allowed destination.
const egressDialTimeout = 15 * time.Second

// startEgressProxy listens on a socket in a private directory of its own.
func startEgressProxy(allow []domain.EgressRule) (*egressProxy, error) {
	dir, err := os.MkdirTemp(socketBase(), "axlr-egress-")
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("unix", filepath.Join(dir, "egress.sock"))
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	p := &egressProxy{allow: allow, dir: dir, listener: listener, dialer: net.Dialer{Timeout: egressDialTimeout}, refused: map[string]struct{}{}, conns: map[net.Conn]struct{}{}}
	transport := &http.Transport{Proxy: nil, DialContext: p.dialAllowed, ForceAttemptHTTP2: false, MaxIdleConns: 4, IdleConnTimeout: 30 * time.Second, ResponseHeaderTimeout: 5 * time.Minute}
	forward := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.Out.URL = r.In.URL
			r.Out.Host = r.In.Host
		},
		Transport: transport,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, "axlr sandbox: "+err.Error(), http.StatusBadGateway)
		},
	}
	p.server = &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { p.serve(w, r, forward) }), ReadHeaderTimeout: 30 * time.Second}
	go func() { _ = p.server.Serve(listener) }()
	return p, nil
}

// socketBase is where the proxy's directory goes: the temporary directory,
// or /tmp when that is too deep for a socket's path (108 bytes on Linux).
func socketBase() string {
	if base := os.TempDir(); len(base) <= 64 {
		return base
	}
	return "/tmp"
}

// socket is the path the sandbox mounts.
func (p *egressProxy) socket() string { return p.listener.Addr().String() }

func (p *egressProxy) serve(w http.ResponseWriter, r *http.Request, forward *httputil.ReverseProxy) {
	if r.Method == http.MethodConnect {
		p.tunnel(w, r)
		return
	}
	if r.URL.Scheme != "http" || r.URL.Host == "" {
		http.Error(w, "axlr sandbox: the egress proxy forwards http:// URLs and tunnels CONNECT host:port", http.StatusBadRequest)
		return
	}
	host, port := r.URL.Hostname(), 80
	if raw := r.URL.Port(); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			http.Error(w, "axlr sandbox: invalid port", http.StatusBadRequest)
			return
		}
		port = parsed
	}
	if !p.allows(host, port) {
		p.refuse(w, host, port)
		return
	}
	forward.ServeHTTP(w, r)
}

// tunnel connects a CONNECT request to its destination and copies both
// ways until either side closes.
func (p *egressProxy) tunnel(w http.ResponseWriter, r *http.Request) {
	host, rawPort, err := net.SplitHostPort(r.Host)
	port, portErr := strconv.Atoi(rawPort)
	if err != nil || portErr != nil {
		http.Error(w, "axlr sandbox: CONNECT needs host:port", http.StatusBadRequest)
		return
	}
	if !p.allows(host, port) {
		p.refuse(w, host, port)
		return
	}
	upstream, err := p.dialer.DialContext(r.Context(), "tcp", net.JoinHostPort(host, rawPort))
	if err != nil {
		http.Error(w, "axlr sandbox: "+err.Error(), http.StatusBadGateway)
		return
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		_ = upstream.Close()
		http.Error(w, "axlr sandbox: the proxy cannot tunnel", http.StatusInternalServerError)
		return
	}
	client, buffered, err := hijacker.Hijack()
	if err != nil {
		_ = upstream.Close()
		return
	}
	if !p.track(client, upstream) {
		_ = client.Close()
		_ = upstream.Close()
		return
	}
	if _, err := client.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		p.untrack(client, upstream)
		return
	}
	go func() {
		defer p.untrack(client, upstream)
		copied := make(chan struct{})
		go func() {
			_, _ = io.Copy(upstream, buffered)
			if closer, ok := upstream.(interface{ CloseWrite() error }); ok {
				_ = closer.CloseWrite()
			}
			close(copied)
		}()
		_, _ = io.Copy(client, upstream)
		_ = client.Close()
		<-copied
	}()
}

// dialAllowed is the forwarder's dial: it checks the destination again,
// so a redirect or a reused connection cannot leave the allow list.
func (p *egressProxy) dialAllowed(ctx context.Context, network, address string) (net.Conn, error) {
	host, rawPort, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil || !p.allows(host, port) {
		return nil, fmt.Errorf("egress to %s is not in the execution's allow list", address)
	}
	return p.dialer.DialContext(ctx, network, address)
}

func (p *egressProxy) allows(host string, port int) bool {
	for _, rule := range p.allow {
		if rule.Allows(host, port) {
			return true
		}
	}
	return false
}

func (p *egressProxy) refuse(w http.ResponseWriter, host string, port int) {
	destination := net.JoinHostPort(strings.ToLower(host), strconv.Itoa(port))
	p.mu.Lock()
	if len(p.refused) < 32 {
		p.refused[destination] = struct{}{}
	}
	p.mu.Unlock()
	http.Error(w, "axlr sandbox: egress to "+destination+" is not in the execution's allow list", http.StatusForbidden)
}

// track keeps a tunnel's connections so close can cut them; false once the
// proxy is closing.
func (p *egressProxy) track(conns ...net.Conn) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conns == nil {
		return false
	}
	for _, conn := range conns {
		p.conns[conn] = struct{}{}
	}
	p.tunnels.Add(1)
	return true
}

func (p *egressProxy) untrack(conns ...net.Conn) {
	p.mu.Lock()
	for _, conn := range conns {
		_ = conn.Close()
		delete(p.conns, conn)
	}
	p.mu.Unlock()
	p.tunnels.Done()
}

// close stops the proxy, cuts its tunnels, removes its socket and returns
// the note of what it refused, "" when it refused nothing.
func (p *egressProxy) close() string {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = p.server.Shutdown(ctx)
	_ = p.server.Close()
	p.mu.Lock()
	for conn := range p.conns {
		_ = conn.Close()
	}
	p.conns = nil
	refused := make([]string, 0, len(p.refused))
	for destination := range p.refused {
		refused = append(refused, destination)
	}
	p.mu.Unlock()
	p.tunnels.Wait()
	_ = os.RemoveAll(p.dir)
	if len(refused) == 0 {
		return ""
	}
	sort.Strings(refused)
	return "axlr sandbox: egress refused to " + strings.Join(refused, ", ") + " (not in the execution's allow list)\n"
}
