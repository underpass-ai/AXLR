# Execution contract

## Prepare and resume

Recover opted-in project memory before re-deriving context. Inspect repository instructions, user constraints, worktree and current revision. Fill required inputs with real task data; do not use placeholders. Acceptance criteria must describe observable behavior. Use the current instance ID from a checkpoint; if lost, list/search and inspect candidates before creating a new one.

The packaged catalogue is a recommended default, not an automatic migration of a user's MADE store. Read the live definition. Compare `definition_digest` with `catalog.json`. Do not execute a different definition merely because its name matches. An existing `1.0` with different content is a conflict requiring a new version/name and explicit review. Tool approval and MADE authorization are separate from ceremony role permissions.

For an authorized catalogue install, validate each exact YAML with `made_validate_ceremony_draft`, inspect the analysis, then publish it with `made_publish_ceremony_definition`. Publication is immutable and does not start work. Delivery and debug intentionally warn that exhausted repeats stop in their current state: this preserves a failed attempt for diagnosis instead of declaring it completed. Require no other unexplained findings.

## Claim, execute, complete

Use current discovered schemas instead of copying stale request shapes. Preserve all claim identities returned by `made_claim_ceremony_step`: instance, step, role, logical worker, host identity, lease owner, attempt, execution ID and `claim_fence`, as applicable. When a tool accepts an idempotency key, keep one per logical operation and reuse it after an uncertain response. Completion retains its exact claim fence; do not add fields absent from the live schema. Renew a long-running lease through the advertised API.

Execute `config.prompt` using real authorized host capabilities. `host_callback` is a responsibility for AXLR, not an installed implementation. Do not use `made_run_ceremony` or `made_run_ceremony_step` with an unverified server handler: the embedded default can be a no-op. A delegated claim followed by real work and `made_complete_ceremony_step` is the default path.

Every result identifies the exact artifact or revision, observable checks, limitations and evidence/artifact references. Put the named success field at the top level with a JSON boolean (`true`, not the string `"true"`). Keep secrets and private reasoning out of results. Definition output guards validate these fields; they do not independently prove a test, citation or reviewer identity. The host must supply honest evidence.

Refresh with `made_get_ceremony_instance`; verify persisted step status/output, then apply the enabled transition using its actual trigger. Never infer eligibility from a role name, a successful request or a pending step. Technical retry is capped at one attempt; semantic delivery/repair repeats are capped at three complete iterations. A tool timeout or connection loss does not prove an external action failed.

Report progress from actual work and persisted state. If reporting live activity, preserve separate event/activity cursors and actual host incarnation. Missing telemetry is unknown; a lease does not prove liveness. At completion report the concrete outcome, checks and limits in the user's language. Write decisions/outcomes to KMP only when opted in and with evidence, not a transcript.

## Publication and human decisions

`axlr_publish` has three phases: prepare an exact packet; cross a genuine human guard; publish and verify. The definition assigns `approve_publication` to `HUMAN_APPROVER`. `INTEGRATOR` owns preparation, execution and readback, and must not approve for a person. Existing explicit user authorization is preserved as evidence; record a guard decision only through a discovered, authorized human mechanism. A caller-supplied role or `actor_kind: human` does not prove a person decided. If the host cannot record the genuine decision, keep the ready packet pending and explain that boundary.

The work-agent principal must not hold `approve_ceremony_guard`, `issue_authorization_grant`, or `revoke_authorization_grant`. Give approval to a separate trusted human/operator channel, and give catalogue installation grants to a separate setup operation. MADE 0.9.1 records caller-declared role provenance: a privileged caller with `approve_ceremony_guard` can set a human guard even when it declares an agent/integrator role. YAML role labels alone are therefore not an enforcement boundary. Before relying on the human gate, inspect effective grants; if the work agent can approve its own gate, surface the configuration issue and keep publication pending. Do not silently alter the user's policy.

No routine human gate is added to changes, delivery, debugging, reviews or research. For publication, verify the artifact digest, destination and scope again immediately before the operation. Any change invalidates the prior approval. Inspect operation receipts before retrying ambiguous side effects. Only verified readback permits `confirmed=true` and a publication success report.

## Capability limits

Default definitions are sequential with one concurrent step. Additional agents require explicit host support and authorization. Record each role's host execution profile outside the YAML: requested model/effort/capabilities, inheritance source, permitted fallback, and actual host agent/incarnation/model if known. Refuse unsupported requests or use only an authorized fallback; do not claim a requested profile was achieved without host evidence.

When a task needs dependent ceremonies or standing roles, compose pinned name/version/digest definitions into a MADE agentic system rather than enlarging one ceremony. These defaults are a routing catalogue, not seven dependent procedures that should always run together. Preserve parent/child/successor relationships using actual supported APIs. No procedure authorizes external messages, deployment or destructive operations by itself.
