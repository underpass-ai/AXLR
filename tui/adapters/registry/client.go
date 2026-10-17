package registry

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/underpass-ai/AXLR/tui/application"
)

// Client speaks to a registry's publish API with signed requests; it
// satisfies application.RegistryPort. Every request carries the headers
// of docs/client-identity.md §3 over the canonical message of
// RequestMessage, a fresh nonce and the clock's seconds, and the
// User-Agent the registry's client gate expects.
type Client struct {
	HTTP *http.Client
	// Now is the clock the timestamp reads; nil uses time.Now.
	Now func() time.Time
}

var _ application.RegistryPort = (*Client)(nil)

const (
	userAgent      = "AXLR-catalog"
	requestTimeout = 60 * time.Second
	maxAnswerBytes = 1 << 20
	nonceBytes     = 16
)

// Signed-request headers.
const (
	HeaderKey       = "X-AXLR-Key"
	HeaderTimestamp = "X-AXLR-Timestamp"
	HeaderNonce     = "X-AXLR-Nonce"
	HeaderSignature = "X-AXLR-Signature"
)

// RequestMessage is the canonical message a signature covers: the
// version line, the method in upper case, the path without its query,
// the query exactly as sent (or empty), the SHA-256 of the body in hex
// (the hash of the empty body when there is none), the timestamp and the
// nonce, each line ended by \n.
func RequestMessage(method, path, query string, body []byte, timestamp int64, nonce string) []byte {
	sum := sha256.Sum256(body)
	return []byte("axlr-request-v1\n" + strings.ToUpper(method) + "\n" + path + "\n" + query + "\n" + hex.EncodeToString(sum[:]) + "\n" + strconv.FormatInt(timestamp, 10) + "\n" + nonce + "\n")
}

// Enroll implements application.RegistryPort.
func (c *Client) Enroll(ctx context.Context, api string, key *application.ClientKey, request application.EnrollRequest) (application.EnrollResult, error) {
	encoded, err := json.Marshal(map[string]any{"id": request.ID, "public_key": key.PublicBase64(), "namespaces": request.Namespaces, "contact": request.Contact})
	if err != nil {
		return application.EnrollResult{}, err
	}
	status, answer, err := c.do(ctx, api, key, http.MethodPost, "/v1/publishers/enroll", encoded)
	if err != nil {
		return application.EnrollResult{}, err
	}
	if status != http.StatusAccepted {
		return application.EnrollResult{}, &application.RegistryError{Status: status, Body: string(answer)}
	}
	var result application.EnrollResult
	if err := json.Unmarshal(answer, &result); err != nil {
		return application.EnrollResult{}, fmt.Errorf("the registry's 202 is not JSON: %v", err)
	}
	return result, nil
}

// Submit implements application.RegistryPort.
func (c *Client) Submit(ctx context.Context, api string, key *application.ClientKey, upload application.UploadRequest) (application.VersionReport, error) {
	files := make(map[string]string, len(upload.Files))
	for name, content := range upload.Files {
		files[name] = base64.StdEncoding.EncodeToString(content)
	}
	body := map[string]any{
		"manifest":  base64.StdEncoding.EncodeToString(upload.Manifest),
		"files":     files,
		"key_id":    upload.KeyID,
		"signature": base64.StdEncoding.EncodeToString(upload.Signature),
	}
	if upload.Tests != nil {
		body["tests"] = base64.StdEncoding.EncodeToString(upload.Tests)
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return application.VersionReport{}, err
	}
	status, answer, err := c.do(ctx, api, key, http.MethodPost, "/v1/versions", encoded)
	if err != nil {
		return application.VersionReport{}, err
	}
	if status != http.StatusCreated {
		return application.VersionReport{}, &application.RegistryError{Status: status, Body: string(answer)}
	}
	return parseReport(answer)
}

// Version implements application.RegistryPort.
func (c *Client) Version(ctx context.Context, api string, key *application.ClientKey, name, version string) (application.VersionReport, error) {
	status, answer, err := c.do(ctx, api, key, http.MethodGet, "/v1/versions/"+url.PathEscape(name)+"/"+url.PathEscape(version), nil)
	if err != nil {
		return application.VersionReport{}, err
	}
	if status != http.StatusOK {
		return application.VersionReport{}, &application.RegistryError{Status: status, Body: string(answer)}
	}
	return parseReport(answer)
}

func parseReport(answer []byte) (application.VersionReport, error) {
	var report application.VersionReport
	if err := json.Unmarshal(answer, &report); err != nil {
		return application.VersionReport{}, fmt.Errorf("the registry's answer is not a version report: %v", err)
	}
	return report, nil
}

// do sends one signed request and returns the status and the body; a 429
// is a RateLimited error with its Retry-After.
func (c *Client) do(ctx context.Context, api string, key *application.ClientKey, method, path string, body []byte) (int, []byte, error) {
	if key == nil {
		return 0, nil, errors.New("no client key to sign the request with")
	}
	base, err := parseAPI(api)
	if err != nil {
		return 0, nil, err
	}
	target := *base
	target.Path = strings.TrimSuffix(base.Path, "/") + path
	target.RawPath = ""
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, target.String(), reader)
	if err != nil {
		return 0, nil, err
	}
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	nonce, err := newNonce()
	if err != nil {
		return 0, nil, err
	}
	timestamp := now().Unix()
	message := RequestMessage(method, req.URL.EscapedPath(), req.URL.RawQuery, body, timestamp, nonce)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set(HeaderKey, key.ID())
	req.Header.Set(HeaderTimestamp, strconv.FormatInt(timestamp, 10))
	req.Header.Set(HeaderNonce, nonce)
	req.Header.Set(HeaderSignature, base64.StdEncoding.EncodeToString(key.Sign(message)))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{}
	}
	secured := *client
	// A signed request is for one host: a redirect would resend the
	// signature elsewhere.
	secured.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := secured.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("cannot reach the registry at %s: network request failed", base.Host)
	}
	defer response.Body.Close()
	answer, err := io.ReadAll(io.LimitReader(response.Body, maxAnswerBytes+1))
	if err != nil {
		return 0, nil, err
	}
	if len(answer) > maxAnswerBytes {
		return 0, nil, fmt.Errorf("the registry's answer exceeds %d bytes", maxAnswerBytes)
	}
	if response.StatusCode == http.StatusTooManyRequests {
		return 0, nil, &application.RateLimited{RetryAfter: retryAfter(response.Header.Get("Retry-After"))}
	}
	return response.StatusCode, answer, nil
}

// retryAfter reads a Retry-After in seconds; an absent or odd value is a
// minute.
func retryAfter(header string) time.Duration {
	seconds, err := strconv.Atoi(strings.TrimSpace(header))
	if err != nil || seconds < 1 {
		return time.Minute
	}
	return time.Duration(seconds) * time.Second
}

func newNonce() (string, error) {
	raw := make([]byte, nonceBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

// parseAPI accepts a registry's api URL: https, or http on the loopback.
func parseAPI(api string) (*url.URL, error) {
	parsed, err := url.Parse(api)
	if err != nil || parsed.Host == "" {
		return nil, fmt.Errorf("api_url %q is not an absolute URL", api)
	}
	switch parsed.Scheme {
	case "https":
	case "http":
		host := parsed.Hostname()
		ip := net.ParseIP(host)
		if host != "localhost" && !strings.HasSuffix(host, ".localhost") && (ip == nil || !ip.IsLoopback()) {
			return nil, fmt.Errorf("api_url %s must use https off the loopback", api)
		}
	default:
		return nil, fmt.Errorf("api_url %s must use https", api)
	}
	parsed.RawQuery, parsed.Fragment = "", ""
	return parsed, nil
}
