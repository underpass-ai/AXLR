# Engine decoupling: the console's lifecycle behind ports

Date: 13 October 2026. Status: decided and applied to the session lifecycle of [PR #117](https://github.com/underpass-ai/AXLR/pull/117) on 14 October 2026; the earlier ceremony driver keeps its engine names until the follow-up listed at the end.

## Why

The session lifecycle (the [design of 13 October](2026-10-13-lifecycle-integration-design.md)) made the console run two engine protocols itself: the memory's agent and context, the orchestrator's instance and turns. The first implementation wrote those protocols into `tui/application` and `tui/domain` with the engines' names: `KMPIdentity`, `SessionInstance.KMPContext`, `MADEPreparationPort`, a switch on the plugin id `"kmp"` to fill `context_id`, a list of `made_*` verbs the model may not call, the `kmp_write_memory` name in the memory reminder, and the KMP and MADE paragraphs of the model's orientation. The review of the PR asked for the architecture the repository already has: ports and adapters, so AXLR can run with another memory or another orchestrator by writing an adapter, not by editing use cases.

## What the application knows

The application knows two roles and nothing about who plays them:

- the **memory**: a `MemoryIdentity` (store, registration key, agent registry, guide, recall, the person's about), a `MemoryAgentsPort`, the `MemoryPort`, `JournalPort`, `RecallPort` and `MemoryIdentityPort` that already existed, and the session's `MemoryAgent` and `MemoryContext` in `domain.SessionInstance`;
- the **orchestrator**: the `CeremonyEnginePort` that already existed and the `OrchestratorPreparationPort` (grants, work identity, published definitions).

Every plugin tool in the session's snapshot carries a `domain.ToolSurface`, decided by the engine adapter when the tool is registered (`ceremonyhost.Surface`, wired into the tool catalog): the plugin's role, the refusal when the console keeps the tool to itself (`Hidden`), the argument the console fills with the session's memory context (`ContextArgument`), whether a call is a memory write (`MemoryWrite`) and whether the name belongs in the model's orientation (`Entry`). The bridge, `axlr_tools`, the model context, the memory reminder, the turn record and the tool events read the surface and never a plugin id or a tool name. The snapshot is frozen per turn and persisted with the session, so an old session keeps the surfaces it ran with.

The model's orientation for a plugin comes from `EngineGuidancePort` (`ceremonyhost.Guidance`): the application passes the plugin id and two facts (does the model record memory in this mode, does it have `axlr_remember`) and appends the paragraph it gets back. The size bound (under 1 KB with the session line) moved with the texts.

Error and log lines of the application say "memory" and "orchestrator" ("memory is not connected in this console", "memory context opened for session …", "the orchestrator is not connected"); evidence lines stored in memory say "ceremony <instance> step turn #n" and "ceremony <instance> closed".

## What stays engine-specific, and where

- `tui/adapters/kmpstore`: the KMP store of a workspace (selection, preparation, guide, count, export).
- `tui/adapters/madesetup`: MADE's grants and identities.
- `tui/adapters/ceremonyhost`: the engine calls (`made_*`, `kmp_*`), the protocol table (`surface.go`) and the orientation texts (`guidance.go`).
- `tui/adapters/storage`: the sidecar keys `kmp_agent` and `kmp_context` stay as written, so the sessions on a person's machine load unchanged; the domain fields they map to are `MemoryAgent` and `MemoryContext`.
- `tui/adapters/terminal`: what the person reads may name the product (the `/mcp` panel, the MADE preparation overlay).

## The rest of the review

- **One type per file.** Every struct and interface of the files this branch changes lives in its own file named after it (`ceremony_ports.go` became twenty-one files and is gone); the moves are separate commits that only move text. Files the branch does not touch and that already held several types (about fifty-four, such as `storage/plan_registry.go` and `application/repair_ports.go`) are left for their own change.
- **SOLID in `kmpstore`.** The store's `Preparer` prepared, synced the guide, counted and exported; it is now a `CLI` runner and four types with one job each (`Preparer`, `GuideSync`, `Counter`, `Exporter`).
- **Values instead of strings.** The memory engine's agent and context are `domain.MemoryAgentID` and `domain.MemoryContextID`, validated where the adapter reads them from the engine and carried typed through the ports (`GuideResult`, `TurnEntry`, `RememberRequest`, `RecallRequest`, `CompareRequest`, `ForgetRequest`, `SettledEntry`); a session instance's predecessor is a `SessionID`. The sidecar keys on disk (`kmp_agent`, `kmp_context`, `predecessor`) are unchanged, and a test reads a sidecar written before.

## Follow-up (not in PR #117)

The ceremony driver that predates the lifecycle (`ceremony_driver.go`, `ceremony_repair.go`, `ceremony_incident.go`, `ceremony_plan.go`, `ceremony_sync.go`, `self_repair.go`, `model_context.go`'s mode texts, `domain/work_mode_policy.go`) names `made` and `kmp` in about a hundred lines on `main`: the plugin ids it calls through the execution port, the mode texts that say "a MADE debug ceremony", and the partial-recall prefix. The same move applies: the plugin ids behind the `CeremonyEnginePort` and `MemoryPort` the adapter already implements, the texts behind `EngineGuidancePort`, and `domain.WorkMode`'s judgement of `made_*` verbs behind the tool surface. The preparation error moved to `application/not_prepared_error.go` ("prepare MADE first: open /mcp …"), the writer mode's "When KMP is connected" and the drivers' "not recorded: KMP is not connected" belong to the same follow-up. It is a separate PR because it touches six modes the lifecycle PR did not change.
