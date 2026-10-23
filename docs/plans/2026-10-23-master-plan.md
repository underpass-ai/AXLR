# AXLR master plan: from console to the runtime of Underpass Bots

Status: proposed (23 Oct 2026). The windows below are proposals. The product vision this plan serves is Underpass Bots (vision and architecture note of 21 Oct 2026, kept outside this repository; § numbers below refer to it): persistent, governed bots that run as infrastructure, with a control plane that decides *when, where, as whom and under which permissions* a unit of work runs, and AXLR deciding *how* an agent reasons and uses tools (vision §6.1, §7.1). This plan is the bridge: what AXLR must grow, and in what order, to become that runtime. It does not restate the vision.

## Boundary

AXLR is the execution plane. Bot registry, identities, triggers, scheduling, durable task queues and federation belong to the control plane, which lives in its own repository and calls AXLR through a runtime port. Nothing in this plan moves them into AXLR.

## Where AXLR stands (main at f979c6d)

| Surface | State |
|---|---|
| Console (`axlr-tui`) | Model and tool loop, work modes, approvals with `why`, autonomy and the destructive list, forged tools, signed catalog and registry, KMP memory and MADE session lifecycle, Jev, 75K context cap with successor sessions, live reasoning row |
| One command (`axlr-tui --exec`) | Same loop without a screen; `--json` events, `--approve none\|local\|all`, `--mode`, exit 0 / 3 (needs a decision) / 1 |
| Service (`axlr-serve`) | Same loop over HTTP, gRPC and MCP with mTLS and principals; 19 verbs including `CloseSession`; `settings_file`; owned successor sessions |
| Models | OpenRouter plus OpenAI-compatible servers (local or remote over HTTPS); reasoning kept across tool calls (current turn) and counted at its size |

Baselines measured on 21–23 Oct 2026 (claude-haiku-5.5, the AXLR repository and an animated page as the task), to compare every later change against:

| Measure | Value |
|---|---|
| Same prompt before / after keeping reasoning | 17 → 5–9 requests, 598 s → 158–167 s, $0.076 → $0.026 |
| Full demo from Grok Bot's cloud computer | 13 requests, $0.028 |
| Provider routing (`sort: throughput`) | 56 s → 5 s → 3.1 s on a one-read task |
| A self-check instruction without limits | 20 of 29 requests spent probing headless Chrome |

## What the runtime contract still lacks (vision §7.1)

| Requirement | Today |
|---|---|
| Context bounded in tokens | Yes (cap and successor) |
| Tool catalog filtered by policy, per run | No: work modes and `--approve` only; no allowlist of tools and arguments |
| Model chosen by a router | Partly: per-session model and per-model routing options |
| Time, token, call and cost limits per run | No: per session or from settings |
| Structured events and result | Events yes; no result schema, no artifacts |
| Interruption, cancellation, checkpoints | Yes |

The autonomy levels of vision §10.4 (Observe, Propose, Act with approval, Act within policy) map onto `--approve` and the work modes, but without a per-run allowlist there is no way to say "this run may only read logs and query metrics".

## Phases

### Phase 1 — Runtime for bots (now → 3 Nov 2026)

Goal: a control plane, or a bot like Grok Bot, can run AXLR unattended and get a bounded, auditable result.

- Release v0.14.0 with #168 and #170 and install it.
- `--exec` resumes a session with a pending call and decides it (`--session ID --approve local` runs it if the policy allows, `--deny "reason"` refuses it). Measured gap: on 23 Oct the bot had to open the console to approve one command.
- Per-run policy: `--allow-tools` (tool and argument patterns), deny by default for everything else; levels `observe`, `propose`, `act` as presets.
- Per-run budgets: `--max-seconds`, `--max-requests`, `--max-tool-calls`, `--max-usd`, enforced outside the model.
- A result schema for `--exec --json` and the service: final status, answer, artifacts, usage per request and in total.
- `axlr_deliver`: the model marks a file as a deliverable; it appears as an `artifact` event.
- Windows: a default PATH that exists on Windows (the console falls back to Unix directories).

Exit: Grok Bot runs the demo in one `--exec` call with an allowlist and a budget, and gets the table from the result instead of reading traces.

### Phase 2 — Governed execution (Nov → Dec 2026)

Goal: unattended runs whose permissions do not depend on the prompt (vision §10.1: the model proposes, it never grants).

- Approvals that reach a person outside the console: the service's `approver` role gets a notification channel (webhook first) and approvals expire.
- A deterministic policy engine per tool and arguments, beyond the destructive list; refusals logged with the rule.
- Sandbox profiles per run (process, rootless container) with egress control.
- Execution receipt: model, tools with argument and result hashes, usage and cost, signed; one link per run.
- Correlation ids (bot, task, execution, trace) carried from the request through events, traces and MADE records.
- Follow-up already owed: decouple the old ceremony driver from the engine names (lifecycle design).

Exit: a forbidden tool call is refused by policy in a run the model was told to complete, and the receipt shows it.

### Phase 3 — Underpass Bots MVP (Q1 2027)

Goal: the first real bot of vision §18, a read-only SRE bot on one Linux host.

- New repository for the control plane: bot registry, triggers (alert, schedule, manual), durable tasks with idempotency keys, leases and recovery.
- AXLR behind the runtime port: `axlr-serve` per host, one session per execution, the control plane holding identities and budgets.
- Two read-only observability MCP servers as the bot's only tools.

Exit: the twelve acceptance criteria and ten failure tests of vision §14 pass on one host.

### Parallel track — Model layer

- Native adapters for the Anthropic Messages and OpenAI Responses APIs, so reasoning, prompt caching and routing stop depending on OpenRouter's translation. The reasoning bug fixed on 23 Oct belongs to this class; Pi already speaks each API natively.
- A model router by cost and latency per task kind, fed by the usage ledger.

### Non-goals

A control plane inside AXLR, federation between hosts, Kubernetes resources, and a web UI.

## Metrics (vision §16)

Tracked from Phase 1, against the baselines above: cost per resolved task, time to first bot, recovery rate after induced failures, share of forbidden actions refused by policy, and requests per task.

## Open questions

1. Is Phase 1 the scope of the NLnet proposal due 3 Nov 2026?
2. Is Q1 2027 the right window for the Bots MVP?
3. Does the control plane start in Go next to AXLR, or in the stack the Underpass registry already uses (Rust on Lambda)?
