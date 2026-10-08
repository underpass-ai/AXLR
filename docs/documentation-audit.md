# Documentation audit — 4 October 2026

## 8 October 2026: a prompt budget for models whose window is unknown

Reported from session `c33e8e86` (Claude Haiku 5.5 through OpenRouter): 198 requests, 30.3M prompt tokens and $14.13, with the projection never dropping a turn. #8 had bounded the history at 96 KiB; #58 raised it to 1 MiB at the user's request, and #68 derived a smaller budget only from a local model's window, so a remote model, whose window the console does not know, kept the 1 MiB ceiling: 315K tokens by the last request, 133 requests above the 100K tokens where Haiku 5.5 costs five times as much. The domain gains `ContextBudgetForPrompt`: a budget derived from the prompt a request may reach (`prompt_tokens`, default 64,000, at the 2.44 bytes per token measured in that session) rather than from a window, with the low watermark at half the ceiling so a cut is followed by many appends. `localmodels.Windows` answers it for a remote model and the window budget for a local one; an unknown window never yields the ceiling. `axlr_history` pages shrink to the model's tool result budget, so a recovered page is not excerpted again; a clipped result keeps one `retrieval` text whether its turn is open or closed, since the earlier rewrite broke the provider's cached prefix at that message on every turn. Replaying the session through the real projector with the default prompt budget gives 62K tokens at most, no request above 100K, 11 cuts and $0.90, against $14.08 for the replay of the 1 MiB ceiling ($14.13 measured); the [research note](research/2026-10-08-bounded-remote-context.md) has the method and the other candidates. The [console guide](console.md#model-context) describes both budgets and the setting.

## v0.4.1 release preparation

Releases `main` at `9069309`. Since v0.4.0: the system prompt stays identical across user turns, so provider prompt caches keep hitting (#80); installed skills are listed by name under their package's description instead of one full entry each, including skills whose `SKILL.md` starts with a multibyte character (#81); when the trace sits inside the workspace, payload captures go to the private state `logs/` directory so the model cannot read its own earlier requests (#82). The chart's `version` and `appVersion` are `0.4.1`. Publication and the six-platform native checks belong to the release workflow.

## v0.4.0 release preparation

Releases `main` at `3078d13`. Since v0.3.0: transcript text is copied by dragging, through OSC 52 with the host clipboard tools as fallback, and `/copy` copies the last reply as the model wrote it (#77); a message queued while the model works joins the running turn after its tool step instead of cancelling it, and the queue is visible in traces (#78). The chart's `version` and `appVersion` are `0.4.0`. Publication and the six-platform native checks belong to the release workflow.

## 8 October 2026: queued messages join the running turn

Reported from session `43e5a435`: a message queued while the model worked cancelled the operation at the next model request, after that request had been sent, and started a new turn. The console showed `context canceled` (sometimes twice, from a cancellation joined with the emit that reported it) through the whole next turn, and 8 of the session's 12 prompts were never answered because each queued one cut the turn of the previous one. The turn now takes the queued message after a tool step, before building the next request, and the model reads it with a note asking it to answer both; ceremony steps keep the old stop, without the error. The [console guide](console.md) and [troubleshooting](troubleshooting.md#inspect-the-right-diagnostics) describe the behavior and the new `prompt_queued`, `steer_applied` and `steer_cancelled` trace stages.

## v0.3.0 release preparation

Releases `main` at `5962f0d`. Since v0.2.3: `/repair` drives a repository fix in a clone through to a merged pull request (#61) and the agent can request it from a running session (#64); the axlr-ceremonies 1.0 skill catalogue is retired and CI validates the driver definitions and their pins (#62); the ceremony recall focuses on the user's request (#63); local OpenAI-compatible models are served from `settings.json` with a configurable context window (#68); debug and delivery run under a compact profile for small local models (#71); plan briefs become verified atomic tasks run by small-model workers with per-wave syncs (#72); a ceremony whose model stops handing the step back ends, and the person can stop one (#73). The chart's `version` and `appVersion` are `0.3.0`. Publication and the six-platform native checks belong to the release workflow.

## 7 October 2026: stalled ceremonies (#70)

A driven ceremony no longer waits forever when its model stops handing the step back.

- **The reminder.** The console's one reminder now names a tool call that leaked as text (`<|tool_call>`, `<tool_call>`, `[TOOL_CALLS]`, `<|python_tag|>` and similar).
- **A second plain reply** cancels the MADE instance (`made_cancel_ceremony`, already in the work grant) with the reason and records the outcome. The session returns to normal mode, and one visible message names the server's tool-call parser as the first suspect when markup was present. A plan worker ends `BLOCKED` with that reason. Self-repair keeps its own nudges.
- **A restored session** with a step open and no turn shows it in the footer; `/stop-ceremony` (alias `/parar`) cancels it.
- **Trace.** The trace gains a `ceremony_stalled` stage.

At Tirso's request there is no time-based limit: a slow model keeps its time, and a stopped one is detected by its behaviour. The [ceremonies guide](ceremonies.md#a-step-the-model-will-not-hand-back), the [console guide](console.md) and [troubleshooting](troubleshooting.md) are updated.

**Checks run**
- Both modules' tests, `go vet` and `gofmt`.
- Unit tests:
  - two replies with leaked markup end the ceremony, the reminder names the markup, and the notice names the parser;
  - the reminder carries no leak note without markup;
  - a paused open step is reported and stopped by the person;
  - a stalled self-repair is left to its runner.

**Not verified**
- A live model reaching the cancellation. The plan runs that motivated this ended earlier through the step budget or the worker rule.
- The footer's rendering in a terminal.

## 7 October 2026: plan, task and sync

`/plan` (alias `/planificar`) drives three newly pinned definitions: `axlr_plan`, `axlr_task` and `axlr_sync` 1.0. `/mcp → P` now publishes seven definitions.

**The plan**
- The planner (`plan.model`, default `z-ai/glm-5.3-flash`) decomposes the brief.
- The console verifies it mechanically:
  - ids, dependencies and waves;
  - scopes disjoint within each wave;
  - citations;
  - commands, which the person approves first and the console runs once as a baseline;
  - 12 KiB context packs.
- The person approves it on the plan card: `a` approves, `d` sends it back, `x` declines; `plan.auto_approve` records an automatic approval instead.

**Tasks and syncs**
- An approved plan runs in the background, wave by wave. Each task gets a fresh `task` session with the compact profile and autonomous local tools.
- Each task runs `start`, then `red` (frozen test digests) when test-first, then `green` (protected digests and the git-status scope), then the console's `handback`. Notes are relayed to later tasks and recorded in KMP with plan, task and wave labels.
- A finished wave is integrated by `axlr_sync` with up to two reconciliation rounds.
- A blocked task's scope files are restored, and its dependents are skipped.
- The registry is `plans.json`. The `/plan` panel and the footer badge show progress.

**Decisions and deviations**
- The eight decisions Tirso took by survey are recorded in the plan.
- `/plan` takes the brief as the next prompt rather than `/plan <brief>`.
- The worker's KMP read is a focused wake, because the adapter has no read by label.
- Reconcile workers end with a free-text summary and `NOTE` lines rather than `axlr_step_done`.

**Docs updated:** [ceremonies](ceremonies.md#plans-atomic-tasks-for-small-models), [console](console.md), [troubleshooting](troubleshooting.md#plans), [architecture](architecture.md), the [MADE runbook](runbooks/made.md), the drafts README and the [plan](plans/2026-10-06-local-27b-ceremonies.md#plan-task-and-sync-measured-7-oct-2026).

**Checks run**
- Both modules' tests (`-race` for the runner), `go vet` and `gofmt`.
- `check_pins.py` and `spike_drafts.py` against MADE 0.10.0 on aarch64, where all seven pins match.
- Unit tests:
  - plan verification, return, decline and automatic approval, and the planner model;
  - red and green with frozen and protected digests and an out-of-scope refusal, and the hand-back;
  - sync rounds;
  - the runner's wave order, skipping and scope restore;
  - registry and sidecar persistence.
- Six live end-to-end plans, with numbers in the plan:
  - Each Gemma 4 run exposed one worker failure that the console now absorbs:
    - a blocked task's half-done edit (the scope is now restored);
    - malformed `local_exec` calls (the arguments are now repaired);
    - code changed during `red` (`red` now writes tests only).
  - The sixth run used Qwen3.8-27B on llama.cpp with `thinking: false`. It finished 3 of 3 tasks in 13.5 min and passed the hidden acceptance test.

**Not verified**
- The frontier-worker arm.
- A sync that actually needed reconciliation with a live model; the rounds are covered by tests.
- Resuming an interrupted plan with `r`.

## 7 October 2026: the compact ceremony profile

`/debug` and `/delivery` gain a compact profile for small models, selected by `ceremonies.profile`:
- `auto`, the default, applies it when the session model's known window is 64K tokens or less;
- `standard` and `compact` force a profile.

Under it, a step has 16 calls and a context of at most 80/56/8/4 KiB. It sees only its own `axlr_step_done` fields, with a schema of its own and an instruction under 500 bytes that leads with an example. The local tools stay, but without write and edit in read-only steps; of the host tools only `axlr_history` remains. Tolerant decoding names ignored fields and splits a string command without shell syntax; such a command still goes through the approval card. The same call twice in a row is refused. Each step starts from a ledger of the accepted steps, while `message_index` references stay absolute.

`local_models[].stream: false` asks a server for whole replies. The [ceremonies guide](ceremonies.md#the-compact-profile-for-small-models), the [console guide](console.md#local-models), [troubleshooting](troubleshooting.md#local-models) and a dated measurement block in the [27B plan](plans/2026-10-06-local-27b-ceremonies.md#compact-profile-measured-7-oct-2026) are updated. Issue [#70](https://github.com/underpass-ai/AXLR/issues/70) records the stalled-ceremony case the measurement exposed.

**Checks run**
- Both modules' tests, `go vet` and `gofmt`.
- Unit tests:
  - every compact instruction is under 500 bytes and its example decodes cleanly;
  - every per-step field exists in the full schema;
  - the tool surface per step, and refusals of hidden tools and repeated calls;
  - tolerant decoding, including the shell refusal, and the approval of a recovered command;
  - the ledger and the projection's origin table;
  - the compact budget, persistence of the new run fields in the `.ceremony` sidecar, and the profile setting and its auto rule;
  - the router asking for whole replies.
- Live: nine console runs with Gemma 4 31B on vLLM against disposable MADE and KMP stores, all accepted by a hidden test. The numbers are in the plan.

**Not verified**
- Qwen3.8-27B and `/debug` under the compact profile.
- A step long enough for the ledger cut to matter.
- The "second reply without a tool call" rule, which stays for the task worker and #70.

## 7 October 2026: delegating judgements to TypeSafe Jev

A `jev` section in `settings.json`, off by default, lets the model lean on TypeSafe Jev, the judgement model KMP already uses: `tool` offers `axlr_judge` (one yes/no or choice question about a state the model writes), and `final_check` asks Jev once per request whether the final answer completes it, returning a doubted answer to the model in a visible `[AXLR · Jev]` message. The tool is added to the host tools only when enabled, so the default request is unchanged. The adapter mirrors KMP's client: the pinned `jev-1.13.0`, a 256 KiB body, no redirects, at most two retries on 429. The [console guide](console.md#jev-an-external-judge-off-by-default), [troubleshooting](troubleshooting.md#jev) and [architecture](architecture.md) are updated.

Checks run: both modules' tests, `go vet`, `gofmt`; unit tests for the request body, the answer validation, retries and key handling against a local fixture; the tool offered only when enabled and hidden again when disabled; the final check returning a doubted answer exactly once, and letting confident or failed judgements pass; startup refusing an enabled Jev without `TYPESAFE_API_KEY`. Live: two judgements against TypeSafe (about 300 ms each; a plan-only answer scored 0.08, the right fix chosen with confidence 1.0); one console turn with Gemma 4 31B on vLLM in which the model called `axlr_judge` and the final check passed in 230 ms. That turn showed the model asking an "A or B" question without `options`, which yields only a yes probability; the tool description now requires options for such questions. Not verified: whether Jev improves task outcomes for a 27B model, which needs a measured comparison with the switches on and off.

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
