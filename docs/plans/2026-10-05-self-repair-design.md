# Self-repair (`/repair`) design

Status: design with the night mandate of 4→5 Oct 2026 (Tirso absent; decisions taken with the advisor and listed below). YAML spiked against MADE 0.10.0.
Builds on: [ceremony driver](2026-10-01-ceremony-driver-design.md) and [incident](2026-10-02-incident-ceremony-design.md). The console drives MADE; the model does the work; the console does the Git/GitHub side itself.

## Goal

Give the agent the capability to repair AXLR: clone the repository into a repairs directory, reproduce and fix the failure there, open the pull request, watch its checks, merge it when they are green and the merge is approved or automatic, and leave the cause and the outcome in project memory with their relations.

## Decisions (night of 4→5 Oct 2026)

- **Where the repair happens.** The console is single-root: the runtime anchors files to the workspace chosen at launch and a saved session is bound to that workspace. A repair therefore starts from the launcher: `axlr-tui --repair "<brief or #issue>"` clones the repository into `<data>/axlr/repairs/<date>-<slug>/`, writes `.axlr-repair.json` there, and opens the console rooted in that clone with the session already in `/repair` mode and the brief in the composer. `/repair` typed in a workspace without that marker refuses the start and prints that command. Re-rooting a running console was judged too large for one night; it is the natural v2.
- **Target repository.** Only AXLR for now (`underpass-ai/AXLR`, dogfooding). `settings.json` → `repair.repository` names it; `repair.directory` overrides the repairs directory; `repair.auto_merge` (default `false`) lets the console merge a green pull request without the person; `repair.watch_minutes` (default 45) bounds the watch.
- **Merge is a recorded decision.** `DECIDE` has two ways into `MERGE`: `merge_automatic` (`output_field:decide:decision="automatic"`, which the console completes only when `auto_merge` is on) and `merge_approved` (`decision="approve"` plus the human guard `person_approves`, granted by the separate approver identity on the person's `a`). `d` records `decline` and the ceremony ends `BLOCKED` with the pull request left open. MADE therefore records who merged: the setting or the person.
- **Check rounds.** `WATCH` red sends the ceremony back to `REPAIR` with the failing checks' output; the console allows **two** check rounds (`RepairRun.Rounds`), then records `blocked`. `max_bounces: 4` is the engine's backstop (`repaired` is used once more than `checks_failed`). A pull request that falls `BEHIND` (`strict` protection) is brought up to date once per round with `gh pr update-branch`; a stall past `watch_minutes`, a closed pull request or an unknown merge state records `blocked`.
- **No merge into AXLR without the person tonight.** The operator proof runs against the private lab repository `underpass-ai/axlr-repair-lab`. A run against AXLR itself keeps `auto_merge` off so the pull request waits for Tirso. Commits carry the trailer `Repaired-by: AXLR axlr_repair 1.0 <instance>` and the pull request body names the ceremony instance, so a console-made change is never mistaken for a hand-written one.
- **Memory.** Wake at `Begin` on the project about (`repair.about`, default `project:axlr`) **with `intent` = the brief**, which lets KMP's Jev focus the recall on the failure, and on `ws:<session>` for continuity. Two writes, not one: the cause when `diagnose` is accepted (`error_path`, worth keeping even if the repair fails) and the outcome at the terminal state (`success_path` with the pull request and merge revision, or `observation` for `BLOCKED` with the evidence). The model may pass `connect_to` in `diagnose` and `repair`: refs with `rel` and `why`; the console writes them only when the ref appeared in the captured wake, so no relation points at an invented memory. The model may not call `kmp_write_memory` in repair mode; the console records.
- **Zero model calls in the console steps.** `propose`, `watch`, `decide` and `merge` never reach the model (the glm lesson from 1 Oct: protocol cost kills weak models). They run inside the `axlr_step_done` call that accepted `repair`, so one tool call carries the ceremony from the passing check to the merge. A cancelled or crashed watch is resumed by the console before the next turn (`Resume`), from the pull request number saved in the sidecar.

## Definition `axlr_repair` 1.0

`REPRODUCE` (repeat ≤ 3) → `DIAGNOSE` → `REPAIR` (repeat ≤ 3) → `PROPOSE` → `WATCH` → `DECIDE` → `MERGE` → `COMPLETED`; `WATCH` → `REPAIR` on `checks_failed`; `BLOCKED` from reproduce exhaustion or non-reproducibility, repair exhaustion, `propose_failed`, `watch_blocked`, `merge_declined` and `merge_failed`.

Inputs: `failure_brief`, `workspace`, `repository`; optional `memory_about`, `issue`.

Spike (disposable store, MADE 0.10.0), all as expected: happy path with automatic merge; red → repair → red → repair → blocked within `max_bounces`; `approve` without the guard enables nothing, with the guard only `merge_approved`; `decline` → `BLOCKED`; reproduce exhausted → `BLOCKED`. Guard approval takes `role_id: HUMAN_APPROVER, role_kind: human`.

Pinned digest: `62264d45ed6cf59b18b1c9e4a6cd01285cf8e52b7b894f251cee8edc2e91161e`.

## Who does what

| Step | Done by | Console checks / does |
|:--|:--|:--|
| `reproduce` | model via `axlr_step_done{check_command, expected, observed}` or `reproducible=false` | as debug: runs the command, must exit non-zero; approval once |
| `diagnose` | model via `{root_cause, evidence, proposed_fix, connect_to?}` | writes the cause to the project about with the validated links |
| `repair` | model via `{summary, summary_en, connect_to?}` | reruns the fixed check; on a later round, the CI tail is in the instruction |
| `propose` | console | `git add -A`, commit with trailer, push `repair/<slug>`, `gh pr create` (or push again to the open pull request) |
| `watch` | console | polls `gh pr view --json state,mergeStateStatus,statusCheckRollup` every 30 s; `update-branch` when `BEHIND`; verdict `green`/`red`/`blocked` |
| `decide` | console | `automatic` under the setting, else the card: `a` approve (+guard), `d` decline |
| `merge` | console | `gh pr merge --squash --delete-branch`; records `merge_sha`; writes the outcome to memory |

## Boundaries

- The console's exec environment carries only an absolute `PATH` and `HOME`; `gh` and `git` take their configuration from `~/.config/gh` and `~/.gitconfig`. A missing `gh` login makes `propose` fail honestly (`proposed=false`).
- The pull request is opened against the repository's default branch; the branch is deleted on merge. Nothing is force-pushed.
- Repair mode limits no local tool (the check decides), but refuses `kmp_write_memory` and hides nothing else.
- `P` publishes the fourth definition like the other three; the approver grant already covers `person_approves`.

## Out of scope tonight

Re-rooting a running console; repairing repositories other than the configured one; reading GitHub issues as briefs beyond `#N` → `gh issue view`; rerunning flaky jobs automatically (a red round goes to the model with the failing checks).
