# Ceremonies

AXLR runs ordinary work directly. The console starts a MADE ceremony when the user selects `/debug` or `/delivery`; the model uses the embedded ceremony skill when the user explicitly requests another tracked procedure. These paths ship different definitions and must not be mixed within an active instance.

## Console-driven ceremonies: version 2.0

| Mode | Definition | Sequence and acceptance |
|:--|:--|:--|
| `/debug` | `axlr_debug` 2.0 | Reproduce with a nonzero command exit → diagnose → repair until the same command exits zero → integrate |
| `/delivery` | `axlr_delivery` 2.0 | Brief with criteria, scope and baseline command → build until that command exits zero → integrate |

The [MADE runbook](runbooks/made.md#prepare-the-driven-ceremonies) prepares the work identity and publishes the two pinned definitions. Selecting a mode does not start an instance; the next prompt does. AXLR checks the published semantic digest, starts the instance and claims its first step. Missing or conflicting definitions stop the start.

The model does real work with AXLR tools and returns the current step's fields through `axlr_step_done`. AXLR runs the check itself, records its output in MADE, applies an enabled transition and claims the next step. The model must not also drive that instance through direct `made_*` calls.

A proposed check is `{program, args}` with no implicit shell. The first command, or a changed reproduction command, needs user approval even in full autonomy. Once reproduce/brief is accepted, repair/build always reruns the saved command: a replacement supplied by the model is ignored. Each invocation has a five-minute timeout, 16 KiB output capture and a 4 KiB evidence tail. The local process receives the console's restricted environment described in [work modes](console.md#work-modes).

Debug reproduction and repair, and delivery build, allow at most three iterations per phase. A failure to reproduce can also be declared explicitly. Exhaustion or declared non-reproducibility leads to `BLOCKED`; successful integration leads to `COMPLETED`. Both clear the active ceremony and return the session to normal. Integration records the report, `git rev-parse HEAD` and dirty-file output; it does not commit, merge or publish. A zero check exit proves only what that command actually checks, and integration does not require a clean Git tree.

### Memory and recovery

With a registration named exactly `kmp`, AXLR attempts a wake before the procedure and an outcome write at its terminal state. It uses `ws:<session-id>`, a session-scoped identity. Wake errors do not stop the ceremony; an outcome write failure is reported as `not recorded`. The wake prose is bounded and is not a complete project-memory audit. For continuity across sessions, explicitly recover and write the project's stable KMP scope as described in the [KMP runbook](runbooks/kmp.md#use-memory-in-a-task).

MADE remains the authority for claims and progress. On some interrupted completions/transitions, the driver inspects the instance and reconciles using MADE's enabled transitions and claimable steps. Multiple enabled transitions or an unresolved active claim require operator inspection. Reloading a session does not itself resume a turn or prove that a check had no effect. See [recovery](runbooks/recovery.md#recover-a-session-or-ceremony).

## Explicitly requested skill catalogue: version 1.0

The console also embeds `made:axlr-ceremonies` and seven definitions. This catalogue remains available for explicit requests, including publication and handoff. It does not automatically run for substantive tasks and `/mcp` → `P` does not install it.

| Procedure | Use it for |
|:--|:--|
| `axlr_change` | Small, understood, reversible edits |
| `axlr_delivery` | Feature work with build, verification and review roles |
| `axlr_debug` | Reproduction, diagnosis and repair roles |
| `axlr_review` | Prioritized review of an exact artifact |
| `axlr_research` | Primary evidence, synthesis and a decision artifact |
| `axlr_publish` | Exact verified artifact → human decision → publication → readback |
| `axlr_handoff` | Checkpoint → actual receiver acceptance → confirmed transfer |

Read the skill through `axlr_skill` without installing a package:

```json
{"plugin":"made","skill":"axlr-ceremonies","path":"SKILL.md"}
```

Then read `references/catalog.json`, the selected `references/axlr_<procedure>.yaml` and the relevant execution/handoff guide. Follow all byte-page cursors. Resources live in the [embedded skill directory](../tui/adapters/axlrplugin/builtin/made/skills/axlr-ceremonies/SKILL.md). The catalogue pins version 1.0, file SHA-256 and MADE semantic digest; a formatting hash is not the semantic definition identity.

These definitions were validated with MADE 0.9.1. Before execution, discover the live tools and verify compatible output-field guards, bounded repeats and timeouts. Publish absent definitions only when that setup is authorized, and compare existing immutable versions. Installing the catalogue starts no task. Use the selected 1.0 version explicitly; never substitute it for a driver expecting 2.0.

The skill-guided path uses claim → actual host work → structured completion → enabled transition. The model supplies the evidence; unlike the console driver, this path does not automatically run a fixed acceptance command. Failed delivery/debug loops preserve their failed state after three unsuccessful rounds. If the engine or grants are unavailable, authorized reversible work can continue locally, explicitly untracked. That fallback does not apply automatically to a selected `/debug` or `/delivery` mode.

Roles describe responsibilities and do not spawn agents. One worker can serve sequential roles, but its review is self-review. Independent review requires a distinct reviewer. Keep guard approval and grant administration off the work principal: MADE 0.9.1 records caller-declared actor/role provenance, so a privileged caller can approve a human guard regardless of its role label. A role name alone does not enforce separation.

## Handoff contract

The checkpoint includes objective and acceptance criteria; authorization and
constraints; worktree, revision and dirty files; decisions and evidence;
completed work, blockers and exact next action; checks tied to a revision;
artifacts and opted-in KMP identities; MADE instance, pinned definition, state,
cursors, active claims and uncertain external operation receipts.

The receiver reads the exact digest, verifies accessible evidence and current
resume state, and acknowledges its real identity and continuation ownership.
The integrator then records the supported handoff/rebinding and reads back its
receipt. Preparing a summary does not release a live claim. A sender playing a
second logical role does not prove receipt, and context compaction on the same
worker normally requires a checkpoint/resume instead of ownership transfer.

## Source and validation

The [driver](../tui/application/ceremony_driver.go), [2.0 pins](../tui/adapters/ceremonyhost/definitions.go) and [preparer](../tui/adapters/madesetup/preparer.go) define console behavior. Run the module checks without invoking a model or touching a live ceremony store:

```bash
GOWORK=off go -C tui test ./application ./adapters/ceremonyhost ./adapters/madesetup
```

To validate the 1.0 catalogue against an installed compatible MADE binary in a disposable store:

```bash
python3 tools/ceremonies/check.py --made-bin /absolute/path/to/made-mcp
```

This checks synthetic success/failure, repeat exhaustion, receiver acceptance and publication boundaries; it performs no external publication or real handoff. `--write-catalog` refreshes 1.0 file pins/digests after a reviewed revision, not the driver's 2.0 pins. Bump immutable published versions when their content changes. The [research notes](../tui/adapters/axlrplugin/builtin/made/skills/axlr-ceremonies/references/research.md) preserve the original catalogue design.
