# The runtime contract

What a program needs from AXLR to run one unit of work and read what came of it, without a screen and without reading traces. It is the contract between the control plane of Underpass Bots, which decides *when, where, as whom and under which permissions* a unit of work runs, and AXLR, which decides *how* an agent reasons and uses tools (the [master plan](plans/2026-10-23-master-plan.md), Phase 0). Two surfaces speak it, with the same words: the one command `axlr-tui --exec` and the service `axlr-serve`.

## One execution is one session

An execution is a session of AXLR: its transcript, its tool calls with their decisions, its usage ledger and its MADE instance and KMP context, as the [console](console.md) keeps them. The session stays after the execution ends, so a later execution can continue it (a decision on a pending call, a new prompt). Past the context cap the execution goes on in a successor session; the result names every session it spanned.

| Concept | In AXLR |
|---|---|
| Execution | A session, plus the successors its context cap opened |
| Request | A prompt (or a decision) for a session, with its model, workspace, work mode and approval level |
| Result | The object below, written once when the execution ends |
| Events | The turn's steps as they happen (JSON lines, or the service's journal) |

## Request

| Input | `axlr-tui` | `axlr-serve` |
|---|---|---|
| Prompt | `--exec "<prompt>"` | `StartTurn` (`prompt`) |
| Model | `--model`, or `settings.json`, or the session's | `CreateSession` (`model`) |
| Workspace | `--root` (the current directory by default) | The service's `workspace` |
| Work mode | `--mode normal\|review\|writer\|research` | None: normal |
| Approval level | `--approve none\|local\|all` | None: every call waits for an `approver` |
| Session to continue | `--session <id>` | `session_id` |
| Decision on a pending call | `--session <id> --decide approve\|deny [--call <id>]` | `DecideSessionTool` |

The approval level is the run's own policy on top of `settings.json`: `none` approves nothing the settings do not, `local` the workspace's reads, writes and commands and the forged tools, `all` every tool. The destructive list (`approvals.always_ask`) asks either way, and a call it holds ends the execution in `needs_decision`. A policy decides outside the model: the model asks, the policy answers.

## Result

```json
{
  "schema": "axlr.execution-result/v1",
  "status": "needs_decision",
  "session_id": "6c1f…",
  "sessions": ["6c1f…"],
  "answer": "I will create the file now.",
  "pending": {"call_id": "exec-1", "tool": "local_exec", "arguments": {"program": "touch", "args": ["notes.md"], "why": "The person asked for it."}},
  "tool_calls": [
    {"call_id": "read-1", "tool": "local_read", "decision": "auto_approve", "status": "completed"},
    {"call_id": "exec-1", "tool": "local_exec", "status": "pending"}
  ],
  "usage": {"requests": 2, "prompt_tokens": 3120, "cached_tokens": 2048, "cache_write_tokens": 0, "completion_tokens": 96, "reasoning_tokens": 40, "cost_usd": 0.0021, "cost_requests": 2, "unmetered_requests": 0},
  "artifacts": []
}
```

| Field | Meaning |
|---|---|
| `schema` | `axlr.execution-result/v1`; check it before the fields |
| `status` | `completed`, `needs_decision`, `interrupted` or `failed` (below) |
| `session_id`, `sessions` | The session the execution ended in, and every session it spanned, first to last |
| `answer` | The last assistant message; empty when the turn gave none |
| `pending` | With `needs_decision`: the call that waits, with its arguments |
| `error` | With `failed` or `interrupted`: why |
| `tool_calls` | Every call of the session: `decision` is `approve`, `auto_approve` or `deny`; `status` is `pending`, `denied`, `completed`, `failed` or `uncertain` (interrupted after it may have run) |
| `usage` | The ledgers of the spanned sessions, summed; `cost_usd` sums the `cost_requests` that reported a cost, and `unmetered_requests` the ones that reported none |
| `artifacts` | Reserved for the deliverables an execution hands over (Phase 1); empty until then |

| `status` | Meaning | `axlr-tui` exit code |
|---|---|---|
| `completed` | The turn ended in an answer | 0 |
| `needs_decision` | A call waits for a decision the run was not allowed to take; the session keeps it | 3 |
| `interrupted` | The turn stopped before its end (cancelled, or the process died); the session can be resumed | 1 |
| `failed` | The loop returned an error | 1 |

Where the result is:

- `axlr-tui --exec --json` and `--decide --json` end with one line `{"type":"result","result":{…}}`, after the `completed`, `decision.required` or `failed` line. Without `--json`, stdout is the answer and the exit code the status.
- `axlr-serve` puts it in `GetSession` (`result`, null until a turn ends) and in the payload of the event that ends a turn: `approval.required`, `turn.completed`, `turn.interrupted` or `operation.failed`. The service's `error` is the operation's error code, never its message.

## Events

| `axlr-tui --json` line | `axlr-serve` journal | When |
|---|---|---|
| `session` | `turn.started` | The turn starts (again in each successor) |
| `reasoning` | — | Readable reasoning as it streams |
| `text` | `text.delta` | Answer text |
| `tool.requested`, `tool.completed` | `tool.requested`, `tool.completed` | A call is made, and its outcome recorded |
| `session.continued` | `session.continued` | The context cap opened a successor |
| `decision.required` | `approval.required` | A call waits |
| `completed` | `turn.completed` | The answer ended the turn |
| `failed` | `turn.interrupted`, `operation.failed` | The turn stopped or failed |
| `result` | (in the payload above) | The execution ended |

The service's events are durable and resumable (`StreamEvents` with a cursor); the command's lines are written once. The [transport parity ledger](transport-parity.md) lists the service's verbs on HTTP, gRPC and MCP.

## Reserved for the next phases

The shape is fixed now so the control plane can be written against it; AXLR does not accept these yet (the service rejects unknown fields, the command has no flag for them). The master plan says in which phase each ships.

**Policy per execution** (Phase 1): tools and argument patterns the execution may use, deny by default, with the autonomy levels of the vision as presets. It composes under the run's approval level and the destructive list, which keep the last word.

```json
{"policy": {"level": "observe", "tools": {"allow": [{"name": "observability.query_logs"}, {"name": "local_read", "arguments": {"path": "logs/**"}}], "deny": [{"name": "local_exec"}]}}}
```

| `level` | What it allows |
|---|---|
| `observe` | Read-only tools in the allow list |
| `propose` | Observe, and writes of a plan or a diff in the workspace |
| `act-with-approval` | Writes in the allow list, each with a decision |
| `act-within-policy` | Writes in the allow list, without a decision |

**Budget per execution** (Phase 2): enforced outside the model; the result then says which limit ended the execution.

```json
{"budget": {"seconds": 900, "requests": 20, "tool_calls": 60, "usd": 0.5}}
```

**Correlation** (Phase 2): carried unchanged through events, traces and the session's MADE records.

```json
{"correlation": {"bot_id": "server-incident-investigator", "task_id": "task-789", "execution_id": "exec-1", "trace_id": "trace-abc"}}
```

**Artifacts** (Phase 1): `artifacts` in the result lists what an execution delivered (a report, a diff, a file), each with a name, a media type and a reference the control plane can fetch.

## Guarantees

- A tool call never runs on the model's say-so: the run's policy, the settings' rules, the work mode and the destructive list decide, and a call none of them approves ends the execution in `needs_decision`.
- The result is written once, after the session is saved, so a reader that sees it can continue the session.
- On the service, a turn, a decision and a direct call are idempotent on their key, revisions guard decisions and cancellation, and a session interrupted by a crash is journaled `turn.interrupted` at the next start.
- Past the context cap the execution continues in a successor; the result names every session, and their usage is summed.
