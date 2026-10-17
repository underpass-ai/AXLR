package registry

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/tui/application"
)

// registryServer is a loopback registry that checks every request as the
// contract's §3 says: the headers, the timestamp, a fresh nonce, and the
// signature rebuilt from the request with the client's public key.
type registryServer struct {
	*httptest.Server
	t      *testing.T
	public ed25519.PublicKey
	now    time.Time
	mu     sync.Mutex
	nonces map[string]bool
	// seen records each request's method, path and body.
	seen []seenRequest
	// answers maps "METHOD path" to a status and body; a 429 adds
	// Retry-After.
	answers map[string]answer
}

type seenRequest struct {
	method, path string
	body         []byte
	userAgent    string
}

type answer struct {
	status     int
	body       string
	retryAfter string
}

func serveRegistry(t *testing.T, public ed25519.PublicKey, now time.Time) *registryServer {
	t.Helper()
	s := &registryServer{t: t, public: public, now: now, nonces: map[string]bool{}, answers: map[string]answer{}}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.Close)
	return s
}

func (s *registryServer) handle(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	body, _ := io.ReadAll(r.Body)
	s.seen = append(s.seen, seenRequest{method: r.Method, path: r.URL.Path, body: body, userAgent: r.Header.Get("User-Agent")})
	fail := func(status int, why string) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"error":` + strconv.Quote(why) + `}`))
	}
	if r.Header.Get("User-Agent") != "AXLR-catalog" {
		fail(http.StatusForbidden, "not an AXLR client")
		return
	}
	keyID, stamp, nonce, signature := r.Header.Get(HeaderKey), r.Header.Get(HeaderTimestamp), r.Header.Get(HeaderNonce), r.Header.Get(HeaderSignature)
	if keyID != application.KeyIDOf(s.public) {
		fail(http.StatusBadRequest, "key id "+keyID)
		return
	}
	seconds, err := strconv.ParseInt(stamp, 10, 64)
	if err != nil || seconds < s.now.Unix()-300 || seconds > s.now.Unix()+300 {
		fail(http.StatusUnauthorized, "stale")
		return
	}
	rawNonce, err := base64.StdEncoding.DecodeString(nonce)
	if err != nil || len(rawNonce) != 16 {
		fail(http.StatusBadRequest, "nonce shape")
		return
	}
	if s.nonces[nonce] {
		fail(http.StatusUnauthorized, "replay")
		return
	}
	s.nonces[nonce] = true
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		fail(http.StatusBadRequest, "signature shape")
		return
	}
	sum := sha256.Sum256(body)
	message := "axlr-request-v1\n" + r.Method + "\n" + r.URL.EscapedPath() + "\n" + r.URL.RawQuery + "\n" + hex.EncodeToString(sum[:]) + "\n" + stamp + "\n" + nonce + "\n"
	if !ed25519.Verify(s.public, []byte(message), sig) {
		fail(http.StatusUnauthorized, "signature")
		return
	}
	if a, ok := s.answers[r.Method+" "+r.URL.Path]; ok {
		if a.retryAfter != "" {
			w.Header().Set("Retry-After", a.retryAfter)
		}
		w.WriteHeader(a.status)
		_, _ = w.Write([]byte(a.body))
		return
	}
	fail(http.StatusNotFound, "no route")
}

func testKey(t *testing.T) *application.ClientKey {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := application.NewClientKey(private)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// The canonical message is the contract's, for a body and for none.
func TestRequestMessageFollowsTheContract(t *testing.T) {
	empty := sha256.Sum256(nil)
	got := string(RequestMessage("get", "/v1/versions/acme_count/1.0.0", "", nil, 1760000000, "bm9uY2Vub25jZW5vbmNlbm8="))
	want := "axlr-request-v1\nGET\n/v1/versions/acme_count/1.0.0\n\n" + hex.EncodeToString(empty[:]) + "\n1760000000\nbm9uY2Vub25jZW5vbmNlbm8=\n"
	if got != want {
		t.Fatalf("no body:\n%q\n%q", got, want)
	}
	body := []byte(`{"id":"acme"}`)
	sum := sha256.Sum256(body)
	got = string(RequestMessage("POST", "/v1/publishers/enroll", "status=scanned", body, 1, "n"))
	want = "axlr-request-v1\nPOST\n/v1/publishers/enroll\nstatus=scanned\n" + hex.EncodeToString(sum[:]) + "\n1\nn\n"
	if got != want {
		t.Fatalf("body:\n%q\n%q", got, want)
	}
}

// Every request carries the four headers and the User-Agent, verifies
// with the public key, and a fresh nonce each time; the server rebuilds
// the message from the request alone.
func TestClientSignsEnrollSubmitAndVersion(t *testing.T) {
	key := testKey(t)
	now := time.Unix(1760000000, 0)
	server := serveRegistry(t, key.Public, now)
	server.answers["POST /v1/publishers/enroll"] = answer{status: 202, body: `{"id":"acme","status":"pending"}`}
	server.answers["POST /v1/versions"] = answer{status: 201, body: `{"name":"acme_count","version":"1.0.0","status":"scanned","findings":[]}`}
	server.answers["GET /v1/versions/acme_count/1.0.0"] = answer{status: 200, body: `{"name":"acme_count","version":"1.0.0","status":"scanned","submitted_at":"2026-10-17T10:00:00Z","findings":[{"rule":"net.socket","file":"count.py","line":3,"severity":"warn","message":"opens a socket"}],"tests_passed":true,"failed_tests":[]}`}
	client := &Client{Now: func() time.Time { return now }}
	ctx := context.Background()

	result, err := client.Enroll(ctx, server.URL, key, application.EnrollRequest{ID: "acme", Namespaces: []string{"acme_*"}, Contact: "ops@acme.example"})
	if err != nil || result.Status != "pending" || result.ID != "acme" {
		t.Fatalf("enroll: %+v, %v", result, err)
	}
	var enrolled map[string]any
	if err := json.Unmarshal(server.seen[0].body, &enrolled); err != nil {
		t.Fatal(err)
	}
	if enrolled["id"] != "acme" || enrolled["public_key"] != key.PublicBase64() || enrolled["contact"] != "ops@acme.example" {
		t.Fatalf("enroll body %s", server.seen[0].body)
	}

	upload := application.UploadRequest{Manifest: []byte(`{"name":"acme_count"}`), Files: map[string][]byte{"count.py": []byte("print(1)\n")}, Tests: []byte(`[]`), KeyID: key.ID(), Signature: key.Sign([]byte("x"))}
	report, err := client.Submit(ctx, server.URL+"/", key, upload)
	if err != nil || report.Status != "scanned" || report.TestsPassed != nil {
		t.Fatalf("submit: %+v, %v", report, err)
	}
	var submitted struct {
		Manifest, Tests, KeyID, Signature string
		Files                             map[string]string
	}
	if err := json.Unmarshal(server.seen[1].body, &submitted); err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(server.seen[1].body, &raw)
	if string(raw["key_id"]) != `"`+key.ID()+`"` || submitted.Manifest != base64.StdEncoding.EncodeToString(upload.Manifest) || submitted.Files["count.py"] != base64.StdEncoding.EncodeToString([]byte("print(1)\n")) || submitted.Tests != base64.StdEncoding.EncodeToString([]byte(`[]`)) || string(raw["signature"]) != `"`+base64.StdEncoding.EncodeToString(upload.Signature)+`"` {
		t.Fatalf("submit body %s", server.seen[1].body)
	}

	status, err := client.Version(ctx, server.URL, key, "acme_count", "1.0.0")
	if err != nil || status.TestsPassed == nil || !*status.TestsPassed || len(status.Findings) != 1 || status.Findings[0].Line != 3 || !status.Settled() {
		t.Fatalf("version: %+v, %v", status, err)
	}
	if len(server.seen) != 3 || server.seen[2].method != http.MethodGet || server.seen[2].path != "/v1/versions/acme_count/1.0.0" || len(server.seen[2].body) != 0 {
		t.Fatalf("requests %+v", server.seen)
	}
	if len(server.nonces) != 3 {
		t.Fatalf("%d distinct nonces for 3 requests", len(server.nonces))
	}
}

// A non-2xx answer surfaces its status and body; a 429 is RateLimited
// with its Retry-After; a wrong key or a replayed nonce is refused by the
// server, which the client reports as the registry's answer.
func TestClientReportsErrorsAndRateLimits(t *testing.T) {
	key := testKey(t)
	now := time.Unix(1760000000, 0)
	server := serveRegistry(t, key.Public, now)
	server.answers["POST /v1/publishers/enroll"] = answer{status: 409, body: `{"error":"publisher acme is active"}`}
	server.answers["POST /v1/versions"] = answer{status: 429, body: `{"error":"too many uploads"}`, retryAfter: "17"}
	client := &Client{Now: func() time.Time { return now }}
	ctx := context.Background()
	_, err := client.Enroll(ctx, server.URL, key, application.EnrollRequest{ID: "acme", Namespaces: []string{"acme_*"}})
	var registryErr *application.RegistryError
	if !errors.As(err, &registryErr) || registryErr.Status != 409 || !strings.Contains(err.Error(), "the registry answered 409: {\"error\":\"publisher acme is active\"}") {
		t.Fatalf("409: %v", err)
	}
	_, err = client.Submit(ctx, server.URL, key, application.UploadRequest{Manifest: []byte("{}"), KeyID: key.ID(), Signature: make([]byte, 64)})
	var limited *application.RateLimited
	if !errors.As(err, &limited) || limited.RetryAfter != 17*time.Second {
		t.Fatalf("429: %v", err)
	}
	// Another key's signature does not verify.
	other := testKey(t)
	_, err = client.Version(ctx, server.URL, other, "acme_count", "1.0.0")
	if !errors.As(err, &registryErr) || registryErr.Status != 400 {
		t.Fatalf("other key: %v", err)
	}
	// A stale clock is refused.
	stale := &Client{Now: func() time.Time { return now.Add(-10 * time.Minute) }}
	_, err = stale.Version(ctx, server.URL, key, "acme_count", "1.0.0")
	if !errors.As(err, &registryErr) || registryErr.Status != 401 || !strings.Contains(registryErr.Body, "stale") {
		t.Fatalf("stale: %v", err)
	}
	// Off the loopback, http is refused before anything is sent.
	if _, err := client.Version(ctx, "http://registry.example/", key, "a", "1.0.0"); err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("plain http: %v", err)
	}
	if retryAfter("") != time.Minute || retryAfter("5") != 5*time.Second {
		t.Fatal("Retry-After parsing")
	}
}

// The key store creates the key once, owner-only and in the CLI's file
// format, and refuses to replace it.
func TestKeyStoreCreatesOnceAndLoads(t *testing.T) {
	store := &KeyStore{Dir: filepath.Join(t.TempDir(), "registry")}
	ctx := context.Background()
	if key, err := store.Load(ctx); err != nil || key != nil {
		t.Fatalf("empty load: %v %v", key, err)
	}
	created, err := store.Create(ctx)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(store.Path())
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("key file mode %v, %v", info.Mode(), err)
	}
	data, _ := os.ReadFile(store.Path())
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 || lines[0] != KeyFileHeader || lines[1] != base64.StdEncoding.EncodeToString(created.Seed()) {
		t.Fatalf("key file:\n%s", data)
	}
	loaded, err := store.Load(ctx)
	if err != nil || loaded == nil || loaded.ID() != created.ID() || loaded.PublicBase64() != created.PublicBase64() {
		t.Fatalf("load: %+v %v", loaded, err)
	}
	if len(created.ID()) != 16 {
		t.Fatalf("key id %q", created.ID())
	}
	sum := sha256.Sum256(created.Public)
	if created.ID() != hex.EncodeToString(sum[:8]) {
		t.Fatal("key id is not the first 16 hex of SHA-256 of the public key")
	}
	if _, err := store.Create(ctx); err == nil || !strings.Contains(err.Error(), "exists") {
		t.Fatalf("second create: %v", err)
	}
	if _, err := ParseKeyFile("x", []byte("other header\nAAAA\n")); err == nil {
		t.Fatal("a foreign header was accepted")
	}
	if _, err := ParseKeyFile("x", []byte(KeyFileHeader+"\nAAAA\n")); err == nil {
		t.Fatal("a short seed was accepted")
	}
}
