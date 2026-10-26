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
| Model | `--model`, or `settings.json`, or the session's; with a routing, its first candidate when none is given | `CreateSession` (`model`, which a routing may leave out) |
| Workspace | `--root` (the current directory by default) | The service's `workspace` |
| Work mode | `--mode normal\|review\|writer\|research` | None: normal |
| Approval level | `--approve none\|local\|all` | None: every call waits for an `approver` |
| Session to continue | `--session <id>` | `session_id` |
| Decision on a pending call | `--session <id> --decide approve\|deny [--call <id>]` | `DecideSessionTool` |
| Policy | `--policy <file.json>` (with `--exec` or `--decide`) | `CreateSession` (`policy`) |
| Budget | `--budget <file.json>` (with `--exec` or `--decide`) | `CreateSession` (`budget`) |
| Sandbox | `--sandbox <file.json>` (with `--exec` or `--decide`) | `CreateSession` (`sandbox`) |
| Routing | `--routing <file.json>` (with `--exec` or `--decide`), else `settings.json`'s `routing` | `CreateSession` (`routing`), else the service's `settings.json` `routing` |
| Correlation | `--correlation <file.json>` (with `--exec` or `--decide`) | `CreateSession` (`correlation`) |

The approval level is the run's own policy on top of `settings.json`: `none` approves nothing the settings do not, `local` the workspace's reads, writes and commands and the forged tools, `all` every tool. The destructive list (`approvals.always_ask`) asks either way, and a call it holds ends the execution in `needs_decision`. A policy decides outside the model: the model asks, the policy answers.

## Policy per execution

A program narrows what an execution may do with tools, under the approval level and the settings: `axlr-tui --exec --policy <file.json>` reads it (and `--decide --policy`, for a decision under it), `CreateSession` takes it as `policy` and `GetSession` shows it. The session keeps it, so every later turn and decision applies it. The file holds the object below, with or without the `policy` wrapper; an unknown field refuses it, as the service refuses unknown fields.

```json
{"policy": {"level": "observe", "tools": {"allow": [{"name": "observability.query_logs"}, {"name": "local_read", "arguments": {"path": "logs/**"}}], "deny": [{"name": "local_exec"}]}}}
```

| Field | Meaning |
|---|---|
| `level` | The execution's autonomy (below) |
| `tools.allow` | The tools the execution may use. With an allow list, every tool it does not name is forbidden; without one, the level alone decides |
| `tools.deny` | The tools it may not use; a deny rule wins over an allow rule |
| A rule's `name` | The tool's name as the model calls it (`local_read`, `axlr_recall`); an MCP tool also by its own name or `<plugin>/<tool>`, since a name the model cannot use (a dot in it, a collision) is aliased. `*` matches within a name or a path segment, `**` across segments, `?` one character: `local_*`, `observability/*` |
| A rule's `arguments` | Patterns the call's arguments must match, each over the argument's value as text (a string as it is, anything else as its JSON). A rule with patterns matches only a call that carries every named argument with a matching value: `{"name": "local_exec", "arguments": {"program": "kubectl", "args": "*get*"}}` |

| `level` | What it allows |
|---|---|
| `observe` | Reads only: `local_read`, `local_search`, `local_list`, the read-only host tools (`axlr_tools`, `axlr_history`, `axlr_skill`, `axlr_session`, `axlr_logs`, `axlr_catalog`, `axlr_repair_status`, `axlr_recall`, `axlr_judge`) and the MCP tools the allow list names. MCP does not say which of a server's tools read, so the allow list vouches for the ones it names; `tui/domain/tool_reads.go` is the one list of reads |
| `propose` | Observe, and `local_write` or `local_edit` of a plan or a diff: a document (`.md`, `.mdx`, `.txt`, `.rst`, a file under `docs/`) or a `.diff` or `.patch` file |
| `act-with-approval` | The allowed writes and MCP tools, each with a decision, whatever the run's approval level allows |
| `act-within-policy` | The allowed writes and MCP tools, without a decision of the policy's own |

What the policy decides, and what it leaves to the rest of the chain:

- A call the policy forbids is refused outside the model, before any card: the model reads `forbidden by the execution's policy: <reason>` as the tool result, the call is `forbidden` in the result's `tool_calls`, and the execution goes on to its answer. A tool the policy forbids by name is not offered to the model at all: neither in its tool list nor by `axlr_tools`; `axlr_call_tool` stays while an MCP tool is allowed, and resolves to the MCP tool the policy judges.
- The policy only narrows. A call it allows still passes the run's approval level (`--approve`), the settings' rules, the work mode and the destructive list, which keep the last word: under `--approve none` an allowed write still waits for a decision (`needs_decision`), and `git push --force` asks under every level. `act-with-approval` adds a card even under `--approve all`.
- A decision taken under the policy cannot run what it forbids: `--decide approve` (or `DecideSessionTool`) on a forbidden call refuses it.

## Budget per execution

A program bounds what an execution may spend, and AXLR enforces it outside the model: `axlr-tui --exec --budget <file.json>` reads it (and `--decide --budget`, for a decision under it), `CreateSession` takes it as `budget` and `GetSession` shows it. The file holds the object below, with or without the `budget` wrapper; an unknown field refuses it. Each limit is zero, or absent, for no limit.

```json
{"budget": {"seconds": 900, "requests": 20, "tool_calls": 60, "usd": 0.5}}
```

| Limit | What it counts | Where it is checked |
|---|---|---|
| `seconds` | The time since the execution began: when `--budget` was read, or `CreateSession` | A deadline on the turn's context: the model request or tool call in flight is cancelled at it |
| `requests` | The model requests the execution made, failed and cancelled ones included | Before each model request |
| `tool_calls` | The tool calls the execution ran: the ones approved, by a person or automatically. A call refused by the policy, the mode, the turn's call limit or a denial counts not | Before each call that would run |
| `usd` | The cost the execution's requests reported, summed; a request that reported none is not counted (`unmetered_requests` in the result's usage) | Before each model request |

What the budget decides, and what it leaves as it is:

- A limit reached ends the execution with the status `budget_exceeded` and `error` naming the limit: `budget: tool_calls 60 reached`, `budget: usd 0.50 reached`. The request or call past the limit is never made. The session is left resumable with its pending state intact: a call the `tool_calls` limit refused stays pending (`pending` in the result), and `--decide approve` (or `DecideSessionTool`) under the same budget refuses it again; a run under a new `--budget` (or a session created without one) takes it. The model is told nothing: its next request is simply not made.
- The budget counts across the successor sessions a context cap opens: they are the same execution, so the successor carries the budget, its start and what was spent before it. The session keeps the budget, so a later turn of the same session counts under it too; a new `--budget` replaces it and counts from that moment, with the session's earlier requests, calls and cost left out.
- Once the execution has no request left, the console's own follow-up turns (the memory reminder, Jev's final check) are skipped, so a prompt that ended in an answer ends `completed`.
- The session's own cost cap (`max_session_usd`) and the turn's call limit (`turn_tool_calls`) in `settings.json` stay as they are; the budget only narrows further.

## Sandbox per execution

A program chooses how an execution's commands are confined and what of the network they reach, and AXLR enforces it outside the model: `axlr-tui --exec --sandbox <file.json>` reads it (and `--decide --sandbox`), `CreateSession` takes it as `sandbox` and `GetSession` shows it. The file holds the object below, with or without the `sandbox` wrapper; an unknown field refuses it. It is a file of its own, as the policy and the budget are: there is no execution bundle.

```json
{"sandbox": {"profile": "process", "egress": "allowlist", "allow": ["api.github.com:443", "*.pypi.org"], "writable": ["/home/bot/.cache/go-build"], "readable": ["/home/bot/go/pkg/mod"]}}
```

```json
{"sandbox": {"profile": "container", "egress": "none", "container": {"image": "registry.example/tools:1.4", "runtime": "podman", "memory_mb": 2048, "cpus": 2, "pids": 512}}}
```

| Field | Meaning |
|---|---|
| `profile` | `none`, `process` or `container` (required) |
| `egress` | `none`, `allowlist` or `full`; absent is `none` for `process` and `container`, and `full`, the only one it takes, for `none` |
| `allow` | With `allowlist`, which needs at least one, at most 64: `host`, `host:port`, `*.domain` (its subdomains, not the domain itself), an IPv4 address or an `[IPv6]` address; no port allows 443 and 80 |
| `writable` | Absolute paths outside the workspace the commands may write, such as a build cache; at most 32, each existing, without commas |
| `readable` | Absolute paths under a hidden directory the commands may read, such as a module cache under the home; at most 32 |
| `container` | With `container`, which needs it: `image` (on the host already: AXLR never pulls), `runtime` (`podman` or `docker`; absent tries Podman, then Docker), and `memory_mb`, `cpus` and `pids`, each zero or absent for the runtime's default |

What each profile enforces:

| | `none` | `process` | `container` |
|---|---|---|---|
| Runs under | The console's own: unconfined, or the settings' [`exec_sandbox`](console.md#exec-sandbox) | bubblewrap | A rootless Podman, or Docker |
| Filesystem | The person's rights, or `exec_sandbox`'s | The system read-only; the workspace and `writable` read-write; `/home`, `/root`, `/run`, `/var/tmp`, `/mnt`, `/media`, `/srv` hidden behind empty filesystems, `$HOME` empty and writable and discarded with the command, `readable` read-only over them; a private `/tmp` | The image read-only; the workspace, `writable` and `readable` at their own paths; a private `/tmp`; nothing else of the host |
| Processes | The person's | PID, IPC, UTS, cgroup and user namespaces of their own, no nested user namespace, a session of their own, no new privileges, killed with the console | No capability, no new privileges, the console's uid and gid, the container removed when the command ends or is killed |
| Limits | The command's timeout and output limit | The same | The same, and `memory_mb`, `cpus`, `pids` |

What each egress enforces:

| `egress` | `process` | `container` |
|---|---|---|
| `none` | A network namespace with only a loopback: no route out, no name resolution, no host service | `--network none` |
| `allowlist` | The same, with one way out: AXLR's egress proxy | The same, through the same proxy |
| `full` | The host's network, and its resolver | The runtime's default network |

The allow list is enforced, not advisory. A command under `none` or `allowlist` has no network interface but its own loopback; under `allowlist` it starts through a relay, `axlr-tui` itself (or `axlr-serve`) bound read-only into the sandbox, which listens on `127.0.0.1:3128` inside it and passes each connection through a Unix socket to an HTTP proxy that AXLR runs outside it for that command. The command gets `HTTP_PROXY`, `HTTPS_PROXY` and `ALL_PROXY` (and their lower-case forms) pointing at the relay and an empty `NO_PROXY`, which curl, pip, npm, git and Go's HTTP clients read; a program that ignores them reaches nothing at all. The proxy tunnels `CONNECT host:port` and forwards plain `http://` requests, checking each destination, and each redirect, against the list; a refused one is answered `403`, and the command's stderr ends with `axlr sandbox: egress refused to <host:port> (not in the execution's allow list)`. What it cannot carry is refused by construction: UDP, name resolution from inside, and TCP from a client that cannot use an HTTP proxy (ssh, most database drivers). Its limits:

- AXLR resolves the names, outside the sandbox, with the host's resolver: a listed name whose DNS points into the host's own network reaches it there. An IP address in the list matches that address only.
- TLS goes end to end through the tunnel: the proxy sees the host and port, not the request, so a CDN that serves other sites behind a listed name is reachable through it.

What the sandbox does not cover:

- AXLR's own model requests (OpenRouter, a local model, a native API) are made by the AXLR process, outside the sandbox: the allow list neither governs them nor needs to name the model provider.
- MCP servers are AXLR's children (stdio) or endpoints AXLR calls, outside the sandbox too. The allow list is for what a command reaches itself: name an MCP server's host only when a command must call it.
- It confines `local_exec` and the forged tools, which `axlr_run_tool` runs through `local_exec`. The file tools (`local_read`, `local_write`, `local_edit`, search and list) are kept to the workspace by AXLR itself, sandbox or not.
- The `process` profile sets no memory or CPU limit and installs no seccomp filter: the command's timeout and output limit apply as to any command.

How it is chosen and refused:

- A sandbox this host cannot give ends the execution before its first request, never with a weaker one. AXLR runs one trivial command under bwrap, or asks the container runtime for the image, and checks the `writable` and `readable` paths: `--exec` exits 1 with `the execution's sandbox is unavailable here: …` (bwrap missing, user namespaces refused, the runtime or the image missing, a path missing), and `CreateSession` answers `422` with `sandbox_unavailable`. A sandbox the host loses during the execution refuses each command (`sandbox_unavailable` in its result) instead of running it unconfined.
- An execution's sandbox replaces the settings' `exec_sandbox` for its commands, with one floor: under `exec_sandbox.mode: required` its profile cannot be `none`, and under `exec_sandbox.network: false` its egress cannot be `full`.
- The session keeps the sandbox: later turns and `--decide` run under it without the flag, a new `--sandbox` replaces it, and the successor sessions a context cap opens carry it. `--exec` says on stderr what it got: `sandbox process (bwrap): …; egress allowlist (api.github.com:443)`, or `sandbox container (rootless podman, image …)`.
- A container on a rootful Docker daemon is confined as well, but the daemon runs as root: a flaw in it or in the kernel is a flaw as root. `--exec` says `rootful docker`; prefer a rootless Podman, or Docker in rootless mode.
- The `process` and `container` profiles need Linux; on macOS and Windows they are refused. Inside a container the relay is the AXLR binary itself, which runs there because AXLR's releases are static.

## Routing per execution

A program lets AXLR choose the model of each request of an execution among candidates, by an objective and under constraints it keeps outside the model: `axlr-tui --exec --routing <file.json>` reads it (and `--decide --routing`), `CreateSession` takes it as `routing` and `GetSession` shows it. The file holds the object below, with or without the `routing` wrapper; an unknown field refuses it. A run or a session created without one takes `routing` from `settings.json`, when it has one, and a session that already has a routing keeps it.

```json
{"routing": {"candidates": ["qwen3.8-flash-next", "anthropic/claude-haiku-5.5"], "objective": "cost", "allow_remote_providers": false, "max_context_tokens": 32000, "max_request_usd": 0.05}}
```

| Field | Meaning |
|---|---|
| `candidates` | The models the execution may ask, 1 to 8, each once: `local_models` ids or OpenRouter ids. With `quality-first`, best first |
| `objective` | `cost`, `latency` or `quality-first` (required): how the candidates the constraints allow are ranked |
| `allow_remote_providers` | `false` keeps every request on this host; absent or `true` allows remote providers |
| `max_context_tokens` | A cap on the request's context for every candidate, on top of each model's own cap; absent or `0`, the models' caps alone |
| `max_request_usd` | A ceiling on what one request is expected to cost; absent or `0`, none |

The session's model stays what it was (the first candidate when the run or `CreateSession` names none); only each request goes elsewhere. A ceremony that asks a model of its own, such as the planner, keeps it.

What the constraints exclude, before any ranking, for each request:

| Constraint | A candidate is excluded when |
|---|---|
| Reach | No client can send to it: not a `local_models` entry, and no OpenRouter key |
| `allow_remote_providers: false` | It runs off this host. A model is on this host only when it is a `local_models` entry whose `url` names a loopback address: `localhost`, a name under `.localhost`, `127.0.0.0/8` or `::1`. A server elsewhere on the network, a native Anthropic or OpenAI entry and every OpenRouter model are remote |
| The window | The request, as AXLR counts it for that model, exceeds the model's context cap (three quarters of a local model's `context_tokens`, a remote model's `prompt_tokens`), narrowed by `max_context_tokens` |
| `max_request_usd` | The request is expected to cost more than the ceiling, or its cost cannot be told: a remote model never seen to report a cost and absent from OpenRouter's catalog |

How the objective ranks the rest:

| `objective` | First is |
|---|---|
| `cost` | The candidate whose request is expected to cost least: a model on this host costs no dollars; a remote one its observed cost per token, else its OpenRouter catalog price, times the request's tokens and an answer's; a cost no one can tell ranks after every known one; ties go to the faster |
| `latency` | The candidate expected to finish the request soonest: its observed time per request, smoothed towards 10 seconds, and a tenth of its time to the first byte; ties go to the cheaper |
| `quality-first` | The first candidate the constraints allow, in the order given |

Under `cost` and `latency` the expected cost or time is per successful request: a model whose recent requests failed (one in twenty is assumed before any) ranks lower. Under every objective a model whose last three requests failed, the latest less than five minutes ago, ranks after every other until those minutes pass, then is tried again.

What the routing learns from: every model request AXLR makes, routed or not, adds what it consumed, its cost, how long it took and whether the provider failed it to a store shared by every console and service of the install (`<state>/model-stats.json`): the last 32 requests per model and provider, 8 providers per model, 64 models, none older than seven days. A request the caller cancelled says nothing of the model and is not kept. OpenRouter's catalog is read once per process, and only when a remote candidate's cost ranks it or meets a ceiling and none of its requests reported one; a routing without remote providers never reads it.

When the model fails:

- A request the provider fails or times out before showing anything (no text, no reasoning, no tool call) is sent again, once, to the next candidate in the ranking. The failed request stays in the ledger and the receipt as `failed`, the second is another request the budget counts, and both are in the events. A request that failed after its answer began, or that the caller or the budget stopped, is not sent again.
- A request no candidate may take (each excluded, or the only one failed) ends the execution `interrupted` with `error` `no candidate model of the execution's routing can take this request: <why>`; it is never sent to the session's model instead. When the only obstacle is the window, the turn continues in a successor session, as past a single model's cap, at the largest cap among the excluded candidates.

The session keeps the routing: later turns and `--decide` route under it without the flag, a new `--routing` replaces it, and the successor sessions a context cap opens carry it. `--exec` says on stderr what it got (`routing cost among qwen3.8-flash-next, anthropic/claude-haiku-5.5; remote providers not allowed`) and, after a fallback, why (`qwen-8080 failed (…); the request goes to anthropic/claude-haiku-5.5`).

## Correlation

A program names the execution for its own records, and AXLR carries the ids unchanged: `axlr-tui --exec --correlation <file.json>` reads them (and `--decide --correlation`), `CreateSession` takes them as `correlation` and `GetSession` shows them. The file holds the object below, with or without the `correlation` wrapper; an unknown field refuses it. Each id is letters, digits and `-_.:/@+=`, at most 128 characters, and any may be left out.

```json
{"correlation": {"bot_id": "server-incident-investigator", "task_id": "task-789", "execution_id": "exec-1", "trace_id": "trace-abc"}}
```

Where the ids go, as given:

- Every `--exec --json` line carries a `correlation` object (absent without one), and the result repeats it in `correlation`.
- Every event the service journals for the session carries `correlation` in its payload: `operation.accepted`, `turn.started`, `text.delta`, the tool events, `approval.required`, `approval.decision`, `turn.completed`, `turn.interrupted`, `operation.failed`, `session.continued` and `session.closed`.
- Every record of the diagnostics trace the turn's loop writes carries `correlation`; in `--exec` the run's whole trace does, the adapters' records included.
- The turn's record in the session's MADE instance carries `correlation`, and its KMP entries are labelled `bot_id`, `task_id`, `execution_id` and `trace_id`.
- A successor session carries the correlation on.

The model never reads them: they are in no prompt, no guidance and no tool result.

## Artifacts

An execution hands over what it made, a report, a diff, a file, with the tool `axlr_deliver`, and AXLR lists it in the result's `artifacts` with what a control plane needs to fetch it and check it. The model calls it with:

| Argument | Meaning |
|---|---|
| `why` | Why it delivers this, one line |
| `name` | The deliverable's file name: letters, digits, `.`, `_` and `-`, at most 128 characters, starting with a letter or a digit, no `..` and no device name (`CON`, `NUL`…) |
| `media_type` | Its media type without parameters, `type/subtype` in lower case: `text/markdown`, `application/json`, `text/x-diff` |
| `description` | One line, at most 300 characters, saying what it is |
| `path` | A file of the workspace to deliver as it is: relative, inside the workspace once links are resolved, a regular file of at most 32 MiB |
| `content` | Or the deliverable's text inline, UTF-8, at most 256 KiB: AXLR writes it to `.axlr/deliverables/<session_id>/<name>` in the workspace |

Exactly one of `path` and `content` is given. Delivering a `path` reads the file and changes nothing; delivering `content` writes the one file under the deliverables folder, replacing an earlier delivery of the same name in the same session. The tool returns the artifact without its content, so the model's context does not grow with it.

How the levels judge a delivery, read as a `local_read` of the path or a `local_write` of the deliverables file:

| | `path` (a read) | `content` (a write) |
|---|---|---|
| `observe` | Allowed | Forbidden |
| `propose` | Allowed | Allowed when the name is a plan or a diff (`.md`, `.mdx`, `.txt`, `.rst`, `.diff`, `.patch`) |
| `act-with-approval` | Allowed | Allowed, with a decision |
| `act-within-policy` | Allowed | Allowed |
| Work mode `review` | Allowed | Refused: review mode changes no file |
| Work modes `writer`, `research` | Allowed | Allowed when the name is a document (`.md`, `.mdx`, `.txt`, `.rst`) |
| `--approve` | Approved, as a read | Approved as `local_write` would be: `local` or `all`, or the settings' rule for `local_write` |

An allow list must name `axlr_deliver` for an execution to deliver, as for any other tool; a rule may match its arguments (`{"name": "axlr_deliver", "arguments": {"path": "reports/**"}}`).

Each artifact, in the result and in the `artifact` event:

```json
{"name": "report.md", "media_type": "text/markdown", "size": 1834, "sha256": "9f2c…", "reference": ".axlr/deliverables/6c1f…/report.md", "description": "What failed and why", "source": "inline", "session_id": "6c1f…", "call_id": "deliver-1"}
```

| Field | Meaning |
|---|---|
| `name`, `media_type`, `description` | As the model gave them |
| `size`, `sha256` | The bytes delivered, hashed when the call ran |
| `reference` | The file's path relative to the workspace, with `/` separators: the control plane fetches it from the workspace the execution ran in |
| `source` | `file` for a delivered `path`, `inline` for `content` written under the deliverables folder |
| `session_id`, `call_id` | The call that delivered it |

The result lists every artifact of the sessions the execution spanned, in delivery order; a later delivery of the same `reference` replaces the earlier one. The file is the workspace's: AXLR does not keep a copy, so a file changed after its delivery no longer matches its `sha256`.

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
  "artifacts": [{"name": "plan.md", "media_type": "text/markdown", "size": 412, "sha256": "5e0b…", "reference": ".axlr/deliverables/6c1f…/plan.md", "description": "The steps before the file is created", "source": "inline", "session_id": "6c1f…", "call_id": "deliver-1"}]
}
```

| Field | Meaning |
|---|---|
| `schema` | `axlr.execution-result/v1`; check it before the fields |
| `status` | `completed`, `needs_decision`, `interrupted`, `failed` or `budget_exceeded` (below) |
| `session_id`, `sessions` | The session the execution ended in, and every session it spanned, first to last |
| `answer` | The last assistant message; empty when the turn gave none |
| `pending` | With `needs_decision`, or with `budget_exceeded` when the `tool_calls` limit refused a call: the call that waits, with its arguments |
| `error` | With `failed`, `interrupted` or `budget_exceeded`: why; a budget's error names the limit, `budget: requests 20 reached` |
| `tool_calls` | Every call of the session: `decision` is `approve`, `auto_approve` or `deny`; `status` is `pending`, `forbidden` (the execution's policy refused it outside the model), `denied`, `completed`, `failed` or `uncertain` (interrupted after it may have run) |
| `usage` | The ledgers of the spanned sessions, summed; `cost_usd` sums the `cost_requests` that reported a cost, and `unmetered_requests` the ones that reported none |
| `artifacts` | What the execution delivered with `axlr_deliver` ([above](#artifacts)), in every session it spanned; empty when it delivered nothing |
| `correlation` | The execution's correlation ids, as given; absent without any |

| `status` | Meaning | `axlr-tui` exit code |
|---|---|---|
| `completed` | The turn ended in an answer | 0 |
| `needs_decision` | A call waits for a decision the run was not allowed to take; the session keeps it | 3 |
| `interrupted` | The turn stopped before its end (cancelled, or the process died); the session can be resumed | 1 |
| `failed` | The loop returned an error | 1 |
| `budget_exceeded` | A limit of the execution's budget ended it before the next request or call; the session can be resumed, its pending state intact | 1 |

Where the result is:

- `axlr-tui --exec --json` and `--decide --json` end with one line `{"type":"result","result":{…}}`, after the `completed`, `decision.required` or `failed` line and the [receipt](#receipt) line (a budget's end is a `failed` line with the budget's error, then the receipt, then the result with `budget_exceeded`). Without `--json`, stdout is the answer and the exit code the status.
- `axlr-serve` puts it in `GetSession` (`result`, null until a turn ends) and in the payload of the event that ends a turn: `approval.required`, `turn.completed`, `turn.interrupted` (the answer's end, a cancellation, a budget's end) or `operation.failed`. The service's `error` is the operation's error code, never its message; a budget's `error` is the budget's own text. One exception: the `turn.interrupted` the service journals at start, for a session a crash left running, carries the status alone; read that session's result from `GetSession`.

## Receipt

The receipt is the execution's account for an audit: every model request with its model, provider, usage and cost, every tool call with hashes of what it was given and what it returned, and what the execution ran under. It is read from the session and its receipt journal, which AXLR writes as each request and call starts and ends, so it survives a restart: a request or call the process died in is listed as such.

```json
{
  "schema": "axlr.execution-receipt/v1",
  "status": "completed",
  "session_id": "6c1f…",
  "sessions": ["6c1f…"],
  "started_at": "2026-10-25T09:14:02.118Z",
  "ended_at": "2026-10-25T09:14:31.904Z",
  "policy": {"level": "observe", "sha256": "41aa…"},
  "budget": {"sha256": "c3d0…", "limits": {"seconds": 900, "requests": 20, "tool_calls": 60, "usd": 0.5}},
  "sandbox": {"sha256": "a1c4…", "profile": "process", "egress": "allowlist", "allow": ["api.github.com:443"]},
  "routing": {"sha256": "79e7…", "objective": "cost", "candidates": ["qwen-8080", "anthropic/claude-haiku-5.5"], "allow_remote_providers": true},
  "correlation": {"bot_id": "server-incident-investigator", "task_id": "task-789"},
  "requests": [
    {"session_id": "6c1f…", "index": 1, "started_at": "2026-10-25T09:14:02.296Z", "duration_ms": 3, "status": "failed", "model": "qwen-8080", "route": "ranked", "provider": "qwen-8080", "prompt_tokens": 0, "cached_tokens": 0, "cache_write_tokens": 0, "completion_tokens": 0, "reasoning_tokens": 0, "cost_usd": null, "unmetered": false},
    {"session_id": "6c1f…", "index": 2, "started_at": "2026-10-25T09:14:02.301Z", "duration_ms": 2310, "status": "completed", "model": "anthropic/claude-haiku-5.5", "route": "fallback", "provider": "Anthropic", "prompt_tokens": 3120, "cached_tokens": 2048, "cache_write_tokens": 0, "completion_tokens": 96, "reasoning_tokens": 0, "cost_usd": 0.0021, "unmetered": false}
  ],
  "tool_calls": [
    {"session_id": "6c1f…", "call_id": "read-1", "tool": "local_read", "arguments_sha256": "0b6e…", "outcome_sha256": "e3b4…", "decision": "auto_approve", "status": "completed", "started_at": "2026-10-25T09:14:04.650Z", "duration_ms": 3}
  ],
  "total": {"requests": 2, "prompt_tokens": 3120, "cached_tokens": 2048, "cache_write_tokens": 0, "completion_tokens": 96, "reasoning_tokens": 0, "cost_usd": 0.0021, "cost_requests": 1, "unmetered_requests": 0},
  "complete": true
}
```

| Field | Meaning |
|---|---|
| `schema` | `axlr.execution-receipt/v1`; check it before the fields |
| `status`, `session_id`, `sessions`, `correlation` | As in the result |
| `started_at`, `ended_at` | When the execution began (the budget's start, else the session's first message or record) and its last record, in UTC |
| `policy` | The execution's policy: its `level` and the `sha256` of its compact JSON as the contract writes it (`{"level":…,"tools":{…}}`, fields in the order above, argument patterns by name); absent without one |
| `budget` | The execution's budget: its `limits` and the `sha256` of their compact JSON (`{"seconds":…,"requests":…,"tool_calls":…,"usd":…}`, zero limits left out); absent without one |
| `routing` | The execution's routing: its `objective`, `candidates` and `allow_remote_providers`, and the `sha256` of its compact JSON as `GetSession` shows it (`{"candidates":[…],"objective":…,"allow_remote_providers":…,"max_context_tokens":…,"max_request_usd":…}`, zero constraints left out); absent without one |
| `sandbox` | The execution's sandbox: its `profile`, `egress` and `allow`, and the `sha256` of its compact JSON as `GetSession` shows it (`{"profile":…,"egress":…,"allow":[…],"writable":[…],"readable":[…],"container":{…}}`, the egress spelled out, each destination as the contract reads it: lower case, no trailing dot, empty lists left out); absent without one |
| `requests` | Every model request of the spanned sessions, in order. `index` counts from 1 in each session; `model` is the one asked for, the one the routing chose when the execution has one, and `provider` who served it, when the endpoint says; `route` is `ranked` (the routing ranked it first) or `fallback` (the one ranked first failed and this one took the request again), absent without a routing. `status` is `completed`, `failed`, `cancelled` or `interrupted` (the process died during it: tokens and cost unknown). `cost_usd` is null when the request reported none; `unmetered` marks a request whose tokens are estimates |
| `tool_calls` | Every call of the spanned sessions, in order, as the result's `tool_calls` with `arguments_sha256` (the call's arguments as the session keeps them, compact JSON), `outcome_sha256` (its result's text; absent while pending), and `started_at` and `duration_ms` for a call that ran (absent for one refused, or one that never ended) |
| `total` | The requests above, summed as the result's `usage` is |
| `complete` | True when every request the sessions' ledgers counted and every call that ran is itemized; false for a session that predates receipts, a journal line a crash cut or a journal past its bound (4 MiB a session) |

The receipt covers the whole session, every turn of it, as the result's `tool_calls` do. A request the process died in is in no ledger: the receipt lists it `interrupted` and counts it in `total.requests` and `total.unmetered_requests`, so its `total` can exceed the result's `usage` by those requests.

Where the receipt is:

- `axlr-tui --exec --json` and `--decide --json` write one line `{"type":"receipt","receipt":{…}}` just before the result line.
- `axlr-serve` puts it in `GetSession` (`receipt`, null until a turn ends), over that session's own requests and calls; a successor's receipt is read from the successor.

## Events

| `axlr-tui --json` line | `axlr-serve` journal | When |
|---|---|---|
| `session` | `turn.started` | The turn starts (again in each successor) |
| `model.routed` | `model.routed` | The routing sent a request to `model`: `route` `ranked`, or `fallback` with `from` (the model that failed) and its `error` |
| `reasoning` | — | Readable reasoning as it streams |
| `text` | `text.delta` | Answer text |
| `tool.requested`, `tool.completed` | `tool.requested`, `tool.completed` | A call is made, and its outcome recorded |
| `artifact` | `artifact` | `axlr_deliver` delivered an artifact: the line's (or payload's) `artifact`, after its `tool.completed` |
| `session.continued` | `session.continued` | The context cap opened a successor |
| `decision.required` | `approval.required` | A call waits |
| `completed` | `turn.completed` | The answer ended the turn |
| `failed` | `turn.interrupted`, `operation.failed` | The turn stopped or failed, or the budget ended it |
| `receipt` | (in `GetSession`) | The execution ended: its receipt, before the result |
| `result` | (in the payload above) | The execution ended |

A call the console approves by itself, such as a read of the workspace, goes from `tool.requested` to `tool.completed` with nothing between: `decision.required` and `approval.required` appear only when the execution stops for a decision, once, as the event that ends it.

Every line and every payload carries the execution's `correlation` when it has one. The service's events are durable and resumable (`StreamEvents` with a cursor); the command's lines are written once. The [transport parity ledger](transport-parity.md) lists the service's verbs on HTTP, gRPC and MCP.

## Guarantees

- A tool call never runs on the model's say-so: the execution's policy, the run's approval level, the settings' rules, the work mode and the destructive list decide. A call the policy forbids is refused outside the model, and a call none of them approves ends the execution in `needs_decision`.
- The result is written once, after the session is saved, so a reader that sees it can continue the session.
- On the service, a turn, a decision and a direct call are idempotent on their key, revisions guard decisions and cancellation, and a session interrupted by a crash, in a model request or in a tool's run, is journaled `turn.interrupted` at the next start.
- Past the context cap the execution continues in a successor; the result names every session, and their usage is summed. The successor is the same execution: it carries the policy, the budget with what was spent, the sandbox, the routing and the correlation.
- A routing is kept outside the model: no request goes to a candidate its constraints exclude, a routing without remote providers sends nothing off the host, and a request no candidate may take is refused, never sent to another model.
- The budget is kept outside the model: a limit reached refuses the next request or call before it is made, whatever the model asked for, and the session stays resumable.
- A command of an execution with a sandbox runs under that sandbox or not at all: a sandbox the host cannot give ends the execution before its first request, and the allow list is the only way out of a sandbox without the network.
- The receipt is journaled as each request and call starts and ends, before the next step: a restart loses none of what ran, and a request or call cut by the restart is listed as interrupted or without its end.
