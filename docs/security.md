# Security and data

What AXLR sends off the machine, what it keeps, how it limits what a model can do, and how to report a vulnerability. The [security policy](../SECURITY.md) covers reporting; this page covers behaviour. It describes `main`.

## No telemetry

AXLR has no product telemetry: no usage statistics, crash reports or analytics are sent to Underpass or anyone else. Every network request it makes is one of those below, and each is caused by your configuration or your action.

## What leaves the machine

| Destination | When | What is sent |
|:--|:--|:--|
| `openrouter.ai` | An OpenRouter model is selected | The model request: prompts, tool definitions, tool results and the files the model read |
| `api.anthropic.com`, `api.openai.com` | A `local_models` entry names `api: messages` or `api: responses` against them ([native APIs](configuration.md#native-apis)) | The model request, as above |
| Your local model server | A local model is selected | The model request, as above; it stays on the host or network you configured |
| `api.typesafe.ai` | Jev is on: the setup assistant turns it on only when `TYPESAFE_API_KEY` is set, and says so on stderr | The questions Jev judges: final answers, the turn's evidence and memory candidates |
| `api.github.com`, `github.com` | The console installs or updates the KMP and MADE engines, or `/repair` drives a pull request | Release downloads; for `/repair`, the repository operations you approved |
| `registry.underpassai.com`, `api.registry.underpassai.com` | You search, install or publish catalog tools | Catalog index and manifest downloads; for publishing, the tool you approved |
| MCP servers and plugins | You connect them | Whatever the tool call carries; each server has its own data policy |

[Model keys](configuration.md#native-apis) are never passed to local commands or plugins, and are redacted from diagnostics. Leave `TYPESAFE_API_KEY` unset to keep Jev off.

## What AXLR keeps, and for how long

| Data | Where | Retention |
|:--|:--|:--|
| Sessions (the full transcript) | `$XDG_STATE_HOME/axlr/sessions` (fallback `~/.local/state/axlr/sessions`) | Until you delete the session (`Ctrl+O`, `Ctrl+Del`) |
| Timing traces (no prompts, responses, tool arguments or credentials) | `$XDG_STATE_HOME/axlr/logs` | Deleted after `trace_retention_days` (default 30) |
| Request and response payloads | Beside the trace, only with `--trace-payloads` or `trace_payloads: true` | Off by default; redacted and deleted with the traces |
| App log (errors and plugin events, secrets redacted) | `logs/axlr.log` | Rotated at 4 MiB, one previous file kept |
| Service journal, audit records and execution receipts (`axlr-serve`) | The service's state directory | Kept until the operator removes them |

Every file is owner-only (`0600`) in owner-only directories (`0700`). [Diagnostics and privacy](console.md#diagnostics-and-privacy) has the exact rules.

## Approvals and sandboxing

- **Approval cards.** A tool call shows its exact name, arguments and target before it runs, unless autonomy covers it. The setup assistant turns autonomy on; the [destructive list](configuration.md#approvals) (deletes outside the workspace, `git push`, `git reset --hard`, `git clean`, branch deletion, rebase, network writes) always asks.
- **Work modes and policy levels.** A [work mode](console.md#work-modes) hides the tools a run may not use; the service and `--exec` apply a policy level per execution.
- **Exec sandbox.** With `exec_sandbox.mode` `auto` or `required` on Linux, commands run under bubblewrap or a container with an enforced egress allow list ([`exec_sandbox`](configuration.md#exec_sandbox)). With it `off`, commands run with your account's rights.
- **Forged tools** run as an approved `local_exec` under the same confinement, and a tool whose files changed since it was forged does not run.
- **The service** uses manual approvals for local and engine tools, an `approver` role decides each exact call, and mTLS authentication never approves a tool by itself ([service API](api.md)).

The console and the JSON worker are not sandboxes when the exec sandbox is off. Run AXLR in an account and workspace you trust.

## Prompt injection

A model can read text written by someone else: a web page, a file in a checkout, an issue, a tool result. That text may try to give the model instructions. AXLR treats it as data:

- MCP tool descriptions, schemas and results, plugin descriptions and catalog text are untrusted data, not grants of authority. A tool's own annotations do not authorize anything.
- Earlier transcript excerpts, checkpoints and session metadata reach the model marked as untrusted evidence, not new instructions.
- Jev's judge questions open with an instruction to ignore requests found inside answers or tool outputs.
- A model's request to run a tool is checked against the host's own registry, policy and approvals; text cannot add a tool or widen a policy.
- An interrupted effectful call is left `uncertain` for a person, never retried automatically.

What you should do:

- Keep the destructive list on and review cards for actions you did not ask for, above all after the model read external content.
- Turn the exec sandbox on, and give an execution policy only the tools and egress it needs.
- Connect only MCP servers you trust: their results reach the model, and their side effects are theirs.

## Audit

The console keeps each session's record, the redacted app log and `/session`. The service keeps an event journal, audit records, correlation ids on every event and a receipt with the SHA-256 of every call's arguments and outcome ([service API](api.md), [runtime contract](runtime-contract.md)).

## Integrity of what you install

Release archives carry a `.sha256` beside each file, the container image and chart are published by version from immutable build artifacts ([releasing](releasing.md)), and the tool catalog index is signed with minisign and checked before any install.
