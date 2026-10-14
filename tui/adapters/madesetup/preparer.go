package madesetup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/underpass-ai/AXLR/mcpclient"
	"github.com/underpass-ai/AXLR/plugins"
	"github.com/underpass-ai/AXLR/tui/adapters/ceremonyhost"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/application"
)

const (
	hostIdentityKey  = "MADE_AUTH_TRUSTED_HOST_ID"
	embeddedLauncher = "run-embedded-mcp.sh"
)

// ToolCaller runs one MCP tool in a fresh process with a complete environment.
type ToolCaller func(ctx context.Context, server mcpclient.Server, tool string, arguments map[string]any) (map[string]any, error)

// Preparer issues AXLR's work grant through the store's trusted host. The
// launcher resolves that host from MADE's own embedded configuration when the
// work identity override is absent; AXLR never computes it.
type Preparer struct {
	ConfigPath string
	Getenv     func(string) string
	Store      EnvironmentStore
	Call       ToolCaller
	Now        func() time.Time
}

var _ application.OrchestratorPreparationPort = (*Preparer)(nil)

func (p *Preparer) Prepare(ctx context.Context) (application.OrchestratorPreparation, error) {
	result := application.OrchestratorPreparation{GrantID: WorkGrantID}
	if p.ConfigPath == "" || !filepath.IsAbs(p.ConfigPath) || p.Store == nil {
		return result, errors.New("MADE preparation is not configured")
	}
	configuration, err := storage.LoadMCPConfiguration(p.ConfigPath, p.Getenv)
	if err != nil {
		return result, err
	}
	var made *plugins.Registration
	for i := range configuration.Registrations {
		if configuration.Registrations[i].Manifest.ID == "made" {
			made = &configuration.Registrations[i]
		}
	}
	if made == nil {
		result.Status = "missing"
		return result, nil
	}
	values := environment(made.Env)
	if made.Manifest.URL != "" || values["MADE_MCP_BACKEND"] == "grpc" {
		result.Status, result.Detail = "remote", "the remote MADE operator must issue the grant"
		return result, nil
	}
	if filepath.Base(made.Manifest.Command) != embeddedLauncher {
		result.Status, result.Detail = "unsupported", "only the embedded MADE launcher resolves the trusted host"
		return result, nil
	}
	configured := values[hostIdentityKey]
	work := configured
	switch {
	case configured == "":
		digest := sha256.Sum256([]byte(p.ConfigPath))
		work = "axlr-work-" + hex.EncodeToString(digest[:8])
	case !isWorkIdentity(configured):
		result.Status, result.Detail = "unsupported", "the MADE entry sets an explicit trusted host; issue the grant as that operator"
		return result, nil
	}
	result.WorkIdentity = work
	result.GrantID = grantID(WorkGrantID, work)
	admin := server(made, without(made.Env, hostIdentityKey))
	issued, err := p.call(ctx, admin, "made_issue_authorization_grant", map[string]any{
		"grant_id": result.GrantID, "grantee_id": work, "scope": map[string]any{"kind": "global"},
		"valid_from": "2000-01-01T00:00:00Z", "delegation_depth": 0, "actions": append([]string(nil), workGrantActions...),
	})
	if err != nil {
		var refused *refusal
		if errors.As(err, &refused) && refused.Code == "conflict" {
			result.Status, result.Detail = "conflict", "a grant with this id already exists for another grantee or with other actions"
			return result, nil
		}
		return result, err
	}
	result.Status = "granted"
	if existing, _ := issued["existing"].(bool); existing {
		result.Status = "ready"
	}
	// The approver acts for the person at the console: it may grant human
	// guards, which the work identity may not.
	approved, err := p.call(ctx, admin, "made_issue_authorization_grant", map[string]any{
		"grant_id": grantID(ApproverGrantID, approverIdentity(work)), "grantee_id": approverIdentity(work), "scope": map[string]any{"kind": "global"},
		"valid_from": "2000-01-01T00:00:00Z", "delegation_depth": 0, "actions": append([]string(nil), approverActions...),
	})
	if err != nil {
		var refused *refusal
		if errors.As(err, &refused) && refused.Code == "conflict" {
			result.Status, result.Detail = "conflict", "an approver grant with this id already exists for another grantee or with other actions"
			return result, nil
		}
		return result, fmt.Errorf("issue the approver grant: %w", err)
	}
	if existing, _ := approved["existing"].(bool); !existing {
		result.Status = "granted"
	}
	result.ApproverIdentity = approverIdentity(work)
	if configured == "" {
		if err := p.Store.SetPluginEnvironment(ctx, made.Manifest.ID, hostIdentityKey, work); err != nil {
			return result, err
		}
		result.RestartRequired = true
	}
	worker := server(made, append(without(made.Env, hostIdentityKey), hostIdentityKey+"="+work))
	if _, err := p.call(ctx, worker, "made_list_ceremony_definitions", map[string]any{}); err != nil {
		return result, fmt.Errorf("work identity cannot read definitions after the grant: %w", err)
	}
	published, err := p.publishCeremonies(ctx, admin, worker, work)
	result.Published = published
	return result, err
}

// publishCeremonies publishes the console-driven definitions the store lacks.
// The work identity gets a five-minute install grant for the publication and
// loses it immediately after; it never keeps publication rights.
func (p *Preparer) publishCeremonies(ctx context.Context, admin, worker mcpclient.Server, work string) ([]string, error) {
	var missing []ceremonyhost.Definition
	for _, definition := range ceremonyhost.Definitions() {
		found, err := p.call(ctx, worker, "made_get_ceremony_definition", map[string]any{"ceremony": definition.Name, "version": definition.Version})
		if err == nil {
			if digest, _ := found["digest"].(string); digest != definition.Digest {
				return nil, fmt.Errorf("%s %s is already published with different content", definition.Name, definition.Version)
			}
			continue
		}
		var refused *refusal
		if !errors.As(err, &refused) || (refused.Code != "not_found" && !strings.Contains(refused.Message, "not found")) {
			return nil, err
		}
		missing = append(missing, definition)
	}
	if len(missing) == 0 {
		return nil, nil
	}
	now := p.now().UTC()
	grant := fmt.Sprintf("axlr-ceremony-install-%d", now.Unix())
	if _, err := p.call(ctx, admin, "made_issue_authorization_grant", map[string]any{
		"grant_id": grant, "grantee_id": work, "scope": map[string]any{"kind": "global"},
		"valid_from": "2000-01-01T00:00:00Z", "valid_until": now.Add(5 * time.Minute).Format(time.RFC3339), "delegation_depth": 0,
		"actions": []string{"validate_ceremony_draft", "publish_ceremony_definition"},
	}); err != nil {
		return nil, fmt.Errorf("issue the install grant: %w", err)
	}
	var published []string
	var publishErr error
	for _, definition := range missing {
		text, err := definition.YAML()
		if err == nil {
			_, err = p.call(ctx, worker, "made_publish_ceremony_definition", map[string]any{"definition_yaml": text})
		}
		if err != nil {
			publishErr = fmt.Errorf("publish %s %s: %w", definition.Name, definition.Version, err)
			break
		}
		published = append(published, definition.Name+" "+definition.Version)
	}
	_, revokeErr := p.call(ctx, admin, "made_revoke_authorization_grant", map[string]any{"grant_id": grant, "reason": "AXLR ceremony definitions installed"})
	if revokeErr != nil {
		revokeErr = fmt.Errorf("revoke install grant %s (it expires in five minutes): %w", grant, revokeErr)
	}
	return published, errors.Join(publishErr, revokeErr)
}

func (p *Preparer) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

func (p *Preparer) call(ctx context.Context, s mcpclient.Server, tool string, arguments map[string]any) (map[string]any, error) {
	if p.Call == nil {
		return CallOnce(ctx, s, tool, arguments)
	}
	return p.Call(ctx, s, tool, arguments)
}

// isWorkIdentity recognises identities this setup or the documented manual
// setup created; any other value is an operator's trusted host.
func isWorkIdentity(id string) bool {
	return strings.HasPrefix(id, "axlr-work-") || strings.HasSuffix(id, "-axlr-work")
}

func server(r *plugins.Registration, env []string) mcpclient.Server {
	return mcpclient.Server{Name: mcpclient.ServerName(r.Manifest.ID.String()), Command: r.Manifest.Command, Args: r.Manifest.Args, Env: env}
}

func environment(env []string) map[string]string {
	values := map[string]string{}
	for _, entry := range env {
		if key, value, ok := strings.Cut(entry, "="); ok {
			values[key] = value
		}
	}
	return values
}

func without(env []string, key string) []string {
	out := make([]string, 0, len(env))
	for _, entry := range env {
		if !strings.HasPrefix(entry, key+"=") {
			out = append(out, entry)
		}
	}
	return out
}

// CallOnce starts the server, calls one tool and stops it.
func CallOnce(ctx context.Context, s mcpclient.Server, tool string, arguments map[string]any) (map[string]any, error) {
	name, err := mcpclient.NewToolName(tool)
	if err != nil {
		return nil, err
	}
	client := mcpclient.New()
	defer client.Close()
	if err := client.Connect(ctx, s); err != nil {
		return nil, err
	}
	response, err := client.Call(ctx, mcpclient.ToolRef{Server: s.Name, Name: name}, arguments)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{}
	switch {
	case len(response.StructuredContent) > 0:
		err = json.Unmarshal(response.StructuredContent, &payload)
	case len(response.Content) > 0:
		var block struct{ Text string }
		if err = json.Unmarshal(response.Content[0], &block); err == nil {
			err = json.Unmarshal([]byte(block.Text), &payload)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("decode %s result: %w", tool, err)
	}
	if response.IsError {
		code, _ := payload["code"].(string)
		message, _ := payload["message"].(string)
		return nil, &refusal{Code: code, Message: message}
	}
	return payload, nil
}

// WorkIdentity is the identity the console's MADE registration works as:
// the configured work identity, or the one this setup derives from the
// configuration path when the entry sets none; "" when the entry names an
// operator's trusted host or is absent.
func WorkIdentity(configPath string, env []string) string {
	configured := environment(env)[hostIdentityKey]
	switch {
	case configured == "":
		digest := sha256.Sum256([]byte(configPath))
		return "axlr-work-" + hex.EncodeToString(digest[:8])
	case isWorkIdentity(configured):
		return configured
	}
	return ""
}

// Publish validates and publishes one definition under a five-minute
// install grant, as publishCeremonies does for the console's own; the work
// identity never keeps publication rights.
func (p *Preparer) Publish(ctx context.Context, definitionYAML string) (string, error) {
	if strings.TrimSpace(definitionYAML) == "" {
		return "", errors.New("the definition is empty")
	}
	if p.ConfigPath == "" || !filepath.IsAbs(p.ConfigPath) || p.Store == nil {
		return "", errors.New("MADE preparation is not configured")
	}
	configuration, err := storage.LoadMCPConfiguration(p.ConfigPath, p.Getenv)
	if err != nil {
		return "", err
	}
	var made *plugins.Registration
	for i := range configuration.Registrations {
		if configuration.Registrations[i].Manifest.ID == "made" {
			made = &configuration.Registrations[i]
		}
	}
	if made == nil {
		return "", errors.New("MADE is not registered")
	}
	values := environment(made.Env)
	if made.Manifest.URL != "" || values["MADE_MCP_BACKEND"] == "grpc" || filepath.Base(made.Manifest.Command) != embeddedLauncher {
		return "", errors.New("only the embedded MADE launcher can publish from this console; the remote operator publishes")
	}
	work := WorkIdentity(p.ConfigPath, made.Env)
	if work == "" {
		return "", errors.New("the MADE entry sets an explicit trusted host; publish as that operator")
	}
	admin := server(made, without(made.Env, hostIdentityKey))
	worker := server(made, append(without(made.Env, hostIdentityKey), hostIdentityKey+"="+work))
	now := p.now().UTC()
	grant := fmt.Sprintf("axlr-ceremony-install-%d", now.Unix())
	if _, err := p.call(ctx, admin, "made_issue_authorization_grant", map[string]any{
		"grant_id": grant, "grantee_id": work, "scope": map[string]any{"kind": "global"},
		"valid_from": "2000-01-01T00:00:00Z", "valid_until": now.Add(5 * time.Minute).Format(time.RFC3339), "delegation_depth": 0,
		"actions": []string{"validate_ceremony_draft", "publish_ceremony_definition"},
	}); err != nil {
		return "", fmt.Errorf("issue the install grant: %w", err)
	}
	var name string
	report, err := p.call(ctx, worker, "made_validate_ceremony_draft", map[string]any{"definition_yaml": definitionYAML})
	if err == nil {
		if valid, ok := report["valid"].(bool); ok && !valid {
			err = fmt.Errorf("the definition is not publishable: %v", bounded(fmt.Sprint(report["problems"]), 600))
		}
	}
	if err == nil {
		var published map[string]any
		published, err = p.call(ctx, worker, "made_publish_ceremony_definition", map[string]any{"definition_yaml": definitionYAML})
		if err == nil {
			ceremony, _ := published["ceremony"].(string)
			version, _ := published["version"].(string)
			name = strings.TrimSpace(ceremony + " " + version)
		}
	}
	_, revokeErr := p.call(ctx, admin, "made_revoke_authorization_grant", map[string]any{"grant_id": grant, "reason": "AXLR ceremony definition published"})
	if revokeErr != nil {
		revokeErr = fmt.Errorf("revoke install grant %s (it expires in five minutes): %w", grant, revokeErr)
	}
	if err != nil {
		return "", errors.Join(err, revokeErr)
	}
	return name, revokeErr
}

func bounded(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + "…"
}
