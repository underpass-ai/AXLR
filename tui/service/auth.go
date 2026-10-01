package service

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
)

type Principal struct {
	ID    string
	Roles map[string]bool
}

type principalEntry struct {
	Fingerprint string   `json:"certificate_sha256"`
	ID          string   `json:"principal_id"`
	Roles       []string `json:"roles"`
}

type principalPolicy struct {
	Version int              `json:"version"`
	Entries []principalEntry `json:"entries"`
}

func loadPrincipals(path string) (map[string]Principal, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) > 1<<20 {
		return nil, errors.New("principal policy too large")
	}
	var policy principalPolicy
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&policy); err != nil {
		return nil, err
	}
	if err := d.Decode(new(any)); err != io.EOF || policy.Version != 1 {
		return nil, errors.New("invalid principal policy version or trailing data")
	}
	result := map[string]Principal{}
	for _, entry := range policy.Entries {
		if len(entry.Fingerprint) != 64 || strings.ToLower(entry.Fingerprint) != entry.Fingerprint || entry.ID == "" || len(entry.Roles) == 0 {
			return nil, errors.New("invalid principal entry")
		}
		if _, err := hex.DecodeString(entry.Fingerprint); err != nil {
			return nil, err
		}
		if _, exists := result[entry.Fingerprint]; exists {
			return nil, errors.New("duplicate client certificate")
		}
		p := Principal{ID: entry.ID, Roles: map[string]bool{}}
		for _, role := range entry.Roles {
			switch role {
			case "session_client", "approver", "tool_operator", "admin":
			default:
				return nil, errors.New("unknown principal role")
			}
			if p.Roles[role] {
				return nil, errors.New("duplicate principal role")
			}
			p.Roles[role] = true
		}
		result[entry.Fingerprint] = p
	}
	return result, nil
}

func serverTLSConfig(cfg Config) (*tls.Config, error) {
	ca, err := os.ReadFile(cfg.ClientCAFile)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return nil, errors.New("invalid client CA")
	}
	if _, err := tls.LoadX509KeyPair(cfg.ServerCertFile, cfg.ServerKeyFile); err != nil {
		return nil, err
	}
	return &tls.Config{
		MinVersion: tls.VersionTLS13,
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  pool,
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			cert, err := tls.LoadX509KeyPair(cfg.ServerCertFile, cfg.ServerKeyFile)
			return &cert, err
		},
	}, nil
}

func authenticate(r *http.Request, policy map[string]Principal) (Principal, bool) {
	if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.PeerCertificates) == 0 {
		return Principal{}, false
	}
	digest := sha256.Sum256(r.TLS.PeerCertificates[0].Raw)
	p, ok := policy[hex.EncodeToString(digest[:])]
	return p, ok
}

func (p Principal) Can(role string) bool { return p.Roles[role] || p.Roles["admin"] }
