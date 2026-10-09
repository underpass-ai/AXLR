# Ceremonies

AXLR runs ordinary work directly. The console starts a MADE ceremony when the user selects `/debug`, `/delivery`, `/incident`, `/repair` or `/improve`, or when the agent requests a [self-repair](#self-repair-from-a-running-session) with evidence; the console drives the instance and the model does each step's work. The model never drives a console ceremony through direct `made_*` calls. For any other MADE procedure the user asks for explicitly, the model works with the registered MADE tools under their own approval policy, without a bundled catalogue.

## Console-driven ceremonies

| Mode | Definition | Sequence and acceptance |
|:--|:--|:--|
| `/debug` | `axlr_debug` 2.0 | Reproduce with a nonzero command exit → diagnose → repair until the same command exits zero → integrate |
| `/delivery` | `axlr_delivery` 2.0 | Brief with criteria, scope and baseline command → build until that command exits zero → integrate |
| `/incident` | `axlr_incident` 1.0 | Triage → evidence timeline → analysis and actions → draft and fresh-context review → person's approval → postmortem publication |
| `/plan` | `axlr_plan`, `axlr_task`, `axlr_sync` 1.0 | Decompose a brief into verified atomic tasks → the person's approval → each task in a fresh worker (red, green, hand-back) → a sync after each wave; see [plans](#plans-atomic-tasks-for-small-models) |
| `/repair` | `axlr_repair` 1.0 | Reproduce → diagnose → repair in a fresh clone → the console commits, pushes and opens the pull request → watches its checks → merge decision (automatic or the person's) → squash merge |
| `/improve` | `axlr_improve` 1.0 | Brief with a check that fails in a fresh clone → build until it passes → the console commits, pushes and opens the pull request → watches its checks → the person's merge decision → squash merge |

The [MADE runbook](runbooks/made.md#prepare-the-driven-ceremonies) prepares the work identity and publishes the eight pinned definitions. Selecting a mode does not start an instance; the next prompt does. AXLR checks the published semantic digest, starts the instance and claims its first step. Missing or conflicting definitions stop the start.

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

### Self-repair from a running session

The agent of any session can ask the console to repair AXLR itself while its own task continues. Four stages are kept apart: **detection** is the agent's, **activation** is the console's, **repair** is the separate repair session's, and **updating the runtime** is still the person's.

**Detection.** The model is told to retry once when a `local_*` or `axlr_*` tool fails in a way that points at AXLR itself, and to request a repair only when the failure recurs. The host tool `axlr_request_repair` takes `description`, `expected`, `observed`, `evidence` (one to eight concrete items) and `tool_calls` (one to eight IDs of calls from the same session). It needs no approval card of its own: the request starts nothing the person has not already agreed to, because the reproduction command and the merge keep theirs.

**Activation.** The console validates the request against the session's own transcript before anything starts. Every cited call must exist, be resolved and belong to AXLR's own surface (`local_read`, `local_write`, `local_edit`, `local_exec` or an `axlr_*` host tool); MCP plugin tools are refused because their failures belong to the server. A call is accepted as evidence when the runtime reported `internal_error`, `filesystem_error` or `process_error`, when its outcome could not be read, when a host tool refused for a reason that is not about the arguments, or when the tool execution failed with an unknown effect for a reason that is not external; a call that succeeded can be cited as a wrong result, but then two cited calls are required. The console refuses, naming the reason, a program that exited nonzero (the project's failure), a rejected call (the arguments), `not_found`, `permission_denied` or `start_failed` (the workspace or the program), a timed-out, cancelled or denied call, and any request whose text names a provider, credential, permission or network cause (`OpenRouter`, `api key`, `401`, `permission denied`, `not logged in`, `connection refused` and similar). An isolated failure is refused: the same tool must have failed the same way at least twice in the session, counting calls the model did not cite. The failure signature (repository, tools and failure classes) then drives the limits: one active repair per console; a duplicate of an active or interrupted repair is refused; a signature already merged by this build is refused with the pull request and a reminder to update and restart; `repair.max_attempts` (default 2) bounds the repair sessions one signature may start; a repair session, a session whose workspace is a repair clone and a console without MADE cannot request a repair. Only then does the console check the pinned definition, clone the repository with the same preparer the launcher uses (the marker records `origin_session` and `build`), create the repair session bound to the clone in `/repair` mode, record both sessions in `<state>/axlr/repairs.json` and start driving the new session in the background. The reply to the model carries the repair ID, the session ID and the clone path; `axlr_repair_status` reads the records later.

**Repair.** The repair session runs the same `axlr_repair` 1.0 ceremony as the launcher path, with its own AXLR runtime rooted in the clone, the console's model client, session store and MCP connections, and the brief assembled from the request: description, expected, observed, evidence and the exact outcomes of the cited calls. Nobody reads its transcript, so the console drives it: local tools run without a card when `repair.autonomous` is true (the default; set it to `false` to keep the console's normal policy, and each call then waits on the panel), the reproduction command is approved once by the person, the console steps run as described above, and a model that ends its turn with a step open is reminded at most three times before the repair is recorded as failed. A failure that cannot be reproduced in the clone ends `BLOCKED` with the reason, as with `/repair`. The person's two decisions arrive on the **self-repairs panel**: `/repair` opens it whenever the session has linked repairs, it opens on its own when a decision waits and nothing else is on screen, and the footer shows `repair <name> · <status>` while one runs. `a` approves the check command or the merge, `d` denies the command or declines the merge with a reason, `r` recovers an interrupted repair, `esc` closes. `repair.auto_merge` keeps its meaning and its default (`false`). The cause and the outcome reach KMP through the ceremony's own writes; the procedure is MADE's instance; both reports appear on the panel and in the record (`memory`, `instance`), including a KMP write that was refused.

**Interruption and recovery.** Stopping the console cancels the background session; its record becomes `interrupted` with where it stood, the clone and the saved session are kept, and the console that starts next marks any record left running by another launch the same way. `r` on the panel reattaches a repair: the console loads the saved session, opens a workbench on the same clone and continues from what MADE and the forge already recorded, so a pull request is not proposed twice and a merge is not repeated. A repair that cannot continue is recorded as `failed` with the error, never silently retried past two attempts.

**Updating the runtime.** When the repair ends, the original session's next prompt carries a visible `[AXLR] Self-repair …` note with the pull request, the merge commit, the MADE instance and the KMP report, or the reason it stopped. A merge changes the repository, not the process that detected the defect: the note says which build the console still runs and that it needs an update or a rebuild and a restart before the repaired behaviour can be relied on. The console never replaces its own executable, and a second request for the same failure from the same build is refused with that reminder.

### Self-improvement: the same pull request, the person's merge

`/improve` is the sibling of `/repair` for a change that is not a failure: a capability the console lacks, a rough edge, an issue such as "make the application log visible to the agent". It works on the same repository and in the same kind of clone: `axlr-tui --improve "<improvement brief or #issue>"` clones `repair.repository` beside the repair clones, writes `.axlr-repair.json` with `"kind": "improve"` and opens the console there in improve mode with the brief in the composer. `/improve` refuses a workspace without that marker, `/repair` refuses an improvement clone and `/improve` a repair clone, each printing the launcher command.

`brief` takes `criteria`, `scope` and `check_command`. The console refuses the brief while `git status --porcelain` shows any change in the clone, then runs the command once, after the person approves it, and accepts the brief only when it exits non-zero: the check must show that the improvement is missing, so the pull request can later show the same command failing before the change and passing after it. A check that already passes is sent back with that reason, at most three briefs. When the improvement already exists or cannot be made safely as a small change, the model sends `feasible=false` with `observed` and the ceremony ends `BLOCKED` with the reason. `build` makes the smallest change, returns `summary` and `summary_en` and must make the approved check exit zero, at most three attempts.

From there the console does what it does for a repair: it commits on `improve/<slug>` with the trailer `Improved-by: AXLR axlr_improve 1.0 <instance>`, opens the pull request titled `Improve: <first line of the brief>` with the criteria, scope, summary and the before/after evidence, watches its checks and brings a red round back to `build` at most twice. The merge is always the person's: `axlr_improve` 1.0 has no automatic transition and `repair.auto_merge` does not apply, so green checks open the merge card (`/improve` reopens it), `a` approves and merges with squash and `d` declines with a reason and ends `BLOCKED` with the pull request open. At the terminal state the console records the outcome in the project about (`success_path` or `observation`, labelled `improvement`); the model may not write project memory in this mode, and an improvement session cannot request a self-repair. A watch interrupted by cancellation, a crash or a reload is resumed as a repair's is. The [design](plans/2026-10-08-self-improve-design.md) records the decisions.

### Self-improvement from a running session

The agent of a normal, review, writer, research, debug, delivery or incident session can ask the console to improve AXLR with `axlr_request_improvement` when AXLR's own tools made its task needlessly hard. It is on by default whenever MADE is connected, like self-repair, and needs no card of its own: the improvement's check command and its merge keep the person's approval. The request takes the same fields as `axlr_request_repair` and cites at least two calls of the session. The console cannot judge whether a result was awkward, so it validates what the transcript shows: every cited call exists, was resolved, belongs to `local_*` or `axlr_*` (a plugin's tools are that server's to improve), and was neither denied nor cancelled; a program that exited non-zero counts, since working around a missing capability often looks like that. A text that names a provider, credential, permission or network cause is refused. The console then bounds how often it acts: one self-repair or improvement runs at a time, an origin session may start one improvement, a console build at most three, a running or merged improvement with the same repository, tools and first words of the description is a duplicate, and repair, improvement, plan and task sessions and clones cannot ask. An accepted request clones the repository with the improvement marker, starts a separate session in `/improve` with the brief assembled from the request and the cited calls, and drives it in the background exactly as a repair session is driven: the check command and the merge wait on the panel (`/improve` or `/repair` opens it; rows say Repair or Improvement), `axlr_repair_status` reports both kinds, and the origin session's next prompt carries a visible `[AXLR] Self-improvement …` note with the outcome and the build it still runs.

### A step the model will not hand back

When the model ends its turn with the step still open, the console reminds it once with a visible `[AXLR]` message. If the answer held a tool call written as text (`<|tool_call>`, `<tool_call>`, `[TOOL_CALLS]`, `<|python_tag|>` and similar), the reminder says so: the server's parser failed to turn it into a call.

If the model ends its turn again, the console does not leave the session waiting. It cancels the MADE instance with the reason, records the outcome in KMP and returns the session to normal mode. It then adds one visible message naming the server's tool-call parser as the first suspect when the markup was there, and the model answers the user. A plan worker ends `BLOCKED` with that reason instead. Self-repair and self-improvement keep their own nudges. The trace records a `ceremony_stalled` stage.

A session restored with a step open and no turn running shows `step <name> open: send a message to continue, or /stop-ceremony` in the footer. Any message continues the step. `/stop-ceremony` (alias `/parar`) cancels the instance in MADE and returns to normal mode; nothing in the workspace is rolled back.

### Memory and recovery

With a registration named exactly `kmp`, AXLR attempts a wake before the procedure and an outcome write at its terminal state. It uses the exact about saved through `axlr_session`, with `ws:<session-id>` as fallback. The driver rechecks the selected about at completion, since startup may have established it after the first claim. Wake errors are disclosed without stopping MADE; an outcome write failure is reported as `not recorded`. Bounded recall retains refs and marks pending continuation; it is not a complete project-memory audit. Terminal outcomes are observations with stable idempotency keys. A `needs_review` response remains pending for writer inspection, rather than being acknowledged blindly. For other reusable decisions, follow the [KMP runbook](runbooks/kmp.md#use-memory-in-a-task).

In a standard ceremony the system prompt names the ceremony and nothing that changes while it runs, so a provider's prompt cache survives its steps. The current step travels in console messages: an `[AXLR] Ceremony <name>, current step: …` note appended to the person's prompt when the ceremony begins (with the KMP recall) and on every later prompt while it runs, the `instruction` of each `axlr_step_done` result (with the attempt and the approved check command) and the console's reminders. The latest one is in force. The planner's decompose step and compact steps keep their instruction in their own, shorter system prompt.

MADE remains the authority for claims and progress. On some interrupted completions/transitions, the driver inspects the instance and reconciles using MADE's enabled transitions and claimable steps. Multiple enabled transitions or an unresolved active claim require operator inspection. Reloading a session does not itself resume a turn or prove that a check had no effect. See [recovery](runbooks/recovery.md#recover-a-session-or-ceremony).

### The compact profile for small models

`/debug` and `/delivery` run under one of two profiles. `ceremonies.profile` in `settings.json` selects it: `auto` (the default) uses the compact profile when the session model's known window is 65,536 tokens or less, from a [local model's](console.md#local-models) `context_tokens` or the global `context_tokens` cap; `standard` and `compact` force one. The profile is decided when the ceremony begins and kept in its `.ceremony` record, so a resumed session keeps it. `/incident`, `/repair` and `/improve` always use the standard profile.

Under the compact profile:

| What | Standard | Compact |
|:--|:--|:--|
| Tool calls per step | 32 | 16 |
| Model context | the window-derived budget | the smaller of that and 80 KiB ceiling, 56 KiB low watermark, 8 KiB per tool result, 4 KiB checkpoint |
| Tools | four local tools and every host tool | the local tools (without `local_write` and `local_edit` in `reproduce`, `diagnose` and `brief`), `axlr_step_done`, `axlr_history`, and `axlr_judge` when Jev is on; a hidden tool called anyway is refused |
| `axlr_step_done` | one schema with every ceremony's fields | the current step's fields only |
| Instructions | a description of the fields | under 500 bytes, leading with one exact example call; the guidance drops the plugin, history and self-repair paragraphs |
| A malformed hand-back | refused | fields the step does not take are dropped and named in the reply (`ignored_fields`); a `check_command`, or its `args`, sent as one string is split on spaces unless it holds shell syntax, and still reaches the approval card |
| The same call twice in a row | runs | refused with "same call as before; change something" |
| A malformed `local_exec` (also in plan workers) | runs as sent | one layer of wrapping quotes (`` ` ``, `«»`, `“”`, `‘’`, `<|"|>`) is removed from the program and each argument, and a program holding spaces without shell syntax is split; seen with Gemma 4 on vLLM |
| `red` of a plan task | — | `local_write` and `local_edit` reach only test files (a base name containing `test` or `spec`); the code changes in `green` |
| Between steps | the transcript continues | the next step starts from a ledger: the request that began the ceremony, one line per accepted step with the transcript messages it spanned, then the hand-back that opened the step; the saved transcript keeps everything and `axlr_history` reads any message |
| Memory recall / check output shown | 2 KiB / the full tail | 1 KiB / the last 2 KiB; MADE keeps the full evidence |

## Why there is no bundled skill catalogue

Until 5 Oct 2026 the console embedded a `made:axlr-ceremonies` skill with seven 1.0 definitions (`axlr_change`, `axlr_delivery`, `axlr_debug`, `axlr_review`, `axlr_research`, `axlr_publish`, `axlr_handoff`) for the model to drive itself. Measured on 1 Oct 2026 with a real model, the skill-guided path spent its whole turn on protocol (55 calls, context exhausted, code left broken) where direct work took 11 calls; publication had no approver and handoff no receiver. Review and research became [work modes](console.md#work-modes); debug and delivery became console-driven 2.0 definitions; change and handoff were dropped; the catalogue itself was retired. The [research notes](plans/2026-10-01-ceremony-driver-design.md) record the measurements and the decision.

Roles in a definition describe responsibilities and do not spawn agents. Independent review requires a distinct context or person: the incident ceremony uses a fresh-context reviewer and a person's approval, the repair ceremony a person's merge decision. MADE records caller-declared actor/role provenance, so guard approval and grant administration stay off the work identity; a role name alone does not enforce separation.

## Plans: atomic tasks for small models

`/plan`, then the brief as the next prompt (`/planificar` is an alias), runs three pinned definitions. Small models take small tasks well when the console builds their context and owns every verdict.

1. **Decompose.** The planner splits the brief into 1 to 12 tasks:
   - Each task has an id, a goal, a scope of 1 to 8 paths, up to 8 citations `{path, line, quote}`, a unit check, dependencies, test-first and protected files.
   - The plan as a whole has an end-to-end check, shared interfaces and an English summary.
   - The planner is `plan.model`: by default the session's model (`session`). Until 9 October 2026 it was `z-ai/glm-5.3-flash`, whose decompose request took 138 s when OpenRouter routed it to its slowest provider; a session on a small local model names a larger planner here. Without `OPENROUTER_API_KEY` a remote planner falls back to the session model, and the console says so at launch.
   - The commands the hand-back names go through the approval card, under autonomy too, because the console runs each one once while verifying.
2. **Verify.** The console checks the proposal without a model. Every defect goes back to the planner with the task id; three unverified rounds end the plan `BLOCKED`. It checks:
   - slug ids, unique;
   - known, acyclic dependencies, from which it computes the waves;
   - paths inside the workspace;
   - disjoint scopes within a wave;
   - each citation's quote, at least 12 characters, present on its line with whitespace collapsed;
   - commands that run, with their exit recorded as the baseline;
   - a context pack of at most 12 KiB per task: the task, the scope with new files marked, the interfaces, the check, the protected files, and the cited regions with 20 lines of margin.
3. **Approve.** The plan card shows the task table:
   - `a` approves through the separate approver identity and covers the plan's check commands;
   - `d` sends it back with a reason, twice at most;
   - `x` declines it with a reason.

   `plan.auto_approve` records an automatic approval instead. After the person approves, the plan's session asks the model nothing more, because a planner that kept working would edit the workspace under the workers. After an automatic approval, the model is told to answer in a sentence and stop.
4. **Tasks.** The approved plan runs in the background, wave by wave. The tasks of a wave run one after another in the same workspace, each in a fresh `task` session with the session's model and autonomous local tools. Its first message is the context pack, plus the notes earlier tasks addressed to it and what KMP recalls for its plan and task.
   - **`start`** (console): records the digests of the scope, protected and already-changed files, and runs the unit check as a baseline.
   - **`red`** (test-first only): the named test files must have changed and the check must fail; their digests are then frozen.
   - **`green`:** the frozen and protected files must be intact, every change must be inside the scope (`git status`; without Git only the digests), and the check must pass.
   - **`handback`** (console): records the changed files and the revision.

   The worker's notes (up to 4, at most 500 characters, to a task or `all`) and questions (up to 2) are kept for later tasks and the person. A task blocked by its rounds, by a call needing the person, or by a second reply without a tool call ends `BLOCKED`, and the tasks that depend on it are skipped.
5. **Sync.** After a wave whose tasks all finished, the console runs the end-to-end check.
   - Green records the integration.
   - Red opens a reconciliation round. Each affected task gets a fresh worker with its own hand-back, the others' hand-backs, the notes for it and the failing tail. It may change only its scope, or leave a `NOTE <task>: text` line.
   - Two red rounds end the sync `BLOCKED`.

`/plan` shows the plans panel while a plan runs or is interrupted. `r` runs an interrupted plan again, and `n` starts a new one. The footer shows `plan <id> · wave 2/3 · <task> <step>`. The registry is `<state>/axlr/plans.json`; nothing is written into the workspace except the tasks' work.

The plan, each hand-back and each sync are written to KMP with `plan`, `task`, `wave`, `session` and `ws` labels; the model never writes memory in these modes. The [design](plans/2026-10-06-local-27b-ceremonies.md) records the contracts, the decisions taken and the measurements; the [research record](research/2026-10-06-local-27b-agents.md) holds the evidence.

## Source and validation

The [driver](../tui/application/ceremony_driver.go), [definition pins](../tui/adapters/ceremonyhost/definitions.go) and [preparer](../tui/adapters/madesetup/preparer.go) define console behavior. Run the module checks without invoking a model or touching a live ceremony store:

```bash
GOWORK=off go -C tui test ./application ./adapters/ceremonyhost ./adapters/madesetup
```

To validate the shipped definitions and their pins against a compatible MADE binary in a disposable store, as CI's `ceremonies` job does:

```bash
python3 tools/ceremonies/check_pins.py --made-bin /absolute/path/to/made-mcp
```

Each YAML is validated and published into the disposable store; its semantic digest must match the pin in `definitions.go`, and every pin must have a YAML. Drafts, when there are any, are validated and published the same way without a pin, and `python3 tools/ceremonies/spike_drafts.py --made-bin …` walks the happy and blocked paths of the plan, task, sync and improve definitions. Bump the immutable published version when a definition's content changes.
