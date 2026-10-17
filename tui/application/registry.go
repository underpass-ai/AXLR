package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

// A registry (axlr-registry docs/api.md) is the write side of a catalog:
// the service a publisher enrols with and uploads tool versions to, at
// the catalog's api_url. Every request is signed with the client key
// (docs/client-identity.md §3); RegistryPort signs them.

// RegistryPort speaks to a registry at an api URL with a client key.
type RegistryPort interface {
	// Enroll is POST /v1/publishers/enroll; a 202 is a pending enrolment.
	Enroll(ctx context.Context, api string, key *ClientKey, request EnrollRequest) (EnrollResult, error)
	// Submit is POST /v1/versions; a 201 is the version's first report.
	Submit(ctx context.Context, api string, key *ClientKey, upload UploadRequest) (VersionReport, error)
	// Version is GET /v1/versions/{name}/{version}, the publisher's view.
	Version(ctx context.Context, api string, key *ClientKey, name, version string) (VersionReport, error)
}

// EnrollRequest is the enrolment body: the publisher id, the key being
// enrolled (its public key, added by the adapter), the namespaces asked
// for and, when given, a contact.
type EnrollRequest struct {
	ID         string
	Namespaces []string
	Contact    string
}

// EnrollResult is a 202's body.
type EnrollResult struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// UploadRequest is what POST /v1/versions carries: the manifest bytes,
// the files, the optional tests.json, and the bundle signature by key_id.
type UploadRequest struct {
	Manifest  []byte
	Files     map[string][]byte
	Tests     []byte
	KeyID     string
	Signature []byte
}

// VersionReport is a version as the publisher sees it, from the upload's
// answer and from GET /v1/versions/{name}/{version}.
type VersionReport struct {
	Name           string    `json:"name"`
	Version        string    `json:"version"`
	Status         string    `json:"status"`
	SubmittedAt    string    `json:"submitted_at,omitempty"`
	Findings       []Finding `json:"findings"`
	TestsPassed    *bool     `json:"tests_passed"`
	FailedTests    []string  `json:"failed_tests,omitempty"`
	RejectedReason string    `json:"rejected_reason,omitempty"`
}

// Settled reports whether the scan and the sandbox tests are done: the
// tests reported, or the scan rejected the version first.
func (r VersionReport) Settled() bool {
	return r.TestsPassed != nil || (r.Status != "" && r.Status != "scanned")
}

// Finding is one policy-scan finding.
type Finding struct {
	Rule     string `json:"rule"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Severity string `json:"severity"`
	Message  string `json:"message,omitempty"`
}

// RegistryError is a non-2xx answer with its body, which the registry
// writes as {"error": "…"}.
type RegistryError struct {
	Status int
	Body   string
}

func (e *RegistryError) Error() string {
	body := strings.TrimSpace(e.Body)
	if body == "" {
		return fmt.Sprintf("the registry answered %d", e.Status)
	}
	return fmt.Sprintf("the registry answered %d: %s", e.Status, body)
}

// RateLimited is a 429 with its Retry-After.
type RateLimited struct {
	RetryAfter time.Duration
}

func (e *RateLimited) Error() string {
	return fmt.Sprintf("the registry is rate limiting this key; retry after %s", e.RetryAfter)
}

// BundleDigest is the SHA-256 of the listing "manifest <sha>\n" then
// "file <name> <sha>\n" for every file in name order (docs/api.md).
func BundleDigest(manifest []byte, files map[string][]byte) string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	var listing strings.Builder
	listing.WriteString("manifest " + sha256Hex(manifest) + "\n")
	for _, name := range names {
		listing.WriteString("file " + name + " " + sha256Hex(files[name]) + "\n")
	}
	return sha256Hex([]byte(listing.String()))
}

// BundleSignatureMessage is what the client key signs for a bundle.
func BundleSignatureMessage(digest string) []byte {
	return []byte("axlr-tool-bundle-v1\n" + digest)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
