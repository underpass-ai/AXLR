# Self-improvement (`/improve`) design

Status: implemented as the launcher path (8 Oct 2026). Scope chosen by Tirso: the sibling of [self-repair](2026-10-05-self-repair-design.md) for changes that are not failures. Decisions below were taken with the advisor. YAML spiked against MADE 0.10.0.
Builds on: [self-repair](2026-10-05-self-repair-design.md), which already owns the clone, the forge steps, the merge card and the memory writes. The console drives MADE; the model does the work; the console does the Git/GitHub side itself.

## Goal

Let AXLR improve itself the way it already repairs itself: clone the repository into a disposable directory, settle a check that shows the improvement is missing, make the change there until the check passes, open the pull request, watch its checks and merge it only when the person approves, and leave the outcome in project memory. Repair starts from a failure; improvement starts from a brief or an issue such as [#75](https://github.com/underpass-ai/AXLR/issues/75) or [#76](https://github.com/underpass-ai/AXLR/issues/76).

## Decisions (8 Oct 2026)

- **Same pipeline, new front.** `axlr-tui --improve "<brief or #issue>"` prepares the clone exactly like `--repair` (same preparer, same directory, same `repair.repository`, `repair.about`, `repair.directory` and `repair.watch_minutes`) and opens the console rooted in it, in `/improve` mode, with the brief in the composer. The marker `.axlr-repair.json` gains `kind: "improve"`; `/improve` refuses a workspace whose marker is not an improvement's, and `/repair` refuses one that is, so one clone never carries two ceremonies' branches.
- **The baseline must fail.** Repair proves the defect by reproducing it; an improvement proves it is missing. `brief` takes `criteria`, `scope` and `check_command`; the console runs the command once in the untouched clone and accepts the brief only when it exits non-zero. A check that already passes is sent back with that reason (at most three briefs, then `BLOCKED`). The pull request can then say that the same command failed before the change and passed after it, which is the only evidence a reviewer needs to trust a change nobody typed. When the improvement already exists, or cannot be done safely as a small change, the model sends `feasible=false` with `observed` and the ceremony ends `BLOCKED` with the reason, as `reproducible=false` does for a repair.
- **Build reuses delivery's step.** `build` is delivery's step with repair's contract: the smallest change that meets the criteria, `summary` and `summary_en`, at most three attempts; the console reruns the approved check and must see exit zero. A red check round on the pull request returns to `build` with the failing checks named, at most twice (`domain.MaxRepairRounds`), as in repair.
- **No automatic merge, by definition.** An improvement changes the product rather than restoring it, so its taste is the person's. `axlr_improve` 1.0 has no `merge_automatic` transition at all: `DECIDE` leads to `MERGE` only through `merge_approved` with the human guard `person_approves`. `repair.auto_merge` does not apply. MADE therefore records that every merged improvement was approved by the person.
- **Attribution.** The branch is `improve/<slug>`, the title `Improve: <first line of the brief>`, the commit trailer `Improved-by: AXLR axlr_improve 1.0 <instance>` and the body names the instance, the criteria, the scope, the summary and the before/after check.
- **Memory.** Wake at `Begin` on the project about with the brief as `intent`, and on `ws:<session>`, as repair does. One write at the terminal state: `success_path` with the pull request and merge revision, or `observation` with the reason it stopped, labelled `ceremony`, `improvement`, `repository`, `pull_request`, `session` and `ws`. The model may not call `kmp_write_memory` in improve mode.
- **Reuse over a parallel type.** The improvement travels in `domain.RepairRun` with `Improvement=true` and its `Criteria` and `Scope`, so propose, watch, decide, merge, resume and reconciliation are the repair code paths, not copies.

## Definition `axlr_improve` 1.0

`BRIEF` (repeat ≤ 3) → `BUILD` (repeat ≤ 3) → `PROPOSE` → `WATCH` → `DECIDE` → `MERGE` → `COMPLETED`; `WATCH` → `BUILD` on `checks_failed`; `BLOCKED` from `not_feasible`, `brief_exhausted`, `build_exhausted`, `propose_failed`, `watch_blocked`, `merge_declined` and `merge_failed`.

Inputs: `improvement_brief`, `workspace`, `repository`; optional `memory_about`, `issue`.

Spike (`tools/ceremonies/spike_drafts.py`, disposable store, MADE 0.10.0): happy path through the approved merge; a brief whose check already passes, then a failing one; `feasible=false` → `BLOCKED`; briefs exhausted; red → build → red → build within `max_bounces`; `approve` without the guard enables nothing, and `merge_automatic` does not exist; `decline` → `BLOCKED`.

## Who does what

| Step | Done by | Console checks / does |
|:--|:--|:--|
| `brief` | model via `axlr_step_done{criteria, scope, check_command}` or `feasible=false` | runs the command once (approval once): it must exit non-zero |
| `build` | model via `{summary, summary_en}` | reruns the approved check; must exit zero; on a later round the CI tail is in the instruction |
| `propose` | console | as repair, on `improve/<slug>` with `Improved-by:` |
| `watch` | console | as repair |
| `decide` | console | always the card: `a` approve (+guard), `d` decline |
| `merge` | console | as repair; writes the outcome to memory |

## Boundaries

- Everything repair's boundaries say holds: `gh` and `git` from the person's configuration, the default branch as base, nothing force-pushed, one ceremony per clone.
- `axlr_request_repair` is hidden in improve mode and refused there: an improvement clone is AXLR's own work in progress.
- Improve uses the standard profile, never the compact one, like repair and incident.

## Out of scope, next

- **The agent asks for an improvement** (`axlr_request_improvement`), the sibling of `axlr_request_repair`. It needs a stricter bar than repair, not a looser one: a friction has no failure class to validate against the transcript. Candidate rules: cited calls that succeeded with a wrong or awkward result, at least two of them; a dedup key from the normalised brief and touched paths; one active improvement per console and a small cap per build; never from an improvement or repair clone.
- `connect_to` links from the brief to what memory already knows.
- Building and installing the improved console ([#75](https://github.com/underpass-ai/AXLR/issues/75)) applies to both kinds once it exists.
