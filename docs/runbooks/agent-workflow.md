# Runbook: execute a task with AXLR, KMP and MADE

Use AXLR as the working entry point. KMP supplies prior evidence and durable decisions; MADE tracks a procedure when selected. This runbook applies to the console. The [product matrix](../product.md#entry-points-and-current-capabilities) explains other hosts.

## Prepare once

1. Build and launch with the [getting-started guide](../getting-started.md); select a tool-capable OpenRouter model.
2. Connect [KMP](kmp.md) to the intended store and verify a known memory. Choose a stable project scope for cross-session work.
3. Connect and [prepare MADE](made.md#prepare-the-driven-ceremonies), using a work identity and the exact pinned driver definitions. Restart if preparation changed the identity.
4. Inspect `/mcp` and `/approvals`. Decide which exact tools or servers may run automatically. A catalogue entry, connection and authorization are separate facts.

## Start the task

Give the objective, workspace scope and observable acceptance criteria. Include the project's existing KMP scope when known. For example:

> Recover the evidence for project:example. Update the operator documentation to match this checkout. Preserve the documented public interface. Finish with a reviewed diff, working examples and the checks you ran.

Choose the mode before sending the prompt:

| Task | Mode | Dependency |
|:--|:--|:--|
| General work or a small change | `/normal` | No ceremony required |
| Audit without editing | `/review` | Local file changes refused |
| Documentation or prose | `/writer` | Local changes limited to documents |
| Evidence and recommendations | `/research` | Memory first when connected; document writes only |
| Repair a reproducible failure | `/debug` | Prepared MADE `axlr_debug` 2.0 |
| Deliver a change against a fixed check | `/delivery` | Prepared MADE `axlr_delivery` 2.0 |
| Reconstruct and approve a postmortem | `/incident` | Prepared MADE `axlr_incident` 1.0 and console approver |

Modes can change only between turns without pending calls. Review/writer/research require approval for every local process and refuse nonempty process stdin; MCP policy is separate. Ordinary work does not need a ceremony simply because KMP and MADE are connected.

The built-in `axlr:axlr-session` skill establishes session context. The first prompt remains the display fallback; after the second user prompt, the agent defines a concise title in your language through `axlr_session`. Existing manual titles are preserved. The tool also stores the exact KMP about separately from the title, so `project:AXLR` keeps its spelling and can be reused on resume. This is model-guided bookkeeping; it does not start a MADE ceremony.

With KMP connected, the startup workflow requests recovery and one relevant comparison through `kmp_relate` once the task is clear, subject to your scope restrictions and plugin approval policy. It prefers a named set of abouts; an explicit `all_abouts` selection can discover candidates when none are known. Comparisons return proposals. Only inspected, evidence-backed identity links are declared, with the proposal context and a reviewed KMP write. A new absent about waits for a real durable fact before comparison; unavailable or partial recall remains explicit.

## Execute and verify

For direct work, AXLR discovers only the needed external tool schemas, executes authorized operations and checks the resulting artifact. Recover existing KMP decisions before replacing them with new conclusions. Read a matching installed skill through `axlr_skill`; a skill is guidance, not authority.

For debug/delivery, AXLR starts the procedure on the next prompt. The model submits a concrete check command to `axlr_step_done`; inspect its program, arguments and meaning before approving. In debug it must first reproduce the failure with a nonzero exit. In delivery it establishes a baseline. Repair/build must then pass that saved command. A command that always exits zero is not meaningful acceptance evidence.

For incident work, the console checks the timeline and actions, requests a draft review in a fresh model context, and waits for the person's decision on the exact draft through `/incident`. Only approved bytes become the published postmortem. See [incident review and approval](../ceremonies.md#incident-review-and-approval).

The console drives claims and transitions. Do not have the model also invoke MADE transitions for the same instance. A new prompt during execution steers work; it does not erase an already executed effect or restart the ceremony with new criteria. If scope or acceptance must change, reconcile the existing instance deliberately before starting another.

## Close with evidence

The handback should identify the changed artifacts, check and outcome, unresolved limits, and the revision/worktree inspected. `/changes` records local write/edit previews; use the actual workspace diff as well when processes or MCP tools changed files.

In a driven ceremony, verify `COMPLETED` or `BLOCKED` and the return to normal mode. Check the KMP write outcome separately. The driver uses the session's selected project about, with `ws:<session-id>` as fallback. It records a terminal observation with a stable idempotency key, and does not automatically acknowledge a KMP request for writer review. Save other reusable decisions with rationale and evidence under the stable project scope when appropriate. Do not turn a failed memory write into a claim of durable recall.

For transfer, use the [handoff contract](../ceremonies.md#handoff-contract): record exact state and evidence and obtain a real receiver's acceptance. For interruption or ambiguous effects, follow [recovery](recovery.md).

## Exit criteria

The requested artifact is reviewable, its checks are tied to the current work, engine state agrees with the reported outcome, and remaining work is explicit. Completing delivery does not itself commit, merge, deploy or publish; perform those actions only when they are part of the authorized task.
