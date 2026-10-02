# Ceremony Driver Design

Status: implemented on `feat/ceremony-driver` (2 Oct 2026). The section "As built" records where the implementation differs from the design below.
Builds on: [work modes](2026-10-01-work-modes.md), which must merge first.
Spec: ceremony review https://claude.ai/artifact/ScnDk7CV34Lt5e1iCUjNeH and Tirso's decisions of 1 Oct 2026 (the host drives MADE; only delivery and debug remain as ceremonies, as 2.0; `/resume` replaces handoff).

## Problem it solves

With the 1.0 catalogue the model drives MADE by hand. It reads about 16 KB of skill, resolves schemas, then calls start, claim, complete and transition through `axlr_call_tool`. Measured with the installed console: 55 calls and a broken edit for a ten-line change, against 11 calls and a correct result without a ceremony. Its success fields (`verified`, `repaired`) are also written by the same model that did the work.

## Principle

The console drives the protocol. The model works.

| Concern | Owner |
|:--|:--|
| Start, claim, fence, complete, transition, resume | Console (Go), deterministic |
| The work of each step | Model, with its normal tools |
| Handing a step's result back | Model, through one host tool: `axlr_step_done` |
| Checks that decide success (reproduction, test command) | Console executes them and writes the result into the step output |
| Memory before and after | Console calls KMP; the model sees the wake packet as context |
| Human approvals | The existing approval card |

## Modes that use it

| Mode | Definition | States |
|:--|:--|:--|
| `/debug` (`/depurar`) | `axlr_debug` 2.0 | `REPRODUCE` → `DIAGNOSE` → `REPAIR` (repeat ≤ 3) → `INTEGRATE` → `COMPLETED`, or `BLOCKED` |
| `/delivery` (`/entrega`) | `axlr_delivery` 2.0 | `BRIEF` → `BUILD` (repeat ≤ 3) → `INTEGRATE` → `COMPLETED`, or `BLOCKED` |
| `/resume` (`/retomar`) | none | finds this workspace's open instance and continues it in its mode |

`BLOCKED` is a real terminal state, reached through `state_repeat_exhausted:<STATE>`, which MADE has supported since v0.9.0. It records the last failure and the cause. It replaces 1.0's choice to leave an exhausted instance open forever.

There is one role (`AXLR`) and no review step that would only be self-review. Delivery's acceptance is the console-run check plus the user's review of the diff in `/changes`.

### What the console checks

| Step | Model returns (`axlr_step_done`) | Console then | Guard field written by the console |
|:--|:--|:--|:--|
| debug `reproduce` | `check_command`, `expected`, `observed` | runs `check_command`; it must **fail** | `reproduced = exit != 0` |
| debug `diagnose` | `root_cause`, `evidence`, `proposed_fix` | none; MADE records it for the trail | `diagnosed = true` when the fields are non-empty |
| debug `repair` | `summary` | runs the same `check_command`; it must **pass** | `repaired = exit == 0` |
| delivery `brief` | `criteria`, `check_command`, `scope` | runs `check_command` once as baseline | `ready = true` when the fields are non-empty |
| delivery `build` | `summary` | runs `check_command`; it must pass | `verified = exit == 0` |
| `integrate` | `report` | records revision (`git rev-parse HEAD`, dirty files) | `integrated = true` |

The model cannot set a guard field. The console writes the model's fields and its own evidence (`command`, `exit_code`, the last 4 KB of output, revision) into the step output, then completes the step.

## Layer map

| Layer | New or changed | Responsibility |
|:--|:--|:--|
| `tui/domain` | `ceremony_run.go`: `CeremonyRun{Instance, Definition, Version, State, Step, Iteration, CheckCommand}` | What the session knows about its live instance |
| | `WorkMode` gains `ModeDelivery`, `ModeDebug`; `Judge` returns `VerdictAllow` for both, so the model can edit code | Modes that start a ceremony |
| | `HostOperationStepDone` in `tool_identity.go` | New host tool identity |
| `tui/application` | `ceremony_engine_port.go`: `Start`, `Claim`, `Complete`, `Transition`, `Get`, `FindOpen(workspace)` | MADE as seen by the driver; DTOs only, no MCP shapes |
| | `check_runner_port.go`: `Run(ctx, workspace, command) CheckResult` | Runs one command in the workspace with a time limit |
| | `memory_port.go`: `Wake(about)`, `Record(about, decision, why, evidence)` | KMP as seen by the driver |
| | `ceremony_driver.go`: `Begin(mode, prompt)`, `StepInstruction(session)`, `StepDone(session, fields)` | The state machine walk: claim → instruct → collect → check → complete → transition |
| | `host_tool_use_case.go` + `host_tool_definitions.go`: `axlr_step_done` | Receives the step result; the agent loop then asks the driver for the next instruction |
| | `continue_turn_use_case.go` | Appends the current step instruction as a short system message after the transcript |
| `tui/adapters` | `madeceremony/engine.go` | `CeremonyEnginePort` over the plugin manager (`made_*` calls, strict DTO mapping, fences kept) |
| | `madeceremony/definitions/axlr_debug-2.0.yaml`, `axlr_delivery-2.0.yaml` | Embedded definitions; published only by the `/mcp` → P action |
| | `checkrun/runner.go` | `CheckRunnerPort` over the existing local exec runtime |
| | `kmpmemory/memory.go` | `MemoryPort` over the plugin manager (`kmp_wake`, `kmp_write_memory`) |
| | `storage/session_ceremony.go` | `<id>.ceremony` sidecar (same reason as `<id>.mode`) |
| | `terminal` | `/delivery`, `/debug`, `/resume`; footer `debug · reproduce` / `delivery · round 2/3 · check ✓` |

The grant `axlr-default-work-v1` (AXLR#43) already covers start, claim, complete, transition, get and search. Publishing 2.0 is a trusted-host write, so it belongs to the explicit `/mcp` → P action, which already runs as trusted host; a mode command never performs it. If 2.0 is not in the store, `/debug` and `/delivery` refuse with "Prepare MADE first: /mcp → P".

## Flow (`/debug`)

1. The user runs `/debug` then describes the failure, or runs `/debug <description>`. The console:
   - wakes memory for `ws:<session_id>` (decision 2);
   - starts `axlr_debug` 2.0 with `failure_brief`;
   - claims `reproduce`;
   - begins a model turn with the step instruction (about 600 bytes) and the wake packet.
2. The model investigates with its normal tools and calls `axlr_step_done{check_command, expected, observed}`.
3. The console runs the command (decision 1), completes `reproduce` with the model's fields, its evidence and `reproduced`, and applies the transition. If the command does not fail, the console tells the model why and `reproduce` repeats, at most 3 times; there is no new instance. Exhaustion, or the model returning `reproducible=false` for a failure no command can show, leads to `BLOCKED` through `step_repeat_exhausted:reproduce` or an output guard.
4. The same pattern runs for `diagnose` and `repair`. After three failed repairs, `BLOCKED` records the last output and the console says so.
5. `integrate` records the revision. The console writes the outcome to KMP: cause, fix and evidence, related to the woken context. The mode returns to normal.

A turn limit hit mid-step is harmless: the instance and claim survive, and Ctrl+R or `/resume` continues from the same step.

## Spikes before the task plan

- A lab test of the 2.0 YAMLs with `made_validate_ceremony_draft`, a happy path, and exhaustion leading to `BLOCKED`. Extend `tools/ceremonies/check.py`.
- Whether claim leases need renewal during a long model step (`renew_ceremony_step_lease` is in the work grant), or a long lease TTL is enough.
- What the 1.0 skill and `axlr-ceremonies` guidance become. Proposal: delete the 1.0 skill and YAMLs from the binary once 2.0 ships. Published 1.0 definitions stay in stores (immutable) and are never started again.

## Decisions (Tirso, 1 Oct 2026)

1. **Check command: approved once per ceremony.** When the step that proposes `check_command` ends, the approval card shows the exact command. The console then re-runs exactly that string each round without asking. A different command needs a new approval. The approved command is stored in the `<id>.ceremony` sidecar with the instance.
2. **KMP about: `ws:<session_id>`.** Each AXLR session that runs a ceremony has its own about, `ws:` followed by the 32-hex session id. Tirso, as KMP's author, set this convention, so it is not an about the console infers. The console wakes it on entry (an empty wake on a new about is expected) and writes the outcome to it. KMP's multidimensional labels carry the rest, on every record: `{"ceremony":["axlr_debug"],"step":["repair"],"ws":["<workspace path>"]}`. Spike: confirm the first `kmp_write_memory` on a new about creates it, and that reusing `ws` as both prefix and label key is fine.

## As built (2 Oct 2026)

- **Packages.** The adapters live in `tui/adapters/ceremonyhost`:
  - `Engine` (MADE), `Checks` (local exec) and `Memory` (KMP) all go through the existing tool executor, as the work identity.
  - The embedded `definitions/axlr_{debug,delivery}-2.0.yaml` files are pinned by MADE digest; `TestPinnedDigestsMatchTheShippedYAML` fails when a YAML changes without its pin. It runs when `made-mcp` is on PATH.
- **`reproduce`'s repeat stops on `settled`, not on `reproduced`.** While a step repeat is unsatisfied, MADE refuses every transition out of the state, `not_reproducible` included. The console sets `settled=true` when the failure is reproduced or declared not reproducible.
- **Approval rides on `axlr_step_done`.** No new approval surface. A `step_done` that proposes a check command not yet approved shows the normal approval card, labelled "ceremony check command". Any other `step_done` is console bookkeeping and auto-approves, like `axlr_tools`. Only `reproduce` and `brief` can propose and approve a command. From then on the check is fixed: a `check_command` sent in `repair` or `build` is ignored and asks nothing. This stops the loop from moving its own goal, and stops a garbled resend from burning an attempt (seen in a live run on 2 Oct 2026).
- **The check command is `{program, args}`, run without a shell.** Timeout is 300 000 ms (the runtime's hard limit) and output is capped at 16 KiB, of which the last 4 KiB reach MADE and the model. A command that never runs is refused in `reproduce` and `brief`, and counts as a failure in `repair` and `build`.
- **Call budget.** `TurnCallCount` is derived from the transcript, so it is never rewritten. Instead, `CeremonyRun.BudgetBase` marks where the last accepted step left the count, and the 32-call limit counts from there. Restoring a saved session no longer applies the live limit to history.
- **Memory.** KMP refuses non-English summaries, so `integrate` asks for `summary_en` alongside the user-language `report`. The `step_done` result says "recorded in ws:<id>" or why not.
- **Publication.** `/mcp` → P publishes 2.0 through a five-minute install grant given to the work identity and revoked with a reason.
- **Lease.** A claim lasts 1 h. Probed: completing after the lease expired still works while no one else claimed the step.
- **Resume.** Reopening the session (`-session <id>` or the picker) shows the live step in the footer. The next message continues with the stored fence. Verified to `COMPLETED`.

### Fixes from the full re-verification (2 Oct 2026)

These were found by rerunning every scenario with the installed `7e0a6c4` console, a fresh default-layout store, and `glm-5.3-flash`:

- **The check is fixed after `reproduce` or `brief`.** A `check_command` sent in `repair` or `build` is ignored and asks nothing. The bug it fixes: a garbled resend replaced the approved check, asked for approval, failed with a SyntaxError, and burned a repair attempt.
- **Memory is woken as prose.** At 2 KiB, KMP shortened the core text to "…", so the wake injected only envelope. The adapter now asks for 12 000 bytes and keeps `current_state`, `open_loops` and `next_actions` without refs.
- **The call budget survives `FinishCeremony`.** Before, the closing answer of a long ceremony turn hit the 32-call limit, because the restarted budget was dropped with the run. `FinishedBudgetBase` keeps it for the rest of the turn.
- **The `.ceremony` sidecar is read leniently.** It is a pointer to a durable MADE instance. A console that does not know a newer field must still find the instance; the version still guards the shape.
- **Open-step reminder.** When a turn ends with a ceremony step open (seen repeatedly: work done, never handed back; or a question to the user in place of `reproducible=false`), the console starts one visible turn: `[AXLR] The <step> step … is still open`. It reminds once per claimed step. While a step is open, the turn does not end on a question: the model has to hand the step back, and it can still advise the user.
