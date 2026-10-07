# Documentation audit — 4 October 2026

## 7 October 2026: local OpenAI-compatible models

`settings.json` gains `local_models` (OpenAI-compatible servers listed in `/model` together with OpenRouter's catalog) and a top-level `context_tokens` that caps the window for every model. The root client takes an `Endpoint`; a loopback endpoint needs no key and sends no `Authorization` header, any other host needs `https` and a key; its provider, transport and timeout errors name the endpoint instead of OpenRouter; it asks a non-OpenRouter server for stream usage. `OPENROUTER_API_KEY` becomes optional once a local model is configured. The diagnostics transport traces a configured local endpoint like OpenRouter, so payload capture and the "model is reasoning" status keep working. The domain learns only the context window: the projection's four byte limits shrink in proportion when the window is too small for the default. The [console guide](console.md#local-models), [architecture](architecture.md), [getting started](getting-started.md), [troubleshooting](troubleshooting.md#local-models), the README and the TUI README are updated.

Deviation from the handoff: the window is not stored with the session. It is resolved at each turn from `settings.json` by model id (`ModelContextWindowPort`), so it follows edits to the settings and the session snapshot keeps its format.

Checks run on the GX10 (aarch64, GB10): both modules' tests, `go vet` and `gofmt`; unit tests for the endpoint rules, the router, the merged catalog, the window cap, the scaled budget, the settings validation, the local-endpoint trace and provider activity, and startup without an OpenRouter key; the stream shapes of llama.cpp (`b23efaa`, Qwen3.8-27B Q6_K) and vLLM 0.22.1 (Gemma 4 31B QAT W4A16, `gemma4` parser) captured and checked against the stream accumulator; a throwaway test routing a tool request through the settings, the router, the client and the diagnostics transport to the three local servers, each answering with one valid `local_read` call; one console turn in a pseudo-terminal with Gemma 4 that read `README.md` and answered. Not verified: Ollama and LM Studio; a non-loopback server with a key; `axlr-serve`, which still uses OpenRouter only; long sessions near the window, where the 3-bytes-per-token estimate has not been checked against real token usage.

## 6 October 2026: design and drafts for 27B-class local models

Added a dated [design](plans/2026-10-06-local-27b-ceremonies.md) and a [research record](research/2026-10-06-local-27b-agents.md) for ceremonies that suit small local models, and three draft definitions under `tools/ceremonies/drafts/` (`axlr_plan`, `axlr_task`, `axlr_sync`, all 1.0). The drafts are not pinned, not embedded and not started by any mode; the [ceremonies guide](ceremonies.md#drafts-for-27b-class-local-models) says so. Checks run: the four pinned definitions and the three drafts validated and published against the checksummed MADE 0.10.0 release in a disposable store (`check_pins.py`, now also covering the drafts); the drafts' happy and blocked paths walked with `spike_drafts.py`, which CI now runs; relative links of the changed pages checked. Unverified boundaries: no model ran the drafts; the research record marks every number read only from a search snippet, since the research environment's proxy blocked several primary hosts; the measured prefix sizes come from a throwaway test over `HostTools()` and the local tool definitions at this revision and are not part of the test suite.

## v0.2.3 context ceiling and oversized results

Reported from session `e27a6170`: `axlr_tools` could never return `made_design_ceremony` (63 KB schema, 41 KB of it the `x-made-pattern-catalog` annotation), the invocation bridge rejected `x-made-shape`, and `axlr_history` escaped JSON tool results twice, so pages arrived half-size and a 687 KB result needed ~92 reads. Fixes: `x-*` keywords are annotations for validation (#57); `axlr_history` pages message content directly (#59); `axlr_tools` omits `x-*` annotations, outlines a schema that still does not fit and selects parts with a JSON-pointer `path` (#60). At the user's request the projection ceiling rises from 96 KiB to 1 MiB (768 KiB low watermark, 64 KiB per tool result, 16 KiB checkpoint), host results to 64 KiB and history pages to 32 KiB (#58). Limits remain fixed bytes; glm-5.3-flash's window is 1,048,576 tokens. The [console guide](console.md) is updated and the chart's `version` and `appVersion` are `0.2.3`.

## v0.2.2 queued messages no longer cancel the model

A message sent while the model was reasoning cancelled the request and restarted it. With a slow reasoning model (glm-5.3-flash spent 25–120 s per request in the reported session) every new message threw that work away, so the session never answered. The console now queues the message, shows it in the transcript and footer, and delivers it at the next model boundary. The [console guide](console.md) is updated and the chart's `version` and `appVersion` are `0.2.2`.

## v0.2.1 host HOME for console exec

Console exec/check children now receive the host's `HOME` when it is absolute, alongside the sanitized `PATH`; previously tools such as `git`, `cargo` or `gh` ran without a home and missed their configuration. Other variables and provider credentials are still not inherited. The [console guide](console.md) is updated and the chart's `version` and `appVersion` are `0.2.1`. Regression tests cover forwarding and the rejection of a relative `HOME`.

## v0.2.0 session context release preparation

Reviewed the session-context implementation at `0743ba6fa923e12fde608d97189ef40ffcdc1eb4`, based on released `main` at `e37ede7888a1389a95ca0194ea5ece028a872914`. The console now embeds `axlr:axlr-session` and exposes `axlr_session` for a missing title after the second user prompt and an exact KMP scope. Manual titles are preserved; stale picker edits retain a later memory binding. The startup skill requests one relevant inter-about comparison, distinguishes proposals from declarations and requires source-backed identity evidence. The MADE skill separates its seven-definition 1.0 catalogue from the pinned host definitions: debug/delivery 2.0 and incident 1.0. The driver uses the selected project scope for terminal observations, adds stable write identity, retains reference-bearing recall and discloses partial recovery, missing known scopes and review-pending writes. The chart's `version` and `appVersion` are `0.2.0`. Release preparation also integrates `main` at `049f506` (PR #49): incident labels and approved-postmortem digest evidence are preserved with the selected session scope; console wiring retains the reviewer, files and separate approver alongside session labels. The skill and current guides describe all three driver pins.

Local Linux ARM64 checks passed: complete race/coverage suites in both modules (root 85.8%, TUI 80.9%), both modules' `go vet`, release preflight unit tests, Helm lint/template, OpenAPI validation with `openapi-spec-validator` 0.7.2, brand renderer/export checks, both skill validators and the seven MADE 0.9.1 catalogue/behavior checks in disposable stores. All four locked KMP/MADE Linux assets matched their SHA-256 values. Earlier integration checks confirmed that KMP 0.24.0 commits and replays the driver's observation shape without duplication. The skills received an independent scenario review, and session metadata, automatic host bookkeeping, concurrency and recovery are covered by regression tests. Title selection and relationship interpretation remain model-guided; no paid model session or live deployment was used. Publication and the six-platform native checks belong to the release workflow.

## v0.1.0 release preparation

Reviewed the composer correction at `58c700177d6c9ad1b29d90515568c74c0ebc9efd`, based on `main` at `e399711dffe730eefa68f0a8b43fb82d52055343`. The textarea retained Bubbles' default dark cursor-line background under light themes. The composer now applies the active palette at startup, during theme preview/cancellation and after terminal background detection. The [console guide](console.md#appearance-and-language) describes the resulting behavior. The chart's `version` and `appVersion` already match the intended first release, `0.1.0`.

Local Linux ARM64 validation passed: both complete Go suites with race detection, both modules' `go vet`, the composer render regression tests, the TUI executable build, release preflight unit tests, OpenAPI validation with `openapi-spec-validator` 0.7.2, and Helm lint/template with `values.example.yaml`. All four KMP/MADE Linux assets were downloaded from their locked release URLs and matched their SHA-256 values. The development preflight accepted the corrected revision. This records preparation; publication and the six-platform native checks are performed by the release workflow. No live model request or KMP/MADE deployment was performed for this change.

## Baseline and scope

Audited `main` and freshly fetched `origin/main` at `559a1b8ceaeaf9cd091c05b2f9db0236083c271b`, starting with a clean worktree. This pass covers README, current guides, engine runbooks, the public OpenAPI description, packaging/deployment claims and the separation of historical designs from operating instructions. Corrections change documentation and the OpenAPI description; runtime behavior and published ceremony definitions are unchanged.

The product direction is explicit: AXLR is Underpass's default agentic execution engine, supported by KMP memory and MADE procedures, with an open extension route through supported MCP tools and OpenAI/Codex plugin components. “Default” names its product responsibility, not automatic approval, mandatory ceremonies or feature parity across hosts.

KMP recovery was attempted before repository analysis. The connected store did not contain the AXLR project scope, and its guide was unavailable; diagnostics identified an unrelated saved store selection. No AXLR memory was written there and no global store selection was changed. Findings below are derived from the checked-out source and checks, not attributed to unavailable memory.

## Findings and corrections

| Finding in the previous current guides | Source evidence in the audited revision | Correction |
|:--|:--|:--|
| Product intent did not clearly define the default execution entry point or distinguish host capabilities | [Console wiring](../tui/cmd/axlr-tui/run.go), [service wiring](../tui/cmd/axlr-serve/run.go), [worker](../cmd/axlr/worker.go) | Added [product contract](product.md), capability matrix and [daily workflow](runbooks/agent-workflow.md) |
| Index claimed three entry points while service code existed | `tui/cmd/axlr-serve`, `tui/service` | Rebuilt [index](index.md) around use, operations and four integration paths |
| Console omitted six work modes and understated restrictions under autonomy | [Work-mode policy](../tui/domain/work_mode_policy.go), [approval policy](../tui/application/automatic_tool_policy.go), [mode tests](../tui/application/work_mode_test.go) | Documented modes, document-only writes, per-exec approval and stdin refusal |
| Ceremonies were described as automatic for substantive work | [Host guidance](../tui/application/model_context.go), [embedded skill](../tui/adapters/axlrplugin/builtin/made/skills/axlr-ceremonies/SKILL.md) | Ordinary work stays direct; modes or explicit requests start procedures |
| Seven 1.0 skill definitions obscured the two host-driven 2.0 definitions | [Driver](../tui/application/ceremony_driver.go), [pins](../tui/adapters/ceremonyhost/definitions.go) | Split [ceremony guide](ceremonies.md) by execution path, version and acceptance mechanism |
| MADE preparation omitted publishing 2.0 and its temporary grant | [Preparer](../tui/adapters/madesetup/preparer.go), [preparer tests](../tui/adapters/madesetup/preparer_test.go) | Rewrote [MADE runbook](runbooks/made.md): launcher vs binary/remote, permanent work grant, temporary install grant, digest conflicts and readback |
| The direct-binary MADE example used the operator identity for routine work | [Permanent work actions](../tui/adapters/madesetup/work_grant.go) and preparer route checks | Separate operator bootstrap/publication from the configured work principal |
| Automatic memory scope and failures were not explained | [Memory adapter](../tui/adapters/ceremonyhost/memory.go), `CeremonyDriver.Begin` / `record` | Documented session scope `ws:<session-id>`, best-effort behavior and explicit stable project-memory workflow |
| The context guide said oversized active turns immediately failed and listed only three host controls | [Projector](../tui/application/model_context_projector.go), [budget tests](../tui/application/turn_budget_test.go), [host controls](../tui/application/host_tool_definitions.go) | Documented progressive result compaction, current-turn history refusal, skills and conditional step completion |
| Plugin guide said new approval choices were stored in legacy `approvals.json` | [Console configuration](../tui/cmd/axlr-tui/run.go), [settings](../tui/adapters/storage/user_settings.go) | Corrected to `settings.json` with the legacy fallback and custom MCP-path distinction |
| Compatibility lacked exact package-ID and host-feature boundaries | [Package importer](../tui/adapters/axlrplugin/manifest.go), [catalogue](../tui/adapters/axlrplugin/catalog.go), [MCP client](../mcpclient/client.go) | Component matrix, package-generated IDs, environment expansion, unsupported root-only manifests/auth/UI, [extension qualification](runbooks/extensions.md) |
| Service reuse could be read as console feature parity | [Service composition](../tui/cmd/axlr-serve/run.go), [strict config](../tui/service/config.go) | Explicitly documented KMP/MADE-only configuration and absent package, mode and ceremony-driver wiring |
| Service guide lacked a standalone configuration and probe limitations | [Config](../tui/service/config.go), [probe flags](../tui/cmd/axlr-serve/run.go), [readiness](../tui/service/remote_engines.go) | Added full placeholder config, fixed port 8081 for native probes and precise readiness meaning |
| OpenAPI advertised a 410 event response that the handler never returns | [Event handler](../tui/service/http_sessions.go), [event journal](../tui/service/persistence.go) | Corrected to 400 for invalid/ahead cursors and documented query precedence; no invented retention recovery |
| Recovery/update/backup lacked end-to-end procedures | [Updater](../tui/adapters/engineupdate/updater.go), [session storage](../tui/adapters/storage/session_store.go), [service recovery](../tui/service/recovery.go) | Added [maintenance/recovery](runbooks/recovery.md) and [service operations](runbooks/service.md) |
| Linux-only wording and source-only assertions were stronger than repository evidence | [Six-platform release workflow](../.github/workflows/release.yml), native OS adapters | Documented release targets and POSIX example syntax without claiming a successful published release |
| Historical designs could be mistaken for current instructions | Plans still contain earlier loop, ceremony, API and platform assumptions | Added historical status pointers and [documentation maintenance](runbooks/documentation.md) |

OpenAI's [packaging guide](https://developers.openai.com/plugins/build/plugins) was checked for the distinction between root plugin manifests and the Codex compatibility manifest. AXLR compatibility claims are bounded by its importer, not by another host's advertised capabilities. Upstream KMP/MADE setup links were checked; engine upgrades still require release-matched instructions.

## Validation

All checks below passed on the local Linux ARM64 host:

| Check | Result |
|:--|:--|
| `GOWORK=off go test ./...` and `GOWORK=off go -C tui test ./...` | Both complete suites passed; the available MADE 0.9.1 binary also allowed the 2.0 definition-pin test to run |
| `go vet` with `GOWORK=off` in both modules | Passed |
| Build `axlr`, `axlr-tui`, `axlr-serve`; inspect CLI help and versions | All three built; documented flags checked |
| Worker read of README | `completed`, with bounded output and no workspace mutation |
| Complete Go examples in library and MCP client guides | Both compiled |
| Markdown links, local assets/fragments and JSON fences | 53 pages, 287 local references and 13 complete current JSON examples passed |
| `helm lint` and `helm template` with `values.example.yaml` | Passed; rendered service JSON and the API guide's JSON both accepted by `service.LoadConfig` |
| OpenAPI 3.1 validation with `openapi-spec-validator` 0.7.2 | Passed after correcting SSE cursor documentation |
| `tools/ceremonies/check.py` with MADE 0.9.1 | All seven 1.0 definitions publishable; synthetic success, failed checks, loop bounds, receiver acceptance and human-publication boundary passed in disposable stores |
| `git diff --check` | Passed |

No paid model call, live deployment, external publication or write to an existing KMP/MADE store was performed. The scratch builds and validators were removed after verification. External documentation checks covered the linked OpenAI packaging and KMP/MADE setup references; this was not a crawl of every historical external URL.

## Implementation limits made explicit

These are current product limits, not documentation features left to implement in this change:

- The service has no generic external-plugin setting, skill installation, mode endpoint or wired console ceremony driver.
- The package importer supports the Codex compatibility manifest and skills/MCP declarations, not every OpenAI plugin component or authentication flow.
- Ceremony memory is best effort and session-scoped; stable project scope, complete evidence recovery and decision relationships require explicit KMP calls.
- Console mode restrictions do not sandbox approved processes or classify remote MCP side effects. Driver/setup bookkeeping relies on engine grants as well as the selected workflow.
- Tests and source manifests do not prove a live deployment, paid model behavior, native execution on every platform or published release availability.

## Earlier audits

The entries below preserve the facts and corrections reported at the time. In particular, service/release work described as planned in September has since been implemented in source; use the current findings above for present behavior.

### 30 September 2026

This audit began against AXLR at `8faf100` and was reconciled with `08f617a`, which replaced the Codex-owned `/plugin` flow with AXLR-owned package installation. KMP and MADE provided structural references: a clear promise, a quick start, a route table and separate guides for detailed contracts. AXLR's current claims were checked against its own code.

| Finding | Evidence in the prior docs | Resolution |
|:--|:--|:--|
| The first task was hard to find | The root README opened with the JSON worker; the interactive console appeared later | The README now starts with the console and a tested launch path |
| No documentation map | Current guides, historical plans, research and diagnostics shared `docs/` without an index | [Documentation home](index.md) separates current guides from dated records |
| One TUI page mixed several jobs | `tui/README.md` contained controls, models, MCP setup, sessions, diagnostics and developer gates | Separate console, plugin, troubleshooting and architecture pages; the module README points to them |
| MCP transport wording had aged | The root README described external plugins as stdio only | The plugin guide documents stdio and Streamable HTTP from `plugins/manifest.go` |
| Package installation and server connection were easy to conflate | `/plugin` changed ownership after the first audit: it now installs into AXLR and can register MCP servers | The plugin guide describes AXLR-owned packages, generated manual MCP registration and `/mcp` as the connection view |
| Codex plugin compatibility was undersold | The guide mentioned a compatible manifest without stating the intended reuse path | The plugin guide now maps Codex package components to AXLR behavior and gives a concrete install-and-verify route |
| Product responsibility was underspecified | Generic "local execution runtime" wording hid which layer owns memory and orchestration | The README and architecture identify AXLR execution, KMP memory and MADE orchestration without claiming their engines are automatically connected |
| Engine setup/removal had no safe path | A built-in `/plugin` row could be mistaken for an active engine | Separate [KMP](runbooks/kmp.md) and [MADE](runbooks/made.md) runbooks describe connection, verification and disconnection while preserving stores |
| HTTP service and cross-platform releases were aspirations, not current features | The worker is a one-request stdin/stdout process; CI runs on Linux only | The [service spec](specs/axlr-service-api.md) and [implementation plan](plans/axlr-service-helm-release.md) mark planned API, Helm and release work explicitly |
| Operational behavior was buried | Session recovery, private payload captures and execution authority were below long feature descriptions | The quick start and console guide put those facts beside the action they affect |
| No shared visual entry point | AXLR had no logo in the repository | Added the project-supplied pixel-art wordmark, documented in [Brand](brand.md) |

Historical design notes remain available as records of decisions. Current behavior should be checked in the [current guides](index.md) and, for exact schemas and limits, in the running binary and source.

### 1 October 2026

| Finding | Evidence | Correction |
|:--|:--|:--|
| Logos had no reproducible shared source | AXLR was a flattened raster; KMP and MADE had separate SVG exports | Added the [pixel renderer](../tools/pixelart/README.md), editable terminal-art definitions, six transparent exports and a CI drift check. Before recoloring, MADE's generated lettering matched the current upstream `made-cuatro-voces.png` pixel for pixel at 2× scale. The later palette change preserves that geometry. |
| Product palettes changed after the shared exports | The user supplied four distinct copper, gold, brown and sand colors per product | Applied the exact [product palettes](assets/brand/README.md) to lettering and bars, added per-logo bar color configuration and regenerated the documentation, website and GitHub profile assets. |
| The bars should form a clean square | The user requested vertical stacking and rejected the bars' corner cuts | Added the reusable square layout: four horizontal bars with equal widths, straight ends and uniform transparent gaps, aligned below each wordmark's right edge. Updated export dimensions and website image metadata; lettering and palettes are unchanged. |
| One-launch KMP setup could select another store | The example passed a manifest without its configured `KMP_MCP_DATA_DIR` | The runbook now forwards an explicit absolute store for that launch and pins embedded mode in persistent configuration. |
| The quick start understated saved approvals | `tui/application/automatic_tool_approval_test.go` covers exact-tool grants and full autonomy | The first-run guide now names those saved policies and `/approvals`. |
| Symlink wording overstated the read restriction | `FileAdapter.Read` opens through `os.Root`; `Load` separately rejects symlinks for editing | The worker guide distinguishes an in-root read target from an escape and a mutable regular file. |
| Loopback probes were not connected to a Kubernetes probe mechanism | A kubelet `httpGet` targets the pod IP, not container loopback | The plan specifies the service's native `probe` subcommand and exec probes; readiness cannot generate paid completions or mutate engine state. |
| Six-platform releases lacked a prerequisite portability step | Unix process groups, `Flock`, `O_NOFOLLOW` and shell fixtures appear in untagged code | The plan adds native process, private-file and lock adapters before enabling Windows and the release matrix. |
| Some proposed HTTP states and preconditions were ambiguous | `uncertain` is not a current worker status; conflicting revisions/cursors had no rule | The plan separates service states and fixes revision, cursor and error-code conventions. |
| MADE's public status lagged its release | GitHub Release and Cargo both publish `made-mcp` 0.9.1 | The runbook and public surfaces now identify 0.9.1 while linking the upstream setup contract. |
