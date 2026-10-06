package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type Dependencies struct {
	Start      application.StartTurnUseCase
	Resolve    application.ResolveToolUseCase
	Catalog    application.ToolCatalogPort
	Tools      application.ToolExecutionPort
	Approval   application.ToolApprovalPolicyPort
	Validation application.ToolArgumentValidationPort
	Ready      func(context.Context) error
}

type Server struct {
	Config      Config
	deps        Dependencies
	base        *storage.SessionStore
	sessions    *sessionStore
	events      *eventStore
	keys        *idempotencyStore
	calls       *callStore
	audit       *auditStore
	principals  map[string]Principal
	mu          sync.Mutex
	jobs        sync.WaitGroup
	closing     bool
	operations  map[string]context.CancelFunc
	locks       map[string]*sync.Mutex
	callLocks   map[string]*sync.Mutex
	directSlots chan struct{}
	streamSlots chan struct{}
	root        context.Context
	stop        context.CancelFunc
}

func NewServer(cfg Config, deps Dependencies) (*Server, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	policy, err := loadPrincipals(cfg.PrincipalsFile)
	if err != nil {
		return nil, err
	}
	if err := secureStateDirectory(cfg.StateDir); err != nil {
		return nil, err
	}
	base, err := storage.NewService(filepath.Join(cfg.StateDir, "sessions"))
	if err != nil {
		return nil, err
	}
	events, err := newEventStore(filepath.Join(cfg.StateDir, "events"))
	if err != nil {
		base.Close()
		return nil, err
	}
	keys, err := newIdempotencyStore(filepath.Join(cfg.StateDir, "idempotency"))
	if err != nil {
		base.Close()
		return nil, err
	}
	calls, err := newCallStore(filepath.Join(cfg.StateDir, "tool-calls"))
	if err != nil {
		base.Close()
		return nil, err
	}
	if err := calls.Recover(); err != nil {
		base.Close()
		return nil, err
	}
	audit, err := newAuditStore(filepath.Join(cfg.StateDir, "audit"))
	if err != nil {
		base.Close()
		return nil, err
	}
	wrapped := &sessionStore{next: base}
	if err := recoverSessions(wrapped, events); err != nil {
		audit.Close()
		base.Close()
		return nil, err
	}
	if err := recoverClaims(keys, calls, events); err != nil {
		audit.Close()
		base.Close()
		return nil, err
	}
	deps.Start.Store = wrapped
	deps.Start.Continue.Store = wrapped
	deps.Resolve.Store = wrapped
	deps.Resolve.Continue.Store = wrapped
	root, stop := context.WithCancel(context.Background())
	return &Server{Config: cfg, deps: deps, base: base, sessions: wrapped, events: events, keys: keys, calls: calls, audit: audit, principals: policy, operations: map[string]context.CancelFunc{}, locks: map[string]*sync.Mutex{}, callLocks: map[string]*sync.Mutex{}, directSlots: make(chan struct{}, 8), streamSlots: make(chan struct{}, 64), root: root, stop: stop}, nil
}

func (s *Server) Close() error {
	s.stop()
	s.mu.Lock()
	s.closing = true
	for _, cancel := range s.operations {
		cancel()
	}
	s.mu.Unlock()
	s.jobs.Wait()
	return errors.Join(s.base.Close(), s.audit.Close())
}

func (s *Server) startBackground(run func()) bool {
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		return false
	}
	s.jobs.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.jobs.Done()
		run()
	}()
	return true
}

func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func (s *Server) sessionLock(id string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock := s.locks[id]
	if lock == nil {
		lock = &sync.Mutex{}
		s.locks[id] = lock
	}
	return lock
}

func (s *Server) callLock(id string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock := s.callLocks[id]
	if lock == nil {
		lock = &sync.Mutex{}
		s.callLocks[id] = lock
	}
	return lock
}

func (s *Server) registerOperation(id string, cancel context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.operations[id] = cancel
}
func (s *Server) finishOperation(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.operations, id)
}
func (s *Server) cancelOperation(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	cancel := s.operations[id]
	if cancel == nil {
		return false
	}
	cancel()
	return true
}

func (s *Server) apiRoutes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/sessions", s.handleCreateSession)
	mux.HandleFunc("GET /v1/sessions/{id}", s.handleGetSession)
	mux.HandleFunc("POST /v1/sessions/{id}/turns", s.handleStartTurn)
	mux.HandleFunc("GET /v1/sessions/{id}/events", s.handleEvents)
	mux.HandleFunc("POST /v1/sessions/{id}/approvals/{call_id}", s.handleApproval)
	mux.HandleFunc("POST /v1/sessions/{id}/cancel", s.handleCancel)
	mux.HandleFunc("GET /v1/tools", s.handleTools)
	mux.HandleFunc("POST /v1/tool-calls", s.handleCreateToolCall)
	mux.HandleFunc("GET /v1/tool-calls/{id}", s.handleGetToolCall)
	mux.HandleFunc("POST /v1/tool-calls/{id}/decisions", s.handleToolDecision)
	return mux
}

func (s *Server) APIHandler() http.Handler {
	mux := s.apiRoutes()
	mux.Handle("/mcp", s.mcpHandler())
	mux.HandleFunc("POST /v1/operations/{verb}", s.handleOperation)
	return s.authenticated(mux)
}

func (s *Server) authenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-Id")
		if !safeRequestID(requestID) {
			requestID, _ = newID()
		}
		w.Header().Set("X-Request-Id", requestID)
		p, ok := authenticate(r, s.principals)
		if !ok {
			writeError(w, requestID, http.StatusForbidden, "forbidden", "certificate is not authorized")
			return
		}
		ctx := context.WithValue(r.Context(), principalKey{}, p)
		ctx = context.WithValue(ctx, requestIDKey{}, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func safeRequestID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for i := 0; i < len(id); i++ {
		if id[i] < 0x21 || id[i] > 0x7e {
			return false
		}
	}
	return true
}

func (s *Server) ProbeHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if s.deps.Ready == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := s.deps.Ready(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	return mux
}

func (s *Server) Serve(ctx context.Context) error {
	tlsConfig, err := serverTLSConfig(s.Config)
	if err != nil {
		return err
	}
	api := &http.Server{Addr: s.Config.APIListen, Handler: s.APIHandler(), TLSConfig: tlsConfig, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	probe := &http.Server{Addr: s.Config.ProbeListen, Handler: s.ProbeHandler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second}
	apiErr := make(chan error, 1)
	probeErr := make(chan error, 1)
	grpcErr := make(chan error, 1)
	if s.Config.GRPCListen != "" {
		server, err := s.grpcServer()
		if err != nil {
			return err
		}
		listener, err := net.Listen("tcp", s.Config.GRPCListen)
		if err != nil {
			server.Stop()
			return err
		}
		defer server.Stop()
		defer listener.Close()
		go func() { grpcErr <- server.Serve(listener) }()
	}
	go func() {
		err := api.ListenAndServeTLS("", "")
		if !errors.Is(err, http.ErrServerClosed) {
			apiErr <- err
		}
	}()
	go func() {
		err := probe.ListenAndServe()
		if !errors.Is(err, http.ErrServerClosed) {
			probeErr <- err
		}
	}()
	select {
	case <-ctx.Done():
	case err = <-apiErr:
	case err = <-probeErr:
	case err = <-grpcErr:
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = api.Shutdown(shutdownCtx)
	_ = probe.Shutdown(shutdownCtx)
	return err
}

type principalKey struct{}
type requestIDKey struct{}

func principal(r *http.Request) Principal {
	p, _ := r.Context().Value(principalKey{}).(Principal)
	return p
}
func requestID(r *http.Request) string {
	id, _ := r.Context().Value(requestIDKey{}).(string)
	return id
}

func (s *Server) loadSession(ctx context.Context, rawID string) (domain.Session, error) {
	id, err := domain.NewSessionID(rawID)
	if err != nil {
		return domain.Session{}, err
	}
	return s.sessions.Load(ctx, id)
}

func isNotFound(err error) bool { return errors.Is(err, os.ErrNotExist) }
