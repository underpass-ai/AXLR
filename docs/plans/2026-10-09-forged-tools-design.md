# Forged tools (meta-tooling) design

Status: implemented (9 Oct 2026): `axlr_forge_tool` and `axlr_run_tool`, on by default (`forged_tools` in settings). Scope chosen by Tirso: the tool half of "agents that write their own tools at runtime" (Sandhya Subramani, AWS, Strands Agents' meta-tooling: an editor, a shell and `load_tool`); runtime sub-agents are left for a separate design. Decisions taken with the advisor.

## Goal

When no tool does what a task needs, the model writes one and calls it at once, in the same session and without restarting the console, and fixes it by writing it again. Strands does this by loading Python files from a tools directory into the running agent. AXLR keeps its own invariants while doing the same: a fixed request prefix, explicit registration, approval per effect and the runtime's confinement.

## Decisions

- **Two fixed host tools, not a new tool kind.** `axlr_forge_tool` (upsert) and `axlr_run_tool` (`{name, arguments}`) are host tools like `axlr_tools` / `axlr_call_tool`. A forged tool never enters `tools[]` or the system prompt, so the prompt cache survives a new tool and `TestForgingKeepsTheRequestPrefix` proves it. A new `ToolIdentity` kind would have touched about twenty switches; an MCP server per tool would have added a system-prompt paragraph and needs an unregister the plugin manager does not have.
- **Resolution at call time.** `axlr_run_tool` reads the workspace registry when it runs, not the turn's frozen snapshot, which is what makes a tool forged in this turn callable in this turn. `axlr_tools` lists forged tools beside plugin tools (marked `forged`, `call_with: axlr_run_tool`) and returns a forged tool's schema by exact name, so the model looks before writing ("always check whether it exists" in the talk).
- **Registration, not scanning.** The registry is written only by `axlr_forge_tool` and lives in the console's private state (`<state>/axlr/forged-tools/<digest of the workspace path>.json`, owner-only), not in the workspace: a directory planted under `.axlr/tools/`, or tools and a registry that arrive with a checkout, are not tools until this console's model forges them on a card. The registry keeps each file's SHA-256 and `axlr_run_tool` refuses a tool whose files changed since it was forged here, so a run executes the files the forge card showed. `args` must name one of the tool's own files; that keeps the tool's code in its files, while anything else in the command (an interpreter's `-c`, say) is visible on the same forge card.
- **Judged and approved as what it does.** As `axlr_remember` is approved as `kmp_write_memory`, `axlr_forge_tool` is judged and approved as `local_write` and `axlr_run_tool` as `local_exec`. Review mode keeps running forged tools but is not offered the forge; writer, research and the other restricted modes refuse forging and ask for every run. Always-allow stays on the local identities (host kinds cannot be always-allowed); autonomy follows them.
- **Arguments on stdin.** A run validates the arguments against the tool's `input_schema` with the plugin validator and passes them as one JSON object on stdin; the card shows them. Restricted modes refuse a model's `local_exec` with stdin because stdin hides code below the card's fold; here stdin is data the card shows in full.
- **Confinement, not a sandbox.** A run is a `local_exec`: workspace cwd, `PATH` and `HOME`, the runtime's timeout and output limit, and the bubblewrap [exec sandbox](../console.md#exec-sandbox) when configured. Without it, a forged tool has the person's rights, like any approved command.
- **Workspace persistence.** Tool files stay in `.axlr/tools/<name>/` and their registration in this console's state, for later sessions of the same workspace. Committing the files shares the code, not the registration. Re-forging replaces the directory, so no file of an old version lingers.
- **Ordinary sessions only.** Ceremonies, repairs, improvements, plans and tasks keep their own tool surface. A forged tool's failure is refused as evidence for `axlr_request_repair`: fixing AXLR would not fix it.

## Smoke test (9 Oct 2026, claude-haiku-5.5, live console)

Asked for revenue per product of a CSV "that I will ask for often", the model forged `total_por_producto` unprompted and ran it; asked to add units, it forged it again under the same name and ran it in the same reply; in a new session it found the tool through `axlr_tools`, refused to run it after the file was changed outside AXLR and did not re-forge the changed code blindly; asked to restore it, it wrote a new version that skips and lists invalid rows. Totals checked by hand. Session cost under $0.01, cache 90-97 %. Fixed from what it showed:

- Haiku named the files by their workspace path three times in a row and looped on the refusal, whose example repeated the prefix: both forms are accepted now.
- The card read "read-only host forge_tool", showed the code as one escaped JSON string and offered an always-allow a host tool cannot save: it now names the tool and what it writes or runs, shows each file as code and offers no always-allow on host cards.
- A search of five terms found nothing because one term was not in the description: a search with no full match returns partial matches, marked `partial`.

## Not done

- **Runtime sub-agents** (the talk's second demo: the agent writes two to four specialised agents and calls them). AXLR's child sessions (`UseCaseWorkbench`) need MADE and a ceremony, `Reviewer` is a single generation without a tool loop, and the product contract says a ceremony role does not spawn a worker. A design would need a bounded child loop with a tool subset, its own budget and cost ledger, and a way to show its transcript.
- **Evals of forged tools.** The talk's evals (goal achieved, right tool and parameters, inter-agent order) map onto Jev and the transcript; nothing scores forged tools yet.
- **Per-tool always-allow** for one forged tool rather than every `local_exec`.
- **Restoring a changed tool.** The registry keeps digests, not the code, so a tool changed outside AXLR cannot be put back; the model writes it again.
- **The run card** shows the tool's name and arguments, not the registered command: the terminal has no access to the registry. Approving a run approves the command the forge card showed; `axlr_tools` with the name shows it again.
- **Review file changes.** The forge writes through the store, not `local_write`, so its files are not in the file-change panel; the forge card is their review.
