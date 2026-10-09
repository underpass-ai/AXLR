# AXLR product contract

AXLR is Underpass's default execution engine for agentic work. Start a task in AXLR, recover relevant evidence from KMP, execute through local tools and connected MCP servers, and use MADE when the task needs a tracked procedure. AXLR brings the result, checks and remaining decisions back to the user.

“Default” describes AXLR's role in the product stack: the entry point for doing the work. It does not mean every tool is automatically approved or every prompt starts a ceremony. A new console session starts in `normal` mode. The model provider currently implemented is OpenRouter.

## Three responsibilities

| Product | Owns | Evidence of success |
|:--|:--|:--|
| **AXLR — execute** | Model/tool loop, workspace operations, approvals, sessions and delivery of results | Actual tool outcomes, changes and checks visible in the session |
| **KMP — remember** | Durable graph-temporal memory, decisions, constraints, evidence and their relationships | Recall from the selected store and acknowledged memory writes |
| **MADE — coordinate** | Published procedures, step claims, transitions, bounded repetition and decision records | Recorded instance state and receipts backed by real AXLR work |

Session history is AXLR's execution record; it is not a replacement for project memory. MADE records progress but does not implement the work merely by changing a state. A skill supplies instructions; it does not grant a tool, connect an engine or create another agent.

## The working path

1. Choose the workspace, model, objective and acceptance evidence in AXLR.
2. Recover existing project context through the connected KMP store. Use explicit project scope; distinguish missing evidence from a connection failure.
3. Work directly in `normal`, or select a [work mode](console.md#work-modes). `/debug` and `/delivery` start console-driven MADE procedures on the next prompt. Other ceremonies run only when requested.
4. AXLR resolves the exact tool, validates arguments and applies the host's policy. Local operations and external MCP tools execute under their configured authority.
5. Verify the result, report limitations and preserve reusable decisions with evidence in KMP. In driven ceremonies, MADE records each completed step and AXLR attempts a KMP outcome write.

The [agent workflow runbook](runbooks/agent-workflow.md) makes this path operational. Connecting KMP and MADE is the recommended complete setup. The console can still work without them; it must not claim memory recall or a tracked ceremony that did not happen.

## Entry points and current capabilities

| Interface | Agent loop and sessions | KMP / MADE | Extension route |
|:--|:--|:--|:--|
| `axlr-tui` | Interactive loop, six work modes, saved sessions and approval UI | Explicit connections; driven debug/delivery; optional KMP recall/outcome recording | `/plugin` packages and `/mcp` stdio or Streamable HTTP tools |
| `axlr-serve` | HTTP `/v1`, gRPC and MCP, mTLS, sessions, event replay/follow, exact approvals and direct calls | Both remote adapters required by configuration/readiness | The shipped service config registers KMP and MADE only; no general plugin setting or work-mode API yet |
| `axlr` JSON worker | One operation per process; caller owns the loop and policy | Calls any explicitly registered engine tool | Repeatable `--plugin` manifests |
| Go library | Caller composes its loop, policy and persistence | Typed MCP client and plugin manager | Explicit registrations; custom HTTP clients for authentication |

The service shares turn use cases with the console, but it does not wire the console's package catalogue, skill reader, mode controls or ceremony driver. Treat those as interface-specific capabilities, not service promises.

The [transport parity contract](transport-parity.md) covers the 18 service/worker capabilities across HTTP operations, gRPC and MCP. Remote file, process and plugin operations retain the service's durable approval flow.

## Open extension model

AXLR can be extended beyond KMP and MADE with any MCP tool server that fits its supported transport, authentication and schema contract. It also reuses the skills and MCP declarations in compatible OpenAI/Codex plugin packages. Extensions add domain capabilities while AXLR remains responsible for execution and approval.

Compatibility is defined by the [component matrix](plugins.md#codex-plugin-compatibility), not a marketplace label. The current importer reads `.codex-plugin/plugin.json`; packages containing only the newer root `plugin.json` need a compatibility manifest. Host-specific hooks, connector accounts, UI components and OAuth flows are not automatically carried over. Use the [extension runbook](runbooks/extensions.md) to qualify a package or server.

## Guarantees and limits

- A built-in KMP or MADE catalogue row identifies the intended engine; a successful MCP discovery establishes connectivity. Verify store, identity and grants separately.
- Driven ceremonies use eight pinned definitions: `axlr_debug` / `axlr_delivery` **2.0** and `axlr_incident`, `axlr_repair`, `axlr_improve`, `axlr_plan`, `axlr_task`, `axlr_sync` **1.0**. The earlier skill catalogue of seven **1.0** definitions was retired on 5 Oct 2026.
- Automatic ceremony memory uses `ws:<session-id>`, not a stable project identity, unless an about was selected. The model's own `axlr_remember` defaults to the workspace's project about (`project:<root name>`, see the [KMP runbook](runbooks/kmp.md#use-memory-in-a-task)). Project-wide recall and richer decision links remain explicit KMP work. KMP failure does not roll back a completed MADE step.
- A work mode constrains AXLR's local file tools and process approval. It does not sandbox programs or classify external MCP side effects. Engine authorization and host approval remain separate.
- A forged tool is a program the model writes into the workspace: only `axlr_forge_tool` registers it, in the console's private state, each run is approved as a `local_exec` and runs under the same confinement, and a tool whose files changed since it was forged does not run. Files that arrive with a checkout are not tools. It is not a sandbox unless the exec sandbox is on.
- A role in a ceremony does not spawn a worker. Independent review and ownership transfer need real participants and evidence.
- Completion means observed work and appropriate checks. A lost response, approval or state transition alone cannot prove that an external operation succeeded.

## Source of truth

This contract describes `main` as audited on 4 October 2026 at `559a1b8`. [Architecture](architecture.md) maps responsibilities to source; the [audit](documentation-audit.md) records corrections and implementation limits. Current guides describe shipped code. Dated plans preserve design intent and may include work that is not implemented.
