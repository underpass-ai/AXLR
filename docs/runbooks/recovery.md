# Runbook: recovery and engine maintenance

AXLR session state, engine state and external effects have different owners. Establish the last observed state before retrying work. Neither timeout nor cancellation proves that a program or MCP operation had no effect.

## Recover a session or ceremony

1. Record the session ID, workspace, last visible operation and any MADE instance or external operation receipt. Keep diagnostic content private.
2. Verify that no other console owns the session. Do not delete a live writer lock. Reopen with `Ctrl+O` or `axlr-tui --root /absolute/workspace --session ID`.
3. Review the restored transcript, pending calls and workspace. Restored interrupted sessions execute nothing until explicitly continued with `Ctrl+R`; pending calls use the current policy.
4. For an ambiguous local or MCP effect, read the target's actual state. Reconcile using its operation ID, artifact digest or other concrete evidence before retrying.
5. For a driven ceremony, inspect the same MADE instance and its claim/fence, state and enabled transitions. The driver can reconcile some interrupted advances; unresolved claims, multiple enabled transitions or missing state require authorized operator recovery. Do not manually edit session sidecars or start a duplicate instance to bypass a conflict.
6. Resume only when the current state supports the next action. Verify the resulting artifact, terminal ceremony state and memory receipt separately.

7. For a self-repair the agent requested, open `/repair`. An `interrupted` record is recovered with `r`: the console reloads the repair session, opens a workbench on the same clone and continues from the MADE instance and the pull request already recorded, so nothing is proposed or merged twice. A `blocked` or `failed` record keeps its clone, session and pull request for a hand-made follow-up (`axlr-tui --root <clone> --session <session>`). A `completed` record changed the repository, not the running console.

Switching work mode is not a MADE cancellation command. An active ceremony record can remain attached to the session; resolve it through MADE rather than assuming `/normal` erased it. Context compaction is also not a handoff: earlier evidence remains in session history. Read smaller source pages and recover earlier turns with `axlr_history`; current-turn tool results cannot be reread through that control.

## Update local KMP and MADE

Preconditions: no active task or pending call; known store paths and identities; backups suitable for the installed engines. Record the current AXLR version, engine versions, `mcp.json` and manifest references.

1. Run `/update`. AXLR checks the latest stable official packages and verifies package SHA-256, plugin version and engine version.
2. Review each engine's result. Only persistent local registrations named exactly `kmp` and `made` without custom arguments are eligible. Unconfigured, remote and command-line-only connections are skipped. The updater supports Linux amd64/arm64 and macOS arm64; this is narrower than AXLR's own release matrix.
3. AXLR changes the persistent manifest reference while keeping prior packages/manifests, explicit environments, stores, keys, allowlists and approvals. It does not bootstrap authorization, publish definitions or migrate a store.
4. Restart AXLR. Existing live connections keep their old executable until restart.
5. Verify discovery, selected store/identity, KMP known-memory recall and MADE pinned definitions. Re-run the appropriate engine runbook if setup needs repair.

For rollback, stop AXLR, restore the prior manifest reference and associated configuration from the private backup, and restart. Do not assume an older engine can open data written by a newer version: use the engine's documented compatible restore procedure if its storage format changed. `/update` does not provide a storage downgrade.

## Back up and restore

| State | Location / owner | Preserve |
|:--|:--|:--|
| Console sessions | `$XDG_STATE_HOME/axlr`, default `$HOME/.local/state/axlr` | Sessions, their sidecars, session labels and legacy preferences if still used |
| Console configuration | `$XDG_CONFIG_HOME/axlr`, default `$HOME/.config/axlr`; possibly custom MCP path | `settings.json`, MCP configuration and referenced private manifests |
| AXLR packages/engines | `$XDG_DATA_HOME/axlr`, default `$HOME/.local/share/axlr` | Exact versions and absolute manifest paths, or reproducible sources |
| HTTP service | Configured `state_dir` / state PVC | Sessions, event journals, tool-call records, idempotency and audit together |
| Workspace | User or service workspace | Worktree and artifacts needed to resume, including uncommitted changes |
| KMP / MADE | Selected engine stores, owned by their operators | Engine-native backup plus matching identities/configuration; MADE stable cursor key |

Stop writers or use each engine's supported online backup. Do not copy only a live SQLite main file and assume its WAL state was captured. Preserve private permissions and keep secrets outside shared artifacts. AXLR does not provide a unified backup command for all three products.

Restore to an isolated workspace/account first, keep original data intact, and verify paths, permissions, versions and store identities. Start a single writer, inspect sessions and engine state, and perform read-only recall/discovery before allowing effects. A restore rehearsal succeeds when the expected session and known engine records are readable and no interrupted operation has been silently replayed.

For HTTP idempotency, SSE reconnect and `uncertain` direct calls, use [service operations](service.md). For a real responsibility transfer, use the [handoff contract](../ceremonies.md#handoff-contract).
