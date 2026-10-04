# Runbook: keep documentation aligned with code

Use this when behavior, configuration, packaging or product responsibilities change. Current guides describe the checked-out implementation; plans describe intent. Keep that distinction visible at the page entry.

## Establish the baseline

```bash
git status --short --branch
git fetch origin main
git rev-parse HEAD origin/main
```

Record the exact revision and any pre-existing changes. If the checkout differs from the requested baseline, inspect that revision explicitly without overwriting another person's work. Recover project memory from the intended KMP store when available; disclose missing or wrong-store memory rather than inferring prior decisions.

## Trace claims to implementation

| Claim | Start here |
|:--|:--|
| Product responsibilities / interface parity | `docs/product.md`, both CLI wiring files under `tui/cmd/` |
| Flags and environment | `cmd/axlr/`, `tui/cmd/axlr-tui/run.go`, `tui/cmd/axlr-serve/run.go` |
| Modes, approvals, context and sessions | `tui/domain/`, `tui/application/`, `tui/adapters/storage/` |
| Ceremony execution and setup | `tui/application/ceremony_driver.go`, `tui/adapters/ceremonyhost/`, `tui/adapters/madesetup/` |
| Plugins, schemas, transport and package compatibility | `plugins/`, `mcpclient/`, `tui/adapters/axlrplugin/` |
| API routes, failures and recovery | `tui/service/`, `api/openapi/axlr-v1.yaml` |
| Release/platform claims | `.github/workflows/release.yml`, `distribution/`, `scripts/release/`, `charts/axlr/` |

Read the relevant tests as evidence of edge cases. Distinguish implemented behavior, tested behavior and external deployment/release verification. A checked-in workflow is not proof that a release succeeded.

## Revise the reader's path

Update the owning guide first, then README/index links and related runbooks. Give each runbook preconditions, actual steps, observable success and recovery. Keep examples consistent with parsed fields and file/engine identities. Do not turn a product direction into a claim of implemented behavior.

Preserve dated design records. Add a short status pointer when an old plan is easy to mistake for an operational guide. Do not rewrite the history to make an earlier proposal appear implemented at the time.

## Verify and record

Run the two module suites when checking runtime claims:

```bash
GOWORK=off go test ./...
GOWORK=off go -C tui test ./...
git diff --check
```

Build affected entry points and exercise help/version and a harmless worker request. Check all relative Markdown links/fragments and complete JSON examples. If deployment examples changed, run Helm lint/template with the checked-in example values. Validate OpenAPI against the actual handlers, including errors and unsupported endpoints. No live model call, engine-store write or deployment is needed for those checks.

Review the resulting diff for contradictory defaults, unimplemented promises, stale paths, credentials and orphan guides. Record the audited revision, source-backed findings, corrections, checks actually run and unverified boundaries in [the audit](../documentation-audit.md). New operational guides belong in [the documentation index](../index.md).
