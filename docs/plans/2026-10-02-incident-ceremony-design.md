# Incident Review Ceremony (`/incident`) Design

Status: design with Tirso's decisions (2 Oct 2026); YAML spiked against MADE 0.9.1.
Builds on: [ceremony driver](2026-10-01-ceremony-driver-design.md). The console drives MADE and the model does the work.

## Decisions (Tirso, 2 Oct 2026)

- **Where:** an AXLR mode, `/incident` (alias `/incidente`), driven by the console like `/debug`.
- **Who:**
  - The agent investigates and writes.
  - A reviewer in a **fresh context** checks the draft, up to two rounds. It is a second model generation with no tools and no transcript, using a configurable `reviewer_model` in `settings.json` (default: the session model). Its independence is of context, not of identity, and it is labelled so.
  - A **person** approves through a real MADE human guard.
- **Output:**
  - a blameless postmortem;
  - actions with owner, due date and verification;
  - a KMP record in `ws:<session id>` with labels `incident:<slug>`, `service:<service>` and `severity:<sev>`.
- **Returning:** `d` on the approval card sends the draft back to review with a typed reason. At most two returns, a limit the console enforces (see the bounce note below).

## Definition `axlr_incident` 1.0

`TRIAGE` → `TIMELINE` → `ANALYSIS` → `REVIEW` (state repeat ≤ 2: `revise`, `review`; exhausted → `BLOCKED`) → `APPROVAL` (`present`) → `PUBLISH` → `COMPLETED`. `APPROVAL` → `REVIEW` on `returned`.

- **`approved`** needs `present` completed and the human guard `person_approves`. Only the approver identity can grant that guard.
- **`returned`** needs `present` completed with `decision="return"`, an automated guard on this visit's output. It is not a second human guard, because MADE 0.9.1 keeps a human-guard approval for the instance across state visits. Spike E: a second return passed with no new approval. An output-field guard only reads the current visit, so neither `reconcile` nor anything else can fabricate a return.
- **`max_bounces: 3`** is the engine's backstop. Each edge counts separately, and `reviewed` is used once more than `returned`, so with 2 the ceremony stuck in `REVIEW` after two returns. The console therefore offers `d` at most twice.

Spike results (disposable store), all as expected:
- the happy path with a 19.6 KB draft in a step output;
- review rejected once, then accepted;
- review exhausted → `BLOCKED`;
- approval refused until the guard is granted;
- return, re-review and a fresh approval;
- two returns, then approval;
- exhaustion after a return → `BLOCKED`.

Pinned digest: `f81e9aa53f7d77c72086ac3baec1a4037df1d0fa3d24e62270e5b62fabe6f6df`.

## Who does what

| Step | Done by | Console checks / does |
|:--|:--|:--|
| `triage` | model via `axlr_step_done{summary, impact, severity, detection, service, slug}` | all present; `severity` is one of `sev1..sev4`; `slug` is kebab-case |
| `timeline` | model `{timeline:[{at, event, evidence}]}` | every entry has an RFC3339 time and evidence; times ordered; evidence that looks like a workspace path exists |
| `analysis` | model `{root_cause, contributing_factors[], went_well[], went_badly[], actions:[{title, kind, owner, due, verification}]}` | `kind` is corrective or preventive; every action has an owner, a future ISO date and a verification |
| `revise` | model writes `docs/incidents/<slug>.draft.md`, returns `{draft_path}` | reads the bytes and digest into the `revise` output, then runs `review` itself |
| `review` | **console**: reviewer generation, tool `review_verdict{accepted, findings[]}` | an unparseable verdict gets one retry, then the `step_done` is refused without touching MADE; output carries `review_mode: independent_context` and `reviewer_model` |
| `present` | **console**, on the person's keypress | `a`: the approver identity grants `person_approves`, the work identity completes `present{decision: approve, draft_digest}`, applies `approved`, writes the approved bytes to `docs/incidents/<date>-<slug>.md`, claims `publish`. `d` + reason: completes `present{decision: return, reason}`, applies `returned`, claims `revise` with the reason |
| `publish` | model `{report, summary_en}` | the file holds the approved digest; KMP record with the incident labels; `published=true` |

The engine cannot verify production facts. The guarantees are structural checks, a fresh-context review and the human gate.

## Layer map

| Layer | Change |
|:--|:--|
| domain | `ModeIncident` (`Judge` → Allow, `StartsCeremony`). `CeremonyRun` gains `Awaiting` ("" or `approval`), `DraftDigest`, `ReturnReason`, `Returns`. These persist in the `.ceremony` sidecar, which is read leniently. |
| application | `ceremonySpecs` and `stateSteps` gain the incident; `stepDone` adds the triage/timeline/analysis fields and `draft_path`, still strict-decoded. **`CeremonyReviewerPort`** `Review(ctx, draft, reason) (Verdict, error)`. **`ApproverPort`** `ApproveGuard(ctx, instance, guard) error`. The driver gains `Approve(ctx, s)` and `Return(ctx, s, reason)` for the keypresses (no tool reaches them), and `remindOpenStep` stays silent while `Awaiting`. The step reminder is reused for the turn after a keypress. |
| adapters | `ceremonyhost`: the reviewer over `ModelStreamPort` with the review tool only; the approver that spawns the MADE command as `<work identity>-approver` (the P pattern); the third embedded YAML with its pinned digest. `madesetup`: P issues the approver grant (`approve_ceremony_guard`, `get_ceremony_instance`) and publishes `axlr_incident`. `storage`: `reviewer_model` in `UserSettings`, preserving unknown keys. |
| terminal | `/incident` + `/incidente`; footer `incidente · <step>`, `incidente · revisando…` and `incidente · aprobación pendiente`. The **approval card is not the tool card**: an Info overlay rendering the draft Markdown, with instance and digest, and `a aprobar · d devolver · esc`. `a` acts as the approver identity. `d` asks for a reason. |

## Limits to state in the PR

- Reviewer independence is of context, not of identity. A review costs one extra model call per round, roughly 30–60 s.
- Host-initiated MADE, KMP and approver calls bypass the plugin approval policy, as before.
- `present` can wait hours for the person. Completing after the lease expires works while nobody else claims the step (probed on 1 Oct).
- Human-guard approvals persist for the instance in MADE 0.9.1. The design does not depend on it; whether to raise it in MADE is Tirso's call.
- P must be pressed again after reinstalling: it adds the approver grant and the third definition.
