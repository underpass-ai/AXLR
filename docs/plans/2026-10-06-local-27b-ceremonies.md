# Plan, task and sync: ceremonies for 27B-class local models

Status: implemented (7 Oct 2026). The OpenAI-compatible endpoint, the compact profile and plan, task and sync run in the console; `axlr_plan`, `axlr_task` and `axlr_sync` 1.0 are pinned. The sections below are the design as written on 6 Oct; "Decisions taken" and the dated measurement blocks record what changed and what was measured. The three definitions below are published and walked against MADE 0.10.0 in a disposable store by [`tools/ceremonies/spike_drafts.py`](../../tools/ceremonies/spike_drafts.py); they live under [`tools/ceremonies/drafts/`](../../tools/ceremonies/drafts/README.md) as drafts, not pinned, not embedded and not started by any console mode yet. No model has run them. The companion [research record](../research/2026-10-06-local-27b-agents.md) holds the sourced evidence this design rests on.
Builds on: [ceremony driver](2026-10-01-ceremony-driver-design.md), [incident](2026-10-02-incident-ceremony-design.md) and [self-repair](2026-10-05-self-repair-design.md). The console drives MADE; the model does the work; the console owns every verdict.
Brief (Tirso, 6 Oct 2026): small models with a precise context can take small tasks. Split the work into atomic tasks that are testable, unit and end to end; let each worker say when it has finished; add a periodic ceremony where the workers talk to each other.

## Problem it solves

The 2.0 ceremonies were tuned with `glm-5.3-flash` over OpenRouter: a 1,048,576-token window, fast prefill and reliable tool calls. A 27B-class model served locally changes every one of those assumptions. In October 2026 the class is Qwen3.6-27B and Qwen3.8-27B, Gemma 4 31B and 26B-A4B, Devstral Small 2 24B, GLM-4.7-Flash, Qwen3.6-35B-A3B and gpt-oss-20b; Qwen3-32B and Gemma 3 27B, their 2025 predecessors, carry most of the published failure data and are this design's pessimistic reference (research record, section 1):

| Assumption behind 2.0 | Measured in this checkout | 27B-class local reality (research record, sections 1 to 3) |
|:--|:--|:--|
| Context is abundant | Projection ceiling 1 MiB, low watermark 768 KiB, 64 KiB per tool result (`tui/domain/context_budget.go`) | 24 to 32 GB of VRAM leaves roughly 32K tokens of usable context, and quality falls well before the advertised window |
| Prefix bytes are cheap | Fixed model-facing prefix about 10.7 KB: 1,870 B of guidance, 1,417 B of local tool schemas, 6,718 B of host tool schemas, of which `axlr_step_done` alone is 2,255 B because its schema is the union of every ceremony's fields | Prefill runs at about 1,300 to 3,000 tokens per second on an RTX 3090 to 5090 and about 190 on an M4 Max: an uncached 16K-token prompt costs 7 to 13 s on the GPUs and about 86 s on the Mac, against well under a second with a prefix-cache hit; a changed prefix pays the whole prefill again |
| Output is cheap | `integrate` asks for a full `report` plus `summary_en`; `/incident` asks for a complete postmortem | 40 to 60 tokens per second on those GPUs, 12 to 16 on the Mac; a 400-token report costs 7 to 25 s |
| Tool calls are reliable | `axlr_step_done` is decoded with `DisallowUnknownFields`; `check_command` must be `{program, args}` | Small models send strings for objects, invent fields, forget the hand-back and repeat the same failing call |
| Exploration is affordable | 32 tool calls per step (`domain.MaxTurnToolCalls`) | 32 calls with 64 KiB results cannot fit the window; the turn is compacted in flight |

The 1 Oct 2026 measurement that retired the skill catalogue (55 calls of protocol against 11 of work) is the small-model lesson in miniature: protocol cost is paid by the model, and a 27B pays more. The response is not another skill. It is a shape of work where every model turn has one small deliverable, every verdict is the console's, and every worker starts from a context the console built for it.

## Principle

The console drives the protocol and owns every verdict, as in 2.0. Three more principles for small models:

1. **One deliverable per model turn.** A step asks for two to four fields, shows one exact example of the hand-back and offers only the tools that step needs. The console writes the guard fields.
2. **Fresh, precise context per task.** The console decomposes the plan into tasks whose whole context fits a fixed budget, builds that context itself from verified citations, and starts each worker in a new session with nothing else. "Atomic" has a mechanical meaning: the task's context pack fits 12 KiB, its scope is at most eight files and its unit check runs.
3. **Coordination through records, not chatter.** Workers do not talk to each other directly. Each hand-back carries structured notes addressed to other tasks; the sync ceremony relays them, runs the end-to-end check and, when it fails, gives each affected worker the others' hand-backs in a fresh context. The person approves the plan before any task starts and reads the exchange when a sync blocks.

## What small models need, and what answers it

| Finding (research record) | Design response |
|:--|:--|
| Multi-turn conversations lose about 39% against single-turn, and models that take a wrong turn rarely recover; consolidating the information into one turn recovers most of it (section 5.2) | Each task is a fresh session with a consolidated context pack; each step repeats from the ledger of recorded fields, not from the previous attempt's chatter |
| Agents scoped to 3 to 20 steps and about 3 attempts per tool before escalation avoid error spin-outs (section 5.2); more OpenHands iterations gave Devstral Small nothing beyond 50 (section 5.7) | 16 calls per step, 3 rounds per phase, 2 reconciliation rounds per sync; exhaustion is a recorded `BLOCKED`, not a longer loop |
| Models cannot reliably correct themselves without external feedback; execution feedback does help (section 5.3) | The console runs the unit check, the end-to-end check and the citation check; the model only sees their output |
| Reinforcement-trained coding agents learn to skip tests, edit the harness or special-case tests (section 5.4) | Test files are frozen by digest after the red phase; out-of-scope changes are refused; protected files are checked on every round |
| Fewer, well-described tools raise selection accuracy; tool search lifted an MCP evaluation from 49% to 74% for one frontier model (section 5.6) | Workers see five tools; read-only steps hide `local_write` and `local_edit`; `axlr_step_done` carries only the current step's fields |
| Prompt-cache hit rate is the metric that matters; stable prefixes, append-only context and masking instead of removing tools keep it high (section 5.5) | Guidance and tool schemas are constant for the whole worker session; the step instruction is appended last; nothing in the prefix carries a timestamp or a counter |
| Long prompts erode instruction following in test-driven tasks (section 5.4) | Step instructions stay under 500 bytes and lead with the example hand-back |
| Qwen3.6-27B and Gemma 4 31B call a world-changing tool on 98% of complete requests but hold back on only 56 to 59% of under-specified ones (section 2) | A worker never receives an under-specified task: the plan is verified mechanically and approved by the person first, and the worker's questions are recorded, not asked |
| A tool parser that does not match the model's template produces identical-call loops and "no tool was used" errors; Ollama truncates the prompt silently from the beginning (sections 2 and 3) | Serving prerequisites are part of the design, and the console refuses to start a worker whose first reply carries no tool call twice in a row |

## The set

| Mode | Definition | Who starts it | Sequence |
|:--|:--|:--|:--|
| `/plan` | `axlr_plan` 1.0 | the person | decompose (at most 3 rounds, console-verified) → the person's approval (at most 2 returns) → `READY` |
| worker | `axlr_task` 1.0 | the console, one per task | start (console) → red when the task is test-first (at most 3) → green (at most 3) → handback (console) → `DONE` |
| sync | `axlr_sync` 1.0 | the console, after each wave | integrate (console runs the end-to-end check) → reconcile (workers in fresh contexts, at most 2 rounds) → `SYNCED` |
| `/debug`, `/delivery` | 2.0, unchanged | the person | as today, under the compact profile below |

`/incident` stays outside the set: a 64 KiB draft, a fresh-context reviewer and a long structured hand-back are what a 27B does worst. `/repair` runs, since its model steps are debug's, but repairing AXLR itself with a 27B is not recommended and the self-repair detector should not start one under the compact profile.

## How a plan runs

1. **`/plan <brief>`.** The console wakes memory focused on the brief, starts `axlr_plan` with `brief` and `workspace`, claims `decompose` and asks the planning model for the task list. The planning model may differ from the workers' model (`plan.model` in `settings.json`, default the session model): decomposition is the hardest cognitive step and the one most worth a larger model when one is reachable.
2. **Verification.** The console checks the proposal mechanically (next section) and either completes `decompose` with `verified=true` or returns the exact defects for the next round. Three unverified rounds end `BLOCKED`.
3. **Approval.** The plan card shows the tasks as a table: id, goal, wave, scope, unit check, test-first, dependencies, and the end-to-end check with its baseline exit. `a` approves through the human guard (the separate approver identity, as `/incident`), `d` returns the plan with a typed reason, at most twice, and `x` declines. `plan.auto_approve` records `automatic` instead. Approving the plan approves every listed check command once: the workers' checks never ask again.
4. **Waves.** `READY` is terminal for the plan instance. The console computes waves from the dependencies (tasks with no unmet dependency form the next wave) and runs the tasks of a wave **sequentially** in the same workspace: one local GPU serves one model at a time, so parallel workers would only queue on it, and sequential execution removes every file conflict inside a wave. Each task is a new session the console drives in the background, as self-repair already drives its repair session: mode `task`, the context pack as the first message, autonomous local tools, the compact tool surface. The session is saved and can be reopened.
5. **Sync.** When a wave's tasks have ended (or every `sync.every_tasks` completions when set), the console starts `axlr_sync` for the wave: the end-to-end check runs; green records the integration; red opens a reconciliation round; two red rounds end `BLOCKED` with the exchange on the `/plan` panel. A task that ended `BLOCKED` is shown with its reason; tasks that depend on it are skipped and the plan ends partial.
6. **Memory.** The plan is written as a `decision` with the task table as evidence; each task's hand-back as a `success_path` or an `observation`; each sync as an `observation` carrying the notes. Labels: `plan`, `task`, `wave`, `session`, `ws`. The model never writes memory in these modes.

## `axlr_plan` 1.0

Inputs: `brief`, `workspace`; optional `memory_about`. States: `DECOMPOSE` → `APPROVAL` → `READY`; `BLOCKED`. `max_bounces: 3` is the engine's backstop; the console allows two returns.

| Step | Model returns | Console checks | Fields the console writes |
|:--|:--|:--|:--|
| `decompose` (at most 3) | `tasks` (1 to 12), `e2e_check`, `interfaces`, `summary_en` | everything below | `verified`, `settled`, `waves`, `defects` |
| `present` | nothing: the person's keypress | shows the table; records the decision; grants the guard on `a` | `decision` = `approve`, `return` (with `reason`), `decline` or `automatic` |

Each task in `tasks` is `{id, goal, scope, context, unit_check, depends_on, test_first, protect}`:

- `id` is a slug, unique in the plan; `goal` is one sentence.
- `scope` lists 1 to 8 workspace paths the worker may change; a path that does not exist yet is marked new. Scopes of tasks in the same wave must be disjoint.
- `context` lists up to 8 citations `{path, line, quote}` to the code the worker must read. Each is verified as in the citation check below; the console extracts the cited region with 20 lines of margin into the pack.
- `unit_check` is `{program, args}` with no shell. The console runs it once now: it must run (a program that does not exist is a defect); its exit code is recorded as the baseline and may be non-zero.
- `depends_on` names task ids; the graph must be acyclic. The console computes the waves.
- `test_first` says the worker writes the failing test before the change. `protect` lists up to 8 files that must not change during the task (the plan's own tests, generated files, lockfiles).

Plan-level fields: `e2e_check` is `{program, args}`, run once now and recorded as the baseline; `interfaces` is at most 2 KiB of text every worker receives (names, signatures and contracts the tasks share); `summary_en` is two or three English sentences for memory.

**Defects the console returns**, each with the task id: a duplicate or malformed id; an unknown or cyclic dependency; a scope path outside the workspace, or shared with another task of the same wave; a citation whose file is missing, whose line is out of range or whose quote is not on that line (whitespace-normalised); a command that does not run; a context pack over 12 KiB ("split the task"); more than 8 files in a scope; more than 12 tasks ("make a second plan"). A proposal with no defect is `verified`.

**Citation check.** The console reads the file through the workspace files port, takes the line at `line`, collapses whitespace on both sides and requires `quote` to be a substring of at least 12 characters. The check is deterministic and needs no model; a failed citation names the file, the line and the first 80 characters actually found there.

**Context pack.** The console builds it once per task, in this order, and refuses the plan when it exceeds 12 KiB: the task line (id, plan slug, wave, goal); the scope with new files marked; the interfaces; the unit check; the protected files; the notes addressed to this task by earlier hand-backs; the verified excerpts, each headed `--- path:first-last ---`. Nothing else from the plan or from other tasks reaches the worker.

## `axlr_task` 1.0

Inputs: `plan`, `task`, `workspace`; optional `memory_about`. States: `START` → `RED` or `GREEN` → `HANDBACK` → `DONE`; `BLOCKED`. The route out of `START` is decided by the plan: `test_first=true` goes through `RED`.

| Step | Model returns | Console checks | Fields the console writes |
|:--|:--|:--|:--|
| `start` (console) | nothing | records the SHA-256 of every existing scope and protected file; runs the unit check once as the baseline | `started`, `test_first`, `baseline_exit` |
| `red` (at most 3) | `test_files` (1 to 4, inside the scope), `expected`; or `untestable=true` with `observed` | every test file exists and differs from its start digest or is new; runs the unit check: it must exit non-zero | `red`, `settled`, frozen digests of `test_files`, command evidence |
| `green` (at most 3) | `summary`, `summary_en`; optional `notes` (up to 4 `{to, text}`), `questions` (up to 2) | frozen and protected digests unchanged, else the call is refused and the attempt is not spent; changed files inside the scope (from `git status --porcelain` when the workspace is a repository), else refused with the list; runs the unit check: it must exit zero | `green`, `summary`, `summary_en`, `notes`, `questions`, command evidence |
| `handback` (console) | nothing | records the changed files, `git diff --stat`, `git rev-parse HEAD`, the unit check evidence, the notes and the questions | `done`, `changed_files`, `revision` |

`red` is where the worker names the test it wrote; the console freezes exactly those files. A broken test (one that fails for a compile error) is still red; the frozen files then keep `green` from passing, the three rounds end `BLOCKED` and the hand-back shows the output, which is the honest outcome. `expected` is kept as evidence and compared against the output tail as a warning, never as a verdict.

`notes` are the only channel between tasks: `to` is a known task id or `all`, `text` is at most 500 characters. They reach the addressed tasks' packs when those tasks start later, and every worker's sync packet. `questions` are for the person: shown on the panel and recorded, never blocking.

Without Git the console cannot see out-of-scope changes; it says so in the hand-back and checks only the frozen and protected digests.

## `axlr_sync` 1.0

Inputs: `plan`, `wave`, `tasks`, `workspace`; optional `memory_about`. States: `INTEGRATE` ⇄ `RECONCILE`; `SYNCED`; `BLOCKED`. `max_bounces: 4` is the backstop; the console allows two reconciliation rounds. The validator warns that the cycle has no human-controlled state; `axlr_repair` 1.0 carries the same warning for its check rounds and is published, so the precedent holds, and the person sees every blocked sync on the panel.

| Step | Who | Console does | Fields the console writes |
|:--|:--|:--|:--|
| `integrate` | console | runs the plan's end-to-end check | `verdict` = `green`; `red` with the 2 KiB output tail while a round remains; `blocked` when the rounds are exhausted or the command cannot run |
| `reconcile` | one fresh worker per affected task, in turn | builds a sync packet per task; enforces scope and digests as in `green`; collects the responses; completes the step once | `reconciled`, `responses` = `[{task, changed, summary, notes}]` |

**Affected tasks** are those whose scope files appear in the failing output; when none does, every task of the wave. **The sync packet** is the task's own hand-back, the other tasks' hand-backs (summary, changed files, notes), the notes addressed to it and the failing output tail, with the instruction to make the end-to-end check pass by changing only its own scope, or to leave a note for the task whose change conflicts. A worker that changes nothing and leaves no note is recorded as such. After the last response the console runs `integrate` again.

This is the periodic ceremony of the brief: the workers "talk" through notes the console relays, each in a fresh context with exactly the others' hand-backs, and the conversation is a MADE record the person can read.

## The compact profile

The profile applies to every ceremony when the configured model is small (`ceremonies.profile: compact` in `settings.json`, or automatically when the model's context is 64K tokens or less once local models can be configured). Values are starting points to be measured, not conclusions.

| Knob | Today | Compact | Why |
|:--|:--|:--|:--|
| Tool calls per step | 32 | 16 | twelve-factor's 3-to-20-step agents; the window cannot hold 32 results |
| Projected tool result | 64 KiB | 8 KiB | the worker reads in pages; a 300-line Go file is about 10 KB |
| Projection ceiling / low watermark | 1 MiB / 768 KiB | 80 KiB / 56 KiB | about 23K tokens at 3.5 bytes per token, leaving 4K for output and the prefix in a 32K window; measure the real bytes-per-token with the diagnostics and set the ceiling at 75% of the window |
| Extractive checkpoint | 16 KiB | 4 KiB | the ledger below carries the step facts; the checkpoint only bridges inside a step |
| `axlr_step_done` schema | 2,255 B, every ceremony's fields | the current step's fields only, about 300 to 600 B | fewer fields to confuse; the schema changes only at step boundaries |
| Tools offered | 4 local + 8 host | workers: 4 local + `axlr_step_done`; read-only steps (`decompose`, `reproduce`, `diagnose`, `brief`) hide `local_write` and `local_edit` | fewer tools, better selection; a hidden tool is also refused when called from memory |
| Guidance | 1,870 B plus mode text | the first paragraph, the mode text and the step instruction; no MCP bridge or self-repair paragraphs when no plugin is exposed | about 1 KB of constant prefix |
| Step instruction | describes the fields | leads with one exact example call, under 500 B | small models copy examples better than they follow descriptions |
| Hand-back decoding | unknown field refuses the call | unknown fields are dropped and named in the reply; a `check_command` sent as one string is split on spaces when it has no shell metacharacters; `args` sent as a string is split the same way | one refused call is one lost model turn |
| Repeated call | allowed | the same tool with the same arguments twice in a row is refused with "same call as before; change something" | the loop small models fall into; OpenHands' stuck detector fires at four identical action-observation pairs |
| Reply without a tool call | the turn ends | a worker whose reply carries no tool call gets one reminder; a second such reply ends the task `BLOCKED` with "the model did not use its tools", which names the serving setup as the first suspect | the "plain chatbot" failure OpenHands and Goose document for local models |
| Step boundary | the transcript continues | **ledger projection**: the step starts from the recorded outputs of the earlier steps (a few hundred bytes each) plus the wake text; the earlier steps' tool chatter leaves the projection but stays in the saved transcript | consolidated context per step; the transcript remains complete and recoverable with `axlr_history` |
| Memory wake in the instruction | 2 KiB | 1 KiB | the pack carries the task's own context |
| Check output in feedback | 4 KiB | 2 KiB | the tail is what the model acts on |

Serving-side prerequisites that no console knob replaces (research record, sections 3 and 4): an OpenAI-compatible endpoint, which AXLR does not have yet (the client is fixed to `https://openrouter.ai/api/v1/chat/completions`); a tool-call parser matched to the model's template (`hermes` or `qwen3_coder` for Qwen, `mistral` for Devstral, `openai` for gpt-oss, `gemma4` for Gemma 4); grammar-constrained tool-call decoding at the server so arguments are always valid JSON; an explicit context length of at least 32K (Ollama defaults to 4,096 under 24 GiB of VRAM and truncates silently from the start of the prompt); KV cache at f16 or q8_0, since extreme KV quantisation degrades tool calling; a prompt cache that survives between turns, which needs the stable prefix above; and the vendor's sampling settings per model, with thinking off for workers and, when reachable, a larger planner.

## Decisions taken (Tirso, 7 Oct 2026)

Answered by survey before section 3 started. They replace the proposals below where they differ:

1. **Planner model:** a large model plans by default, `z-ai/glm-5.3-flash`, overridable with `plan.model`. The local 27B runs the tasks.
2. **Workers:** sequential, in the shared workspace.
3. **Approval:** the person approves every plan, and the approval covers its check commands.
4. **Limits:** 2 returns per plan, 2 reconciliation rounds per sync, 3 rounds per phase and 16 calls per step.
5. **Scope:** `git status --porcelain` plus digests; without Git, only the digests, and the hand-back says so.
6. **Coordination:** notes relayed by the console, and also KMP memory. Notes are recorded with `plan` and `task` labels, and a worker's pack carries the notes the memory holds for its plan.
7. **Where the plan lives:** `plans.json` in AXLR's state; nothing is written into the workspace.
8. **Profile:** the explicit setting first, otherwise automatic by the model's window.

## Decisions to take (Tirso)

1. **Planner model.** Same local 27B for `/plan`, or a larger model named by `plan.model` with the 27B as the worker. The design supports both; the verification step is what makes a 27B plan acceptable.
2. **Sequential workers.** One task at a time in the shared workspace. Git worktrees per task are the natural v2 when a second GPU or a cloud planner makes parallel workers worthwhile.
3. **Approval by default.** The person approves every plan (`plan.auto_approve` false) and this approval covers the listed checks; the workers never ask.
4. **Limits.** Two returns per plan, two reconciliation rounds per sync, three rounds per phase, 16 calls per step; engine backstops at 3 and 4 bounces as spiked.
5. **Scope needs Git.** Out-of-scope detection uses `git status`; without a repository only frozen and protected digests are enforced and the hand-back says so.
6. **Notes are the channel.** No direct messaging between workers; a note is at most 500 characters, addressed to a task or to all, and relayed only by the console.
7. **Where the plan lives.** The plan instance holds the tasks; the console keeps `<state>/axlr/plans.json` with the task and sync instances, as `repairs.json` does for repairs; `/plan` opens the panel; nothing is written into the workspace.
8. **Profile selection.** Explicit setting first; automatic by context size once a model's window is known from its configuration.

## Spike against MADE 0.10.0 (6 Oct 2026)

Run with `python3 tools/ceremonies/spike_drafts.py --made-bin <made-mcp>` in a disposable store. All three drafts validate as publishable; semantic digests: `axlr_plan` 1.0 `379619a6f6b49c799f64b43e23c2686507d30f1b1b27e368d14f512d52ef2ad4`, `axlr_task` 1.0 `5b678bc6b5cc1078b2a92dd8fff948dc85aeb547b4dd615bbb81827ae006d763`, `axlr_sync` 1.0 `1a76afda71cc5d9941beb927e5d20b5fb11d2312028e433200f11952406bf090`. These become pins only when the driver runs them.

Walked, all as expected: plan approved by the person (`approve` without the guard is refused, `approved_automatically` after a manual `approve` is refused); plan returned, decomposed again and declined; plan approved automatically; three returns accepted and the fourth cycle refused by the bounce limit; three unverified rounds, a fourth claim refused, `decompose_exhausted` → `BLOCKED`. Task test-first through `RED`, `GREEN`, `HANDBACK` to `DONE` (`test_present` refused when `test_first=true`, `test_failing` refused with `red=false`); task without test-first straight to `GREEN`; declared untestable → `BLOCKED`; three failed greens → `BLOCKED`. Sync green at once; red → reconcile → red → reconcile → blocked; red → reconcile → green; four rounds accepted and the fifth cycle refused by the bounce limit.

## Measured on the GX10 (7 Oct 2026)

ASUS GX10 (GB10, 121 GiB unified memory, aarch64). Three servers on loopback, each answering the smoke tool request with one valid `tool_calls` entry: Qwen3.8-27B UD-Q6_K and its abliterated variant on llama.cpp `b23efaa` (`--jinja`, `-c 262144`, KV q8_0, ports 8080 and 8081), and Gemma 4 31B QAT W4A16 on vLLM 0.22.1 (`--tool-call-parser gemma4 --reasoning-parser gemma4`, `--max-model-len 65536`, port 8082). One request at a time, the other servers idle, a prompt cut from `tui/application/*.go`:

| Model | Prompt | Cold prefill | Decode | Prefix-cache hit |
|:--|--:|--:|--:|--:|
| Qwen3.8-27B Q6_K | 4K | 626 tok/s (6.6 s) | 9.0 tok/s | 0.2 s |
| Qwen3.8-27B Q6_K | 32K | 595 tok/s (55 s) | 8.3 tok/s | 0.2 s |
| Gemma 4 31B W4A16 | 4K | 644 tok/s (6.4 s) | 9.3 tok/s | 0.2 s |
| Gemma 4 31B W4A16 | 32K | 553 tok/s (59 s)¹ | 8.4 tok/s | 0.4 s |

¹ The 32K prompt starts with the 4K one, so vLLM's prefix cache served its first 4K tokens.

Bytes per token: Qwen 3.6 on Go source and 3.8 on Markdown; Gemma 3.2 on Go source. The console's window-derived budget assumes 3.

What this changes against the table above: the window is not the constraint on this machine (256K served, and the console lets `context_tokens` lower it); decoding is. At about 9 tokens per second a 400-token report costs 45 s and a 2,000-token step 4 minutes, while a cold 32K prefill costs about a minute and a cached one a fraction of a second. The compact profile's case for short hand-backs, one example call and a stable prefix is stronger here than the design assumed; its case for an 80 KiB ceiling is weaker. Decision 8 should weigh decode speed, not only window size.

## Compact profile measured (7 Oct 2026)

The compact profile is implemented for `/debug` and `/delivery` ([ceremonies guide](../ceremonies.md#the-compact-profile-for-small-models)). It covers every knob in the table above except the "reply without a tool call ends the task" rule, which belongs to the task worker of section 3 and is tracked for driven ceremonies in [#70](https://github.com/underpass-ai/AXLR/issues/70).

The 1 Oct comparison of a ceremony against direct work was repeated with a local model. Setup:
- **Model:** Gemma 4 31B QAT W4A16 on vLLM 0.22.1, with `stream: false`.
- **Environment:** a disposable HOME with its own MADE and KMP stores.
- **Task:** the same ten-line change in a scratch Go module: `WordCount` treats any run of whitespace as one separator and returns 0 for blank text, plus table-driven tests.
- **Runs:** three per arm.
- **Success:** a hidden acceptance test the model never saw.

| Arm | Accepted | Mean time | Tool calls | `axlr_step_done` | Prompt tokens | Largest projected history |
|:--|:--|--:|--:|--:|--:|--:|
| `/normal`, direct | 3/3 | 133 s | 10–13 | — | 38–57K | 12–16 KB |
| `/delivery`, standard profile | 3/3 | 235 s | 16–19 | 3–4 | 81–99K | 18–24 KB |
| `/delivery`, compact profile | 3/3 | 158 s | 13–14 | 3 | 23–25K | 11–14 KB |

**What it shows**
- The compact profile cuts the ceremony's prompt tokens by about 73% and its time by about a third.
- It keeps the ceremony's guarantee that the console, not the model, ran the check. It also spends no calls on host bookkeeping: the standard arm used `axlr_skill`, `axlr_session` and, once, `axlr_tools` with `axlr_call_tool`.
- Every hand-back was accepted the first time, and no run hit a refusal.
- The 1 Oct gap with the skill catalogue (55 calls against 11) does not reappear. With the console driving, a 27B-class model completes the ceremony at about 1.2 times the calls of direct work.

**Streaming confound**
- The first runs streamed. There, vLLM's `gemma4` parser sometimes returned a tool call as text (`<|tool_call>call:…`). Replaying one captured request, the call leaked in 1 of 3 streamed replies and in 0 of 3 non-streamed ones.
- When it happened, a `/normal` turn ended with the code broken, and two compact ceremonies waited with the step open after the console's single reminder.
- `local_models[].stream: false` avoids the leak; detecting the stall is [#70](https://github.com/underpass-ai/AXLR/issues/70).

**Not measured**
- Qwen3.8-27B.
- `/debug`.
- A task larger than ten lines.
- The ledger cut under a long step: each step here stayed far below 80 KiB.

## Plan, task and sync measured (7 Oct 2026)

**Setup**
- **Run:** one three-task plan, run twice end to end in the console, through a pseudo-terminal that approved the commands and the plan.
- **Models:** `z-ai/glm-5.3-flash` planned over OpenRouter; Gemma 4 31B on vLLM (`stream: false`, compact profile) ran the tasks.
- **Environment:** a disposable HOME with its own MADE and KMP stores.
- **Brief:** fix `WordCount`, add `LineCount`, add `CharCount`, each with table-driven tests, in the same scratch module as the profile measurement.
- **Success:** a hidden acceptance test.

| Run | Planning | Plan | Tasks | Syncs | Workspace after | Total |
|:--|--:|:--|:--|:--|:--|--:|
| 1 | 3.5 min, verified first round | 3 tasks in 3 waves (all three share `textstat.go`, so the planner chained them) | `wordcount` done in 5.5 min, `linecount` done in 7 min, `charcount` blocked after 20 min in `red` (16 calls spent repeating a `local_edit` whose `old_text` no longer matched) | waves 1 and 2 green | build broken by `charcount`'s half-done edit; acceptance failed | 37 min |
| 2 (scope restore added) | 13 min, verified first round | same shape | `wordcount-whitespace` blocked after 2.5 min in `red`; the other two skipped | none | `textstat_test.go` restored, `go test` green, nothing implemented | 16 min |

Four more runs the same evening, each after a console-side fix for what the previous one exposed:

| Run | Worker | Fix since the previous run | Outcome | Total |
|:--|:--|:--|:--|--:|
| 3 | Gemma 4 | `local_exec` repair: wrapping quotes stripped, a program holding spaces split | first task blocked in `red`: Gemma wrote the test, then fixed the code in the same step, so `red` never failed (×3) | 11 min |
| 4 | Gemma 4 | `red` may write only test files | 2 of 3 done, syncs green; `charcount` blocked in `green` on arguments sent as one joined string | 21 min |
| 5 | Gemma 4 | joined arguments split; the repeat refusal shows the call shape | 2 of 4 done (glm planned 4 tasks), syncs green; `add-charcount` spent its budget on mixed wrappers before the hint worked | 16 min |
| 6 | **Qwen3.8-27B** (llama.cpp, `thinking: false`) | mixed wrappers stripped; `local_models[].thinking` added after a Qwen run with thinking on spent about 4,000 tokens before a single call | **plan done: 3 of 3 tasks, sync green, hidden acceptance test passed** | **13.5 min** |

Run 7 measured the planner's focused surface: glm made 7 read and exec calls before handing the plan back, with no host-tool detours. It also exposed a race. After the approval, glm kept working in the plan's session and wrote `CharCount` and its test itself. The third worker found them already present and declared its task untestable, so the run was partial: 2 of 3 tasks, both syncs green. Since then an approved plan's session asks the model nothing more.

In run 6, glm put `LineCount` and `CharCount` in new files, so the three scopes were disjoint and one wave held all three tasks. Planning took 3.5 min. The tasks took 3, 1.8 and 3.4 min, each going through `red` and `green` once. Every hand-back left a note for `all`.

**What worked**
- The planner's proposals verified on the first round in every run.
- Plan approval, sequential waves, frozen test digests, the git-status scope check, hand-backs with notes, per-wave syncs and skipping the dependents of a blocked task all behaved as designed.
- Run 1 exposed a real defect: a blocked task left its partial edit and broke the build for everyone. Since then the runner restores a blocked task's scope files (files the task created are named, not deleted).

**What failed, and where**
- **Planner wandering.** Under the standard profile, glm spent calls on host tools (`axlr_skill`, KMP guide, `kmp_wake`) before proposing. One of its responses streamed 1.5 MB, almost all reasoning. Giving the decompose step a compact tool surface is the next step.
- **Malformed worker calls.** Both blocked tasks failed on the Gemma 4 and vLLM `gemma4` interface, not on the orchestration. The worker sent `local_exec` with `program: "go test"` and arguments wrapped in `«…»` or backticks, and repeated the call. The repeated-call guard refused it seven times, but each refusal still spent the step's budget.
- **Follow-up.** Extend the compact profile's tolerant decoding to `local_exec`: split a program with a space, and strip wrapping quote characters. Then measure Qwen3.8-27B as the worker.
- **Not run.** The frontier-worker arm (glm for the tasks too), at Tirso's choice, to save credit.

## Layer map

| Layer | New or changed | Responsibility |
|:--|:--|:--|
| `tui/adapters/ceremonyhost/definitions/` | `axlr_plan-1.0.yaml`, `axlr_task-1.0.yaml`, `axlr_sync-1.0.yaml` moved from the drafts, pinned in `definitions.go`; `/mcp → P` publishes seven definitions | the published contract |
| `tui/domain` | `WorkMode` gains `plan` and `task`; `CeremonyRun` gains `Plan *PlanRun` and `Task *TaskRun` (digests, frozen files, scope, notes); `ContextBudget` profile; `MaxTurnToolCalls` becomes a profile value | what a session knows |
| `tui/application` | `ceremony_plan.go`: decomposition verification, waves, context packs, the plan card decision; `ceremony_task.go`: start, red, green, handback; `ceremony_sync.go`: integrate, reconcile, sync packets; `step_done_schema.go`: the per-step schema; `compact_profile.go`: budgets, tool surface per step, tolerant decoding, repeated-call guard; `ledger_projection.go`: the step-scoped projection; `plans.go`: the registry and the background worker driver, generalised from `self_repair.go` | the state machine walk and the console's checks |
| `tui/adapters/ceremonyhost` | `Checks` gains digest reads and `git status` through the files port; `Memory` labels for plan, task and wave | ports over the existing runtime |
| `tui/adapters/storage` | `plans.json` registry; `<id>.ceremony` sidecar fields for plan and task | durable pointers |
| `tui/adapters/terminal` | `/plan` (`/planificar`), the plan card and panel, the footer `plan <slug> · wave 2/3 · t4 green 2/3`; `settings.json` keys `plan.model`, `plan.auto_approve`, `sync.every_tasks`, `ceremonies.profile` | the person's view |
| `tools/ceremonies` | `check_pins.py` validates the drafts as well; `spike_drafts.py` walks them; CI runs both | evidence without a model |

Order of work: (1) the OpenAI-compatible endpoint, without which no local model reaches the console; (2) the compact profile on `/debug` and `/delivery`, measured against a real 27B with the diagnostics' request bytes and token usage; (3) `/plan`, the task worker and the sync, with the plan card and panel; (4) a measured run of a three-task plan on a 27B and a frontier model, as the 1 Oct measurement was done, before any of this becomes a pin.

## Out of scope

Parallel workers and worktrees; a planner that reads issues or pull requests; agents that message each other outside the console; changing `/incident` for small models; fine-tuning or distilling a worker model; any claim about a model this design has not been measured with.
