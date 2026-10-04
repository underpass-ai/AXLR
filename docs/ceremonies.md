# Ceremonies

AXLR runs ordinary work directly. The console starts a MADE ceremony when the user selects `/debug`, `/delivery`, `/incident` or `/repair`; the console drives the instance and the model does each step's work. The model never drives a console ceremony through direct `made_*` calls. For any other MADE procedure the user asks for explicitly, the model works with the registered MADE tools under their own approval policy, without a bundled catalogue.

## Console-driven ceremonies

| Mode | Definition | Sequence and acceptance |
|:--|:--|:--|
| `/debug` | `axlr_debug` 2.0 | Reproduce with a nonzero command exit → diagnose → repair until the same command exits zero → integrate |
| `/delivery` | `axlr_delivery` 2.0 | Brief with criteria, scope and baseline command → build until that command exits zero → integrate |
| `/incident` | `axlr_incident` 1.0 | Triage → evidence timeline → analysis and actions → draft and fresh-context review → person's approval → postmortem publication |
| `/repair` | `axlr_repair` 1.0 | Reproduce → diagnose → repair in a fresh clone → the console commits, pushes and opens the pull request → watches its checks → merge decision (automatic or the person's) → squash merge |

The [MADE runbook](runbooks/made.md#prepare-the-driven-ceremonies) prepares the work identity and publishes the four pinned definitions. Selecting a mode does not start an instance; the next prompt does. AXLR checks the published semantic digest, starts the instance and claims its first step. Missing or conflicting definitions stop the start.

The model does real work with AXLR tools and returns the current step's fields through `axlr_step_done`. AXLR runs the check itself, records its output in MADE, applies an enabled transition and claims the next step. The model must not also drive that instance through direct `made_*` calls.

A proposed check is `{program, args}` with no implicit shell. The first command, or a changed reproduction command, needs user approval even in full autonomy. Once reproduce/brief is accepted, repair/build always reruns the saved command: a replacement supplied by the model is ignored. Each invocation has a five-minute timeout, 16 KiB output capture and a 4 KiB evidence tail. The local process receives the console's restricted environment described in [work modes](console.md#work-modes).

Debug reproduction and repair, and delivery build, allow at most three iterations per phase. A failure to reproduce can also be declared explicitly. Exhaustion or declared non-reproducibility leads to `BLOCKED`; successful integration leads to `COMPLETED`. Both clear the active ceremony and return the session to normal. Integration records the report, `git rev-parse HEAD` and dirty-file output; it does not commit, merge or publish. A zero check exit proves only what that command actually checks, and integration does not require a clean Git tree.

### Incident review and approval

`/incident` reconstructs a blameless postmortem from workspace evidence. Timeline entries need ordered RFC3339 timestamps and evidence; actions need an owner, a future due date and verification. The model writes `docs/incidents/<slug>.draft.md`. A reviewer in a fresh context checks the draft, with at most two review rounds. This is a separate model context, not proof of an independent human reviewer.

The console then presents the exact reviewed draft for the person's decision. Open `/incident`: `a` approves the displayed bytes; `d` returns them with a reason, up to twice. The approval guard uses a separate approver identity. The model cannot approve the draft through `axlr_step_done`. Approved bytes are written unchanged to `docs/incidents/<date>-<slug>.md`; publication verifies their digest before completion. Reviewer or approval failures remain pending for recovery.

### Self-repair: the console drives the pull request

`/repair` repairs the repository named by `repair.repository` in `settings.json` (default `underpass-ai/AXLR`). Because the console is bound to the workspace it was launched in, a repair starts from the launcher: `axlr-tui --repair "<failure brief or #issue>"` clones the repository into `<data>/axlr/repairs/<date>-<slug>/` (or `repair.directory`), writes `.axlr-repair.json` there, opens the console rooted in that clone in repair mode and fills the composer with the brief; `#123` reads the issue through `gh`. `/repair` in a workspace without that marker refuses to start and prints the command.

Reproduce, diagnose and repair work as in `/debug`, with the same once-approved check command. After the check passes, the console does the rest without the model: it commits the clone's changes as `AXLR <axlr@underpass.ai>` with the trailer `Repaired-by: AXLR axlr_repair 1.0 <instance>`, pushes `repair/<slug>`, opens the pull request against the default branch (cause, fix, summary and evidence in its body) and polls its checks every 30 seconds for at most `repair.watch_minutes` (default 45). A branch that falls behind a protected base is updated once per round. Red checks go back to repair with the failing checks named, at most twice; a third red round, a stall past the deadline, a conflict, a pull request closed outside the ceremony or a forge the console cannot read all end `BLOCKED` with the reason in MADE and the pull request left as it is.

Green checks reach the merge decision. With `repair.auto_merge` true the console records `decision=automatic` and merges; otherwise the merge card opens (`/repair` reopens it): `a` records `approve`, grants the human guard through the approver identity and merges with squash, deleting the branch; `d` records `decline` with a reason and ends `BLOCKED`. MADE therefore records whether the setting or the person merged. The model cannot commit, push, open or merge through `axlr_step_done`, and repair mode refuses `kmp_write_memory`.

Memory follows the repair twice. The initial wake on `repair.about` (default `project:<repository name>`) carries the brief as `intent`, so a store with Jev configured keeps the evidence relevant to the failure; the recall's refs are offered to the model. When diagnose is accepted the console writes the cause (`error_path`) with the `connect_to` links the model proposed among those refs, with their `why`; links to refs the recall did not show or with unknown relation names are refused and named in the reply, and a link KMP refuses does not lose the cause. At the terminal state the console writes the outcome (`success_path` with the pull request and merge revision, or an `observation` of why it stopped) linked to the cause. Both carry the labels `ceremony`, `repair`, `repository`, `pull_request`, `session` and `ws`.

A watch interrupted by cancellation, a crash or a reload is resumed before the next turn: the console reads the instance from MADE, recovers the pull request number from the recorded propose output and re-enters the console step. The design is in [the self-repair plan](plans/2026-10-05-self-repair-design.md).

### Memory and recovery

With a registration named exactly `kmp`, AXLR attempts a wake before the procedure and an outcome write at its terminal state. It uses the exact about saved through `axlr_session`, with `ws:<session-id>` as fallback. The driver rechecks the selected about at completion, since startup may have established it after the first claim. Wake errors are disclosed without stopping MADE; an outcome write failure is reported as `not recorded`. Bounded recall retains refs and marks pending continuation; it is not a complete project-memory audit. Terminal outcomes are observations with stable idempotency keys. A `needs_review` response remains pending for writer inspection, rather than being acknowledged blindly. For other reusable decisions, follow the [KMP runbook](runbooks/kmp.md#use-memory-in-a-task).

MADE remains the authority for claims and progress. On some interrupted completions/transitions, the driver inspects the instance and reconciles using MADE's enabled transitions and claimable steps. Multiple enabled transitions or an unresolved active claim require operator inspection. Reloading a session does not itself resume a turn or prove that a check had no effect. See [recovery](runbooks/recovery.md#recover-a-session-or-ceremony).

## Why there is no bundled skill catalogue

Until 5 Oct 2026 the console embedded a `made:axlr-ceremonies` skill with seven 1.0 definitions (`axlr_change`, `axlr_delivery`, `axlr_debug`, `axlr_review`, `axlr_research`, `axlr_publish`, `axlr_handoff`) for the model to drive itself. Measured on 1 Oct 2026 with a real model, the skill-guided path spent its whole turn on protocol (55 calls, context exhausted, code left broken) where direct work took 11 calls; publication had no approver and handoff no receiver. Review and research became [work modes](console.md#work-modes); debug and delivery became console-driven 2.0 definitions; change and handoff were dropped; the catalogue itself was retired. The [research notes](plans/2026-10-01-ceremony-driver-design.md) record the measurements and the decision.

Roles in a definition describe responsibilities and do not spawn agents. Independent review requires a distinct context or person: the incident ceremony uses a fresh-context reviewer and a person's approval, the repair ceremony a person's merge decision. MADE records caller-declared actor/role provenance, so guard approval and grant administration stay off the work identity; a role name alone does not enforce separation.

## Source and validation

The [driver](../tui/application/ceremony_driver.go), [definition pins](../tui/adapters/ceremonyhost/definitions.go) and [preparer](../tui/adapters/madesetup/preparer.go) define console behavior. Run the module checks without invoking a model or touching a live ceremony store:

```bash
GOWORK=off go -C tui test ./application ./adapters/ceremonyhost ./adapters/madesetup
```

To validate the shipped definitions and their pins against a compatible MADE binary in a disposable store, as CI's `ceremonies` job does:

```bash
python3 tools/ceremonies/check_pins.py --made-bin /absolute/path/to/made-mcp
```

Each YAML is validated and published into the disposable store; its semantic digest must match the pin in `definitions.go`, and every pin must have a YAML. Bump the immutable published version when a definition's content changes.
