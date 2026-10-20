package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// HostPublishToolName uploads a tool forged in this workspace to a catalog's
// registry. On 20 October 2026 the person asked the agent to publish
// image_frequency and it could not: only the person's /tools publish
// could, and the agent did not know the files the registry needs. The
// agent now prepares the tool (the axlr:axlr-publish-tool skill says how)
// and publishes it here, on a card the person always answers.
const HostPublishToolName root.ToolName = "axlr_publish_tool"

// ToolPublisherPort is what axlr_publish_tool needs of the registry client:
// the same steps /tools publish takes (CatalogUseCase).
type ToolPublisherPort interface {
	Prepare(ctx context.Context, workspace string, request PublishRequest) (PublishDraft, error)
	Bundle(ctx context.Context, draft PublishDraft) (PublishBundle, error)
	Submit(ctx context.Context, draft PublishDraft, bundle PublishBundle) (VersionReport, error)
}

// PublishTool is axlr_publish_tool.
func PublishTool() domain.AvailableTool {
	identity, _ := domain.NewHostToolIdentity(domain.HostOperationPublishTool)
	schema, _ := root.NewJSONObject([]byte(`{"type":"object","properties":{"name":{"type":"string","pattern":"^[a-z][a-z0-9_]{1,47}$"},"version":{"type":"string","pattern":"^\\d+\\.\\d+\\.\\d+$"},"catalog":{"type":"string","pattern":"^[a-z][a-z0-9_-]{0,31}$"}},"required":["name"],"additionalProperties":false}`))
	schema = requiredWhy(schema)
	return domain.AvailableTool{Identity: identity, Definition: root.ToolDefinition{
		Name:        HostPublishToolName,
		Description: "Publish a tool forged in this workspace to a catalog's registry so others can install it. Read the axlr:axlr-publish-tool skill first: the tool's files must sit at the top of its directory, and its forge must include tests.json (the registry's test cases) and catalog.json (version, category, tags, license, requires, network). The console uploads the registered files with both, signed with its key, after the person approves the card; the registry then scans and tests the version and an operator approves it. version defaults to catalog.json's and catalog to the only one with a registry. Never approved automatically.",
		Parameters:  schema,
	}}
}

// publishArgs are axlr_publish_tool's arguments.
type publishArgs struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Catalog string `json:"catalog"`
	Why     string `json:"why"`
}

func decodePublishArgs(arguments root.JSONValue) (publishArgs, error) {
	var args publishArgs
	decoder := json.NewDecoder(strings.NewReader(string(arguments.Bytes())))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&args); err != nil {
		return args, schemaError("axlr_publish_tool arguments must be name, with version and catalog optional", err)
	}
	if args.Name == "" {
		return args, errors.New("axlr_publish_tool arguments must be name, with version and catalog optional")
	}
	return args, nil
}

// publishDraft reads the tool for publishing and refuses what the registry
// could never approve, before the card: a draft without tests or with
// catalog fields missing. /tools publish asks the person for those fields;
// the agent writes them in catalog.json and forges again.
func publishDraft(ctx context.Context, publisher ToolPublisherPort, workspace string, arguments root.JSONValue) (PublishDraft, error) {
	args, err := decodePublishArgs(arguments)
	if err != nil {
		return PublishDraft{}, err
	}
	draft, err := publisher.Prepare(ctx, workspace, PublishRequest{Name: args.Name, Version: args.Version, Catalog: args.Catalog})
	if err != nil {
		return PublishDraft{}, err
	}
	if draft.Tests == nil {
		return PublishDraft{}, fmt.Errorf("%s has no %s: the registry would accept the version but could never approve it; forge the tool again with %s among its files (the axlr:axlr-publish-tool skill shows the cases' shape)", args.Name, PublishTestsFile, PublishTestsFile)
	}
	if len(draft.Missing) > 0 {
		return PublishDraft{}, fmt.Errorf("%s/%s/%s lacks %s: forge the tool again with %s among its files, a JSON object with version (1.0.0), category, tags, license (an SPDX id), requires {packages} and network", ForgedToolsDir, args.Name, PublishCatalogFile, strings.Join(draft.Missing, ", "), PublishCatalogFile)
	}
	return draft, nil
}

// publishArgumentError checks axlr_publish_tool before its card, so the
// person is never asked to approve an upload the console would refuse.
func publishArgumentError(ctx context.Context, publisher ToolPublisherPort, s domain.Session, tool domain.AvailableTool, arguments root.JSONValue) error {
	if tool.Definition.Name != HostPublishToolName {
		return nil
	}
	if publisher == nil {
		return fmt.Errorf("%s: publishing to a registry is not enabled in this console", HostPublishToolName)
	}
	if _, err := publishDraft(ctx, publisher, string(s.Export().Workspace), arguments); err != nil {
		return fmt.Errorf("%s: %w", HostPublishToolName, err)
	}
	return nil
}

// hostPublish uploads the tool the approved card named and returns the
// registry's first report; the scan and the sandbox tests settle later.
func (u HostToolUseCase) hostPublish(ctx context.Context, session domain.Session, arguments root.JSONValue) (any, error) {
	if u.Publisher == nil {
		return nil, errors.New("publishing to a registry is not enabled in this console")
	}
	draft, err := publishDraft(ctx, u.Publisher, string(session.Export().Workspace), arguments)
	if err != nil {
		return nil, err
	}
	bundle, err := u.Publisher.Bundle(ctx, draft)
	if err != nil {
		return nil, err
	}
	report, err := u.Publisher.Submit(ctx, draft, bundle)
	if err != nil {
		return nil, err
	}
	result := map[string]any{"published": draft.Tool.Name, "version": draft.Metadata.Version, "catalog": draft.Catalog, "status": report.Status, "findings": report.Findings, "key_id": bundle.KeyID,
		"next": "The registry scans the version and runs tests.json in its sandbox; an operator approves it before anyone can install it. The person follows it with /tools."}
	if report.RejectedReason != "" {
		result["rejected_reason"] = report.RejectedReason
	}
	if report.TestsPassed != nil {
		result["tests_passed"] = *report.TestsPassed
		result["failed_tests"] = report.FailedTests
	}
	return result, nil
}
