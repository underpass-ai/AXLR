# AXLR master plan: from console to the runtime of Underpass Bots

Status: proposed (23 Oct 2026). It follows the phases of the vision's roadmap (§13). The product vision this plan serves is Underpass Bots (vision and architecture note of 21 Oct 2026, kept outside this repository; § numbers below refer to it): persistent, governed bots that run as infrastructure, with a control plane that decides *when, where, as whom and under which permissions* a unit of work runs, and AXLR deciding *how* an agent reasons and uses tools (vision §6.1, §7.1). This plan is the bridge: what AXLR must grow, and in what order, to become that runtime. It does not restate the vision.

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

## AXLR's part in each phase of the vision roadmap (§13)

The phases are the vision's own. For each, what the control plane builds is out of scope here; the columns say what AXLR already has and what it must add.

### Phase 0 — Contracts and technical proof

The vision's exit: show that the persistent bot and the AXLR execution are separate entities.

| AXLR has | AXLR adds |
|---|---|
| `axlr-serve` sessions, turns, events and approvals over HTTP, gRPC and MCP; `axlr-tui --exec` with JSON events; the Grok Bot experiment of 21–23 Oct as an informal proof | The runtime contract written down: one execution = one session; inputs (prompt, model, policy, budget, workspace) and a result schema (status, answer, artifacts, usage per request); `--exec` resuming a session with a pending call and deciding it; release v0.14.0 |

Exit for AXLR: a stub control plane, outside this repository, runs one task through the contract and reads the result without reading traces.

### Phase 1 — MVP on one server (Server Incident Bot)

| AXLR has | AXLR adds |
|---|---|
| MCP tools with manual approval; read-only work modes (`review`, `research`); logs, diagnostics and the usage ledger; the service recovers its sessions after a restart | Static allow/deny per execution (tools and argument patterns, deny by default) with the §10.4 levels as presets; artifacts (`axlr_deliver`, `artifact` events); per-execution metrics in the result; a Windows default PATH |

Exit for AXLR: the SRE bot's two observability MCP servers are the only tools an execution can reach, and a forbidden call is refused outside the model.

### Phase 2 — Durability and governed autonomy

| AXLR has | AXLR adds |
|---|---|
| Idempotency keys and revision guards on the service; durable approval decisions (`approver` role); cancellation; KMP memory and MADE records per session; Jev | Approvals that reach a person (webhook) and expire; deadlines and budgets per execution (seconds, requests, tool calls, USD) enforced outside the model; an execution receipt (model, tools with hashes, usage, cost); correlation ids (bot, task, execution, trace) through events, traces and MADE |

Exit for AXLR: a long execution survives a restart of `axlr-serve`, and its receipt shows every action and what it cost.

### Phase 3 — Kubernetes and collaboration

| AXLR has | AXLR adds |
|---|---|
| A Helm chart; the bwrap exec sandbox; the signed catalog and registry of forged tools | Sandbox profiles per execution (process, rootless container) with egress control; delegation limits (depth and budget) when a bot asks another; skills versioned in the catalog |

### Phase 4 — Product and differentiation

| AXLR has | AXLR adds |
|---|---|
| The console as an operator client; MADE ceremonies; Jev | Bot templates as catalog entries; skills with test harnesses (the registry's tests); cost and strategy comparison with Jev; MADE for multi-step workflows a bot starts |

### Parallel track — Model layer

Native adapters for the Anthropic Messages and OpenAI Responses APIs, so reasoning, prompt caching and routing stop depending on OpenRouter's translation (the reasoning bug fixed on 23 Oct belongs to this class; Pi already speaks each API natively), and a model router by cost and latency fed by the usage ledger (vision: Model Router).

### Non-goals for AXLR

The bot registry, triggers, schedulers, durable task queues, NATS, Kubernetes resources and federation: they are the control plane's (vision §6.1, risk "coupling AXLR to the scheduler or Kubernetes" in §15.1).

## Metrics (vision §16)

Tracked from Phase 1, against the baselines above: cost per resolved task, time to first bot, recovery rate after induced failures, share of forbidden actions refused by policy, and requests per task.

## Open questions

1. Are Phases 0 and 1 the scope of the NLnet proposal due 3 Nov 2026?
2. Does the control plane start in Go next to AXLR, or in another stack (vision ADR 15.2 lists persistence and orchestration as open)?
