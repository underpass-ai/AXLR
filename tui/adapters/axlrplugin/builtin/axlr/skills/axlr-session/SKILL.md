---
name: axlr-session
description: Initialize or resume AXLR session context, define its title after the first exchanges, and find evidence-backed connections between KMP abouts.
---

# AXLR session context

Use this skill at session startup and when a resumed session lacks a title or
memory scope. Session bookkeeping belongs to AXLR, durable evidence to KMP,
and ceremony state to MADE. Keep working on the user's task while establishing
this context; no ceremony is needed for a title or a memory comparison.

## Settle the session

Read `axlr_session` with `{}`. Reuse the returned `session_id`, workspace,
`user_prompt_count`, title and about. Before two user prompts, keep the first
prompt as the display fallback. After the second prompt, define a short title
in the user's language describing the concrete work, using `axlr_session`
with `title`. Do it before ending that turn once the task is clear. If the
topic is still ambiguous, wait for clarification already needed by the task.
Tool calls and console ceremony reminders are not new conversations.

The tool fills missing fields only. Preserve a title the user assigned with
F2; do not repeatedly rename a session or use the title as a memory identifier.
Resume an existing session instead of treating each turn or compaction as new.

## Select and recover memory

When KMP is connected, this AXLR startup workflow requests project-memory
recovery and one relevant inter-about comparison. Respect an explicit user
opt-out. Without a connection, title the session and report the memory step
as unavailable; do not claim it ran.

Reuse the exact about from session metadata, repository instructions, the user
or KMP evidence. Abouts are opaque and case-sensitive: `project:AXLR` and
`project:axlr` differ. Never derive one from the session title or silently change
a supplied identifier. Save a confirmed scope with `axlr_session` using `about`.
For genuinely new work with no canonical project scope, use `ws:<session_id>`
as an explicit session scope, not as proof of a pre-existing project. Do not
create empty memory merely to make recovery succeed.

The persisted scope is the session's original binding. If the user explicitly
chooses another scope during that session, use the supplied identifier for
those explicit memory calls and disclose that the saved binding is unchanged.
Use a new session to bind a different project for a console-driven ceremony;
do not report a fill-only call as a successful scope replacement.

Resolve live tool names and schemas with `axlr_tools`; invoke through
`axlr_call_tool`, retaining the configured plugin approval policy. Start
`kmp_guide` once with a stable registration key based on this session, keep
its actual agent/context IDs and pass `context_id` on work calls. Reuse guidance
already read. After compaction, retain the agent ID and open a fresh context.
Wake known work before re-deriving it. Finish relevant returned pages using the
exact continuation alone; preserve refs and the recall frontier. A missing
new scope, `UNKNOWN`, missing guide assets and a store error are different
outcomes. Check store selection before treating expected memory as new.

## Compare abouts once the task is clear

After the second user prompt establishes the goal, read
[interabouts.md](references/interabouts.md) and perform the bounded comparison.
Reuse its result on later turns; repeat only for a changed scope, new relevant
evidence or an explicit user request. At a durable decision or outcome, record
source-backed facts and useful labels with a stable idempotency key. Review
`needs_review` before resuming. Summarize what memory contributed and any
unresolved limitation without turning tool activity into the user's main task.

If a console MADE ceremony is already active, let `axlr_step_done` advance it.
Do not start, claim or transition a second instance while settling the session.
Its initial recall may use `ws:<session_id>` before the project binding is
known. Selecting the project later does not rewrite that initial recall or
MADE's original inputs; the host checks the saved scope again for the terminal
outcome write. Keep initial recall provenance and final storage scope distinct.
In `/incident`, the console records the approved outcome; the model cannot
write it through `kmp_write_memory`. If a new about needs its first durable
fact, defer that write and comparison until the approved outcome exists.
