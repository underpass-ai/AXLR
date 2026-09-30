package service

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/adapters/axlr"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type fixedModel struct{}

type fixedTool struct{}

func (fixedTool) Execute(context.Context, domain.ToolIdentity, root.JSONValue) (domain.ToolOutcome, error) {
	return domain.ToolOutcome{Content: "done"}, nil
}

func (fixedModel) Stream(_ context.Context, _ root.CompletionRequest, emit func(root.Text) error) (root.CompletionResult, error) {
	if err := emit("hello"); err != nil {
		return root.CompletionResult{}, err
	}
	return root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "hello"}}, nil
}

func certificate(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey, server bool) (tls.Certificate, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "test"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
	if server {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		template.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	} else {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	priv, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: priv})
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return pair, append(certPEM, keyPEM...)
}

func testServer(t *testing.T) (*Server, *httptest.Server, *http.Client, *http.Client) {
	t.Helper()
	dir := t.TempDir()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, err = x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	serverCert, serverPEM := certificate(t, ca, caKey, true)
	clientCert, _ := certificate(t, ca, caKey, false)
	unmapped, _ := certificate(t, ca, caKey, false)
	write := func(name string, data []byte) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	serverBlock, _ := pem.Decode(serverPEM)
	serverKeyPEM := serverPEM[len(pem.EncodeToMemory(serverBlock)):]
	digest := sha256.Sum256(clientCert.Certificate[0])
	policy, _ := json.Marshal(map[string]any{"version": 1, "entries": []any{map[string]any{"certificate_sha256": hex.EncodeToString(digest[:]), "principal_id": "alice", "roles": []string{"session_client", "approver", "tool_operator"}}}})
	cfg := Config{APIListen: "127.0.0.1:0", ProbeListen: "127.0.0.1:0", Workspace: dir, StateDir: filepath.Join(dir, "state"), ServerCertFile: write("server.crt", pem.EncodeToMemory(serverBlock)), ServerKeyFile: write("server.key", serverKeyPEM), ClientCAFile: write("ca.crt", caPEM), PrincipalsFile: write("principals.json", policy), ModelAPIKeyFile: write("model-key", []byte("key")), KMP: EngineConfig{Endpoint: "127.0.0.1:1", ServerName: "kmp.example", TLSDir: dir, Command: filepath.Join(dir, "kmp.exe")}, MADE: EngineConfig{Endpoint: "127.0.0.1:2", ServerName: "made.example", TLSDir: dir, Command: filepath.Join(dir, "made.exe")}}
	validator := axlr.NewToolArgumentValidator()
	continuation := application.ContinueTurnUseCase{Models: fixedModel{}, Validation: validator}
	deps := Dependencies{Catalog: axlr.ToolCatalog{}, Start: application.StartTurnUseCase{Catalog: axlr.ToolCatalog{}, Continue: continuation, Tools: fixedTool{}}, Resolve: application.ResolveToolUseCase{Continue: continuation, Tools: fixedTool{}}, Tools: fixedTool{}, Validation: validator, Ready: func(context.Context) error { return nil }}
	s, err := NewServer(cfg, deps)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewUnstartedServer(s.APIHandler())
	ts.TLS, err = serverTLSConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if ts.TLS.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Fatal("mTLS is not required")
	}
	ts.TLS.GetCertificate = nil
	ts.TLS.Certificates = []tls.Certificate{serverCert}
	ts.StartTLS()
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caPEM)
	client := func(cert tls.Certificate) *http.Client {
		return &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS13}}}
	}
	t.Cleanup(func() { ts.Close(); s.Close() })
	return s, ts, client(clientCert), client(unmapped)
}

func apiRequest(t *testing.T, client *http.Client, method, url, body, key string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func TestMTLSRolesSessionsAndIdempotentTurn(t *testing.T) {
	s, ts, client, unmapped := testServer(t)
	response := apiRequest(t, unmapped, "POST", ts.URL+"/v1/sessions", `{"model":"test/model"}`, "")
	if response.StatusCode != 403 {
		t.Fatalf("unmapped cert: %d", response.StatusCode)
	}
	response.Body.Close()
	response = apiRequest(t, client, "POST", ts.URL+"/v1/sessions", `{"model":"test/model","extra":1}`, "")
	if response.StatusCode != 400 {
		t.Fatalf("unknown field: %d", response.StatusCode)
	}
	response.Body.Close()
	response = apiRequest(t, client, "POST", ts.URL+"/v1/sessions", `{"model":"test/model"}`, "")
	if response.StatusCode != 201 {
		t.Fatalf("create: %d", response.StatusCode)
	}
	var created struct {
		ID       string `json:"id"`
		Revision uint64 `json:"revision"`
	}
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if created.Revision != 1 {
		t.Fatal(created)
	}
	url := ts.URL + "/v1/sessions/" + created.ID + "/turns"
	body := `{"prompt":"hi","expected_revision":1}`
	response = apiRequest(t, client, "POST", url, body, "0123456789abcdef")
	if response.StatusCode != 202 {
		t.Fatalf("turn: %d", response.StatusCode)
	}
	var first map[string]any
	if err := json.NewDecoder(response.Body).Decode(&first); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	deadline := time.Now().Add(2 * time.Second)
	for {
		events, _, err := s.events.Read(created.ID, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(events) >= 3 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("events: %+v", events)
		}
		time.Sleep(10 * time.Millisecond)
	}
	response = apiRequest(t, client, "POST", url, body, "0123456789abcdef")
	if response.StatusCode != 202 {
		t.Fatalf("replay: %d", response.StatusCode)
	}
	var replay map[string]any
	if err := json.NewDecoder(response.Body).Decode(&replay); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if replay["operation_id"] != first["operation_id"] {
		t.Fatalf("new operation on retry: %v %v", first, replay)
	}
	response = apiRequest(t, client, "POST", url, `{"prompt":"changed","expected_revision":1}`, "0123456789abcdef")
	if response.StatusCode != 409 {
		t.Fatalf("conflict: %d", response.StatusCode)
	}
	response.Body.Close()
	response = apiRequest(t, client, "GET", ts.URL+"/v1/sessions/"+created.ID, "", "")
	if response.StatusCode != 200 {
		t.Fatalf("get: %d", response.StatusCode)
	}
	response.Body.Close()
}

func TestProbeListenerHasNoDataAndUncertifiedClientFails(t *testing.T) {
	s, ts, _, _ := testServer(t)
	probe := httptest.NewServer(s.ProbeHandler())
	defer probe.Close()
	response, err := http.Get(probe.URL + "/v1/sessions/0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 404 {
		t.Fatalf("probe exposed data: %d", response.StatusCode)
	}
	response.Body.Close()
	response, err = (&http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}).Get(ts.URL + "/v1/tools")
	if err == nil {
		response.Body.Close()
		t.Fatal("client without certificate connected")
	}
}

func TestDirectCallDenyAndReplay(t *testing.T) {
	_, ts, client, _ := testServer(t)
	response := apiRequest(t, client, "GET", ts.URL+"/v1/tools?origin=local", "", "")
	if response.StatusCode != 200 {
		t.Fatalf("catalog: %d", response.StatusCode)
	}
	response.Body.Close()
	body := `{"tool":"local_read","arguments":{"path":"sample.txt"}}`
	response = apiRequest(t, client, "POST", ts.URL+"/v1/tool-calls", body, "abcdef0123456789")
	if response.StatusCode != 202 {
		t.Fatalf("create: %d", response.StatusCode)
	}
	var created struct {
		ID       string `json:"call_id"`
		Revision uint64 `json:"revision"`
		Status   string `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if created.Revision != 1 || created.Status != "pending_approval" {
		t.Fatal(created)
	}
	decision := `{"decision":"deny","expected_revision":1}`
	url := ts.URL + "/v1/tool-calls/" + created.ID + "/decisions"
	response = apiRequest(t, client, "POST", url, decision, "1234567890abcdef")
	if response.StatusCode != 202 {
		t.Fatalf("deny: %d", response.StatusCode)
	}
	response.Body.Close()
	response = apiRequest(t, client, "POST", url, decision, "1234567890abcdef")
	if response.StatusCode != 202 {
		t.Fatalf("decision replay: %d", response.StatusCode)
	}
	response.Body.Close()
	response = apiRequest(t, client, "GET", ts.URL+"/v1/tool-calls/"+created.ID, "", "")
	if response.StatusCode != 200 {
		t.Fatalf("get: %d", response.StatusCode)
	}
	var result struct {
		Status   string `json:"status"`
		Revision uint64 `json:"revision"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if result.Status != "rejected" || result.Revision != 2 {
		t.Fatal(result)
	}
}

func TestSSEReplayAndProbeReadiness(t *testing.T) {
	s, ts, client, _ := testServer(t)
	response := apiRequest(t, client, "POST", ts.URL+"/v1/sessions", `{"model":"test/model"}`, "")
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	for _, kind := range []string{"turn.started", "text.delta"} {
		if _, err := s.events.Append(created.ID, "op", kind, map[string]string{"text": "a"}); err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequest("GET", ts.URL+"/v1/sessions/"+created.ID+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Last-Event-ID", "1")
	response, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("stream: %d", response.StatusCode)
	}
	line, err := bufio.NewReader(response.Body).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if line != "id: 2\n" {
		t.Fatalf("wrong replay cursor: %q", line)
	}
	probe := httptest.NewServer(s.ProbeHandler())
	defer probe.Close()
	ready, err := http.Get(probe.URL + "/readyz")
	if err != nil {
		t.Fatal(err)
	}
	if ready.StatusCode != 204 {
		t.Fatalf("readyz: %d", ready.StatusCode)
	}
	ready.Body.Close()
}

func TestDirectApprovalRunsExactCallOnce(t *testing.T) {
	s, ts, client, _ := testServer(t)
	response := apiRequest(t, client, "POST", ts.URL+"/v1/tool-calls", `{"tool":"local_read","arguments":{"path":"sample.txt"}}`, "1234567890abcdef")
	if response.StatusCode != 202 {
		t.Fatalf("create: %d", response.StatusCode)
	}
	var created struct {
		ID string `json:"call_id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	url := ts.URL + "/v1/tool-calls/" + created.ID + "/decisions"
	response = apiRequest(t, client, "POST", url, `{"decision":"approve","expected_revision":1}`, "abcdef1234567890")
	if response.StatusCode != 202 {
		t.Fatalf("approve: %d", response.StatusCode)
	}
	response.Body.Close()
	deadline := time.Now().Add(2 * time.Second)
	for {
		response = apiRequest(t, client, "GET", ts.URL+"/v1/tool-calls/"+created.ID, "", "")
		var result struct {
			Status   string `json:"status"`
			Revision uint64 `json:"revision"`
		}
		if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if result.Status == "completed" {
			if result.Revision != 4 {
				t.Fatal(result)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("call did not finish: %+v", result)
		}
		time.Sleep(10 * time.Millisecond)
	}
	audit, err := os.ReadFile(filepath.Join(s.Config.StateDir, "audit", "audit.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{`"principal":"alice"`, `"action":"tool_call.create"`, `"action":"tool_call.decision"`, `"action":"tool_call.execute"`, `"action":"tool_call.result"`, `"tool":"local_read"`} {
		if !strings.Contains(string(audit), required) {
			t.Fatalf("missing audit field %s: %s", required, audit)
		}
	}
	if strings.Contains(string(audit), "sample.txt") || strings.Contains(string(audit), `"arguments"`) || strings.Contains(string(audit), `"result"`) {
		t.Fatalf("sensitive values in audit: %s", audit)
	}
}
