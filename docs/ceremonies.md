# Default working ceremonies

AXLR includes seven MADE definitions and the `made:axlr-ceremonies` skill in the
console binary. The model's working guidance tells it to read that skill for
substantive tasks, select the smallest suitable procedure and reuse active
instances. User instructions can override the defaults. Plain answers and
one-step read-only lookups stay lightweight.

| Procedure | Use it for | Sequence |
|:--|:--|:--|
| `axlr_change` | Small, understood, reversible edits | Scope → change and check → hand back |
| `axlr_delivery` | Features and interacting changes | Brief → build / verify / review, at most 3 rounds → integrate |
| `axlr_debug` | Failures and failing checks | Reproduce → isolate cause → repair / prove, at most 3 rounds → integrate |
| `axlr_review` | Existing diffs or artifacts | Resolve exact artifact → inspect → prioritized report |
| `axlr_research` | Research and decisions | Frame → primary evidence → check synthesis → decision artifact |
| `axlr_publish` | Requested merge, deployment or publication | Exact verified packet → human decision → publish → read back |
| `axlr_handoff` | Transfer of ongoing responsibility | Durable checkpoint → actual receiver acceptance → confirmed transfer |

Definitions are sequential and use one concurrent step. Roles describe
responsibilities; they do not spawn agents. One worker can serve several roles,
but its review is labeled self-review. Independent review requires a distinct
reviewer. A handoff requires an actual receiver. An absent receiver leaves the
checkpoint prepared and the handoff pending.

## Read the defaults

No package installation is needed. The `axlr_skill` tool reads the embedded skill
and its references, even before a MADE server is connected:

```json
{"plugin":"made","skill":"axlr-ceremonies","path":"SKILL.md"}
```

Then read `references/catalog.json`, the selected
`references/axlr_<procedure>.yaml`, and the relevant execution or handoff guide.
Follow the tool's byte-page cursors until the complete resource has been read.
Resources live in
[`tui/adapters/axlrplugin/builtin/made/skills/axlr-ceremonies`](../tui/adapters/axlrplugin/builtin/made/skills/axlr-ceremonies/SKILL.md).

The catalogue pins version `1.0`, the file SHA-256 and MADE's semantic definition
digest for each procedure. These identities differ: YAML formatting changes a
file hash without necessarily changing the semantic definition. Published
name/version/content identities are immutable; conflicts require reviewed
versions, not replacement.

## Connect and install into MADE

Follow the [MADE runbook](runbooks/made.md). The defaults require MADE 0.9.1 or a
discovered compatible engine supporting output-field guards, bounded state
repeats and per-step timeouts. Ask AXLR to **install the default ceremony
catalogue into the connected MADE store** when you want those definitions
published. The skill validates the complete YAML, compares existing versions
and publishes only the authorized exact definitions. It starts a selected
procedure only for requested work; installing the catalogue starts no tasks.

Loading the embedded resources never starts the engine, changes MCP approval
policy or mutates a SQLite store. A disconnected engine or insufficient grants
is reported; the agent can continue authorized reversible work using the same
procedure locally, explicitly untracked. The embedded definitions do not
pretend that a MADE instance ran.

Execution uses fenced claim → real host work → structured completion → enabled
transition. The engine checks success booleans; AXLR must supply the actual
evidence. A successful no-op handler, role label or stale test cannot establish
a delivered result. Delivery/debug deliberately stop in the loop after three
unsuccessful rounds, preserving the failed state for diagnosis rather than
closing it as completed.

Keep `approve_ceremony_guard` and authorization grant administration off the
work-agent principal. Use a separate trusted human/operator channel for
publication approval and a setup operation for catalogue publication. MADE
0.9.1 records caller-declared actor/role provenance: a privileged approval
caller can set a human guard irrespective of those labels. YAML roles alone
do not enforce the human boundary. The skill checks this limit and leaves
publication pending if the separation is missing. It does not rewrite the
user's grants.

## Handoff contract

The checkpoint includes objective and acceptance criteria; authorization and
constraints; worktree, revision and dirty files; decisions and evidence;
completed work, blockers and exact next action; checks tied to a revision;
artifacts and opted-in KMP identities; MADE instance, pinned definition, state,
cursors, active claims and uncertain external operation receipts.

The receiver reads the exact digest, verifies accessible evidence and current
resume state, and acknowledges its real identity and continuation ownership.
The integrator then records the supported handoff/rebinding and reads back its
receipt. Preparing a summary does not release a live claim. A sender playing a
second logical role does not prove receipt, and context compaction on the same
worker normally requires a checkpoint/resume instead of ownership transfer.

## Evidence and maintenance

The catalogue was designed with `made_design_ceremony` and validated against a
checksummed MADE 0.9.1 engine. The publication draft was amended to put approval
before the external operation and readback. Delivery and debug gained explicit
success-field guards. The [research notes](../tui/adapters/axlrplugin/builtin/made/skills/axlr-ceremonies/references/research.md)
link the primary sources and explain the choices. This is an AXLR design, not a
claim that one universally best ceremony set has been established.

Run native validation and synthetic state-machine checks in an isolated store:

```bash
python3 tools/ceremonies/check.py --made-bin /absolute/path/to/made-mcp
```

The check publishes only into its disposable store and tests success, failed
verification, repeat exhaustion, missing receiver acceptance and the
publication permission boundary. It performs no live handoff or external
publication. After a reviewed definition revision, use `--write-catalog` to
refresh hashes/digests; bump published versions when content changes. Go tests
also check default availability, resource paging, path isolation and file pins.
