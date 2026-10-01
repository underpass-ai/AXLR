# Handoff

Use `axlr_handoff` when ongoing responsibility actually moves to another participant or host. Context compaction on the same worker normally needs a checkpoint and resume, not a fictitious receiver. Subtask delegation alone is not ownership transfer; retain the parent owner unless an explicit handoff is intended.

## Packet

Save an attributable artifact with a digest. Include:

- Goal, acceptance criteria, user constraints, authorization scope and pending human decisions.
- Repository, absolute worktree path, branch, commit, dirty files, diff and artifact references. Distinguish committed and uncommitted work.
- Decisions with supporting evidence, completed work, pending work, failed attempts, blockers and the exact next action.
- Checks with command, status, revision, evidence and remaining uncertainty. Never call a stale check current.
- MADE source instance, pinned definition name/version/digest, current state, completed steps, active claims/fences/lease expiry, operation receipts, interventions and event/activity cursors. Capture `made_inspect_ceremony_resume` when available.
- KMP about, agent/context identity and recall frontier when opted in. Copy actual identities; do not synthesize them.
- Source and intended receiver host identities; requested and actual execution profiles, inheritance and fallback. Mark unavailable telemetry `unknown`.

Do not copy secrets or private reasoning. References must be readable by the receiver; a sender-only path is a missing handoff dependency. Preserve the original instance and external operation IDs. Preparing the packet does not transfer a lease or authorize a duplicate execution.

## Acceptance and transfer

The real receiver reads the exact packet, verifies its digest and current workspace revision, checks accessible evidence, and inspects resume state. It acknowledges its actual identity, outstanding uncertainties, accepted next action and ownership. Set `accepted=true` only for that acknowledgment. With no receiver or an unreadable artifact, keep `accepted=false`; the sender retains responsibility or explicitly pauses it.

After acceptance, the integrator handles active claims according to the live engine's recovery contract, records the supported host handoff/rebinding, and reads back the persisted transfer receipt. A host handoff can require a paused source ceremony; inspect its preconditions before changing state. Never reuse stale fences, infer cancellation from a lost connection or repeat an uncertain external operation. Set `transferred=true` only when one continuation owner and the durable receipt are confirmed.

In AXLR without delegation/host-transfer capability, prepare the packet and leave the ceremony pending at acceptance. A logical `RECEIVER` label on the sender, a generated summary or an unacknowledged message does not complete the handoff. Sending the packet to another task still requires authorization to message it.
