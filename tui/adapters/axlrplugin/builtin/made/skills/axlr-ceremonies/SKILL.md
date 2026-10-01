---
name: axlr-ceremonies
description: Choose and execute AXLR's default MADE workflow for changes, feature delivery, debugging, reviews, research, publication, and work handoffs. Use when AXLR begins substantive work or transfers an ongoing task; keep simple answers lightweight.
---

# AXLR ceremonies

Choose the smallest workflow that fits the user's task. These are AXLR's default working procedures; user instructions and existing authorization remain authoritative. Plain answers and one-step read-only lookups need no ceremony. Announce the selected workflow briefly, then do the work.

## Route the task

| Task | Definition | Successful outcome |
|:--|:--|:--|
| Small, understood, reversible edit | `axlr_change` | Scoped diff and proportionate checks |
| Feature, refactor, or several interacting changes | `axlr_delivery` | Accepted, verified revision and delivery report |
| Unexpected failure or failing check | `axlr_debug` | Reproduction, supported cause, proven repair |
| Review an existing artifact | `axlr_review` | Prioritized findings; artifact stays unchanged |
| Research or compare options | `axlr_research` | Supported recommendation and attributable sources |
| Publish, merge, deploy, or another external release | `axlr_publish` | Exact artifact approval, operation receipt, readback |
| Transfer responsibility or resume on another agent/host | `axlr_handoff` | Exact checkpoint accepted by a real receiver |

An active instance takes precedence: inspect and resume it instead of starting a duplicate. A failed check inside delivery first gets a focused diagnosis; keep the parent instance and record the relationship. Handoff preserves the underlying workflow and its state. Publication is a separate final procedure, used only when that action is requested or already authorized.

## Load only what is needed

Read [catalog.json](references/catalog.json) and the selected `references/<definition>.yaml`, following all returned byte-page cursors. In AXLR use `axlr_skill` with plugin `made`, skill `axlr-ceremonies`, and that relative path. In another host read the same packaged resource. Every definition is version `1.0`; require MADE 0.9.1 or a discovered compatible engine supporting output guards, bounded state repeats and per-step timeouts.

Read [execution.md](references/execution.md) before the first engine execution, then reuse it. Read [handoff.md](references/handoff.md) for a transfer. [research.md](references/research.md) explains the design and its sources.

## Work through MADE

Discover the actual tools and schemas. In AXLR, `axlr_tools` resolves the exact registered name/schema and `axlr_call_tool` invokes it. Use `made_discover_capabilities` and `made_get_help` with `audience: agent` once per connected engine. A catalogue row is not a connection.

List/get the selected published definition and compare its version and digest with the packaged catalogue. If absent, validate the complete YAML and publish the exact definition only when installation/publication is authorized. A conflict is not permission to overwrite an immutable version. Start only the selected published version with real inputs, stable instance identity and actual participant bindings. Do not publish the whole catalogue or start a system for every task.

Claim the returned next step, retain its fence, perform the actual host work, and complete with structured output and evidence. Refresh the instance and apply only enabled transitions. A claim or no-op handler proves no work. Boolean success fields must follow the step's stated criteria; failure, missing evidence and unavailable checks do not become success.

Roles are responsibilities, not additional agents. One authorized AXLR agent can serve sequential roles; label its review `self`. Independent review and a handoff receiver require distinct real participants. Never spawn, message, or invent agents merely because a YAML declares them. Keep model/effort/capability profiles outside the definition; record requested versus actual identities, or `unknown`.

Delivery and repair stop after three unsuccessful iterations. Diagnose or report the concrete blocker, preserve the instance, and renegotiate only the missing decision. Do not reset the loop by silently starting a new instance. Human publication approval applies to the exact artifact, destination and scope; agents cannot manufacture it. Verify that the work-agent principal has no guard-approval or grant-administration capability; see the enforcement limit in `execution.md`.

If MADE is unavailable or required grants are missing, say so. Continue authorized reversible work using the selected procedure locally, preserving a checkpoint. Report it as an untracked workflow, never as a MADE instance or engine-confirmed completion. Do not change approval policies to get past a denial.
