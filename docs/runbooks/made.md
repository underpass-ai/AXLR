# Runbook: connect and prepare MADE

MADE owns ceremony definitions, claims, state transitions and decision records. AXLR performs the work. Use the exact MCP registration ID `made` for console-driven ceremonies and setup. A built-in `/plugin` row is not proof of connectivity or authorization.

The distribution locks MADE 0.9.1 in [engines.lock.json](../../distribution/engines.lock.json). Use a release-matched binary, plugin assets and the [upstream embedded setup](https://github.com/underpass-ai/made/blob/main/docs/embedded/README.md) for engine installation and store lifecycle. Record the binary version, SQLite path, policy, trusted-host identity and stable cursor key without exposing the key.

## Choose the connection route

| Route | Setup owner | AXLR preparation |
|:--|:--|:--|
| Installed `run-embedded-mcp.sh` launcher | Upstream embedded setup has bootstrapped the intended store and its private configuration | `/mcp` → select `made` → `P` can issue the work grant and publish the three driven definitions |
| Direct `made-mcp` binary | Operator supplies store, policy, work identity and cursor key | Operator must issue the work grant and publish the exact definitions |
| Remote engine | Remote operator owns authorization and definitions | `P` reports remote; it does not administer that engine |

AXLR's preparer recognizes the launcher by the executable basename `run-embedded-mcp.sh`. A directly registered binary, even in embedded mode, is not that route. Neither `/update` nor installing a package bootstraps an engine store.

## Register the installed embedded launcher

After completing upstream setup, save a private AXLR manifest at an absolute path, replacing the launcher path with the one actually installed:

```json
{
  "manifest_version": 1,
  "id": "made",
  "command": "/absolute/path/to/made-plugin/scripts/run-embedded-mcp.sh",
  "args": [],
  "allow_tools": ["*"]
}
```

Add this entry to the private `mcp.json` `plugins` array, preserving other entries and file mode `0600`. The top-level shape is `{"version":1,"plugins":[...]}`:

```json
{
  "manifest": "/absolute/path/to/made.json",
  "name": "MADE",
  "purpose": "ceremony",
  "approval": "manual",
  "env": {
    "MADE_MCP_BACKEND": "embedded",
    "MADE_MCP_STORE_PATH": "/absolute/path/to/ceremonies.sqlite3"
  },
  "env_from": {"HOME": "HOME", "PATH": "PATH"}
}
```

The launcher needs its matching plugin files, shell utilities and already initialized store configuration. These explicit variables are the child's complete environment; add any required XDG selection from your installation. Initially omitting `MADE_AUTH_TRUSTED_HOST_ID` lets the launcher resolve its trusted host for setup. Complete preparation before giving ordinary work to the agent; after preparation AXLR persists a separate work identity.

Restart AXLR and verify discovery in `/mcp`. Avoid duplicate package-generated registrations: their IDs are normally `<package>-<server>` and are not the driver's `made` identity.

## Prepare the driven ceremonies

When the console has no active turn or pending calls, select MADE in `/mcp` and press `P`:

1. AXLR invokes the embedded launcher with the trusted-host override removed and issues `axlr-default-work-v1` to a work identity. It recognizes existing `axlr-work-…` or `…-axlr-work` identities; a different explicit trusted-host override requires manual operator setup.
2. If needed, it persists an `axlr-work-…` identity in `mcp.json`. Restart when requested so the running connection uses it.
3. It verifies the work identity can read definitions and compares the published digests of `axlr_debug` 2.0, `axlr_delivery` 2.0, `axlr_incident` 1.0, `axlr_repair` 1.0 and `axlr_improve` 1.0 with the [shipped pins](../../tui/adapters/ceremonyhost/definitions.go).
4. It publishes missing definitions with a temporary five-minute install grant, then revokes that grant. Conflicting content under an existing immutable name/version is an error, not an overwrite.

The permanent grant allows running, inspecting and resuming published ceremonies; it excludes guard approval, definition publication and grant administration. Its exact action set lives in [work_grant.go](../../tui/adapters/madesetup/work_grant.go). The setup action itself makes privileged engine calls; a manual MCP policy does not turn those setup calls into individual agent approval dialogs.

Preparation is repeatable for matching identities and definitions. If it partially fails, inspect the displayed error, persisted work identity, published versions and temporary grant before retrying. If revocation failed, the install grant expires after five minutes; report and verify its state rather than assuming it was removed.

`P` installs only the seven driver definitions: debug/delivery **2.0**, and incident, repair, plan, task and sync **1.0**. It also prepares a separate approver identity for the incident and merge approval cards; the work identity cannot approve those guards. The seven **1.0** skill definitions are installed separately on an explicit request. See [ceremonies](../ceremonies.md).

## Direct binary or remote operator setup

For a directly managed embedded binary, initialize authorization with the matching engine and the intended store:

```bash
/absolute/path/to/made-mcp bootstrap-authorization /absolute/path/to/ceremonies.sqlite3 \
  --policy-id my-policy --trusted-host-id my-local-host
```

Use the trusted operator channel to grant a separate identity such as `my-axlr-work` the exact permanent action set linked above, and to publish the exact pinned YAML from [the driver definitions directory](../../tui/adapters/ceremonyhost/definitions/). Compare returned digests. Keep approval and grant administration with the operator.

For the console's incident approval card, use an AXLR-recognized work identity (`axlr-work-…` or `…-axlr-work`) and grant `<work-identity>-approver` only `approve_ceremony_guard` and `get_ceremony_instance`, matching [approver.go](../../tui/adapters/madesetup/approver.go). The console invokes that identity only for the person's approval; the agent connection retains the work identity. Remote incident approval needs an operator integration: the shipped card approver invokes a separately configured local process.

Use the same AXLR manifest as above with `command` set to the absolute binary. Its configuration entry supplies the work identity:

```json
{
  "manifest": "/absolute/path/to/made.json",
  "name": "MADE",
  "purpose": "ceremony",
  "approval": "manual",
  "env": {
    "MADE_MCP_BACKEND": "embedded",
    "MADE_MCP_STORE_PATH": "/absolute/path/to/ceremonies.sqlite3",
    "MADE_AUTH_POLICY_ID": "my-policy",
    "MADE_AUTH_TRUSTED_HOST_ID": "my-axlr-work",
    "MADE_CEREMONY_STORE_ID": "my-local-store"
  },
  "env_from": {
    "MADE_CEREMONY_SEARCH_CURSOR_HMAC_KEY": "AXLR_MADE_CURSOR_HMAC_KEY"
  }
}
```

`AXLR_MADE_CURSOR_HMAC_KEY` must be present in AXLR's environment, contain the initialized store's stable key and remain private. Do not generate a new key each launch. Discovery alone does not test the work grant. For remote engines, the operator performs the equivalent grant and definition checks using the remote deployment's identities; the [service configuration](../api.md) uses gRPC mTLS adapters.

## Verify actual execution

1. Confirm `made` discovery and work-identity access to the three exact pinned definitions.
2. Select `/delivery` and send a small reversible task with a meaningful check. Observe a new instance and the brief step.
3. Review the proposed command. AXLR executes the baseline and subsequent verification itself; build cannot replace the approved command.
4. Confirm the terminal state, report, command result and revision/dirty evidence. `COMPLETED` and `BLOCKED` both return the console to normal.
5. If KMP is connected as `kmp`, inspect the memory outcome. `not recorded` means the ceremony result exists in MADE but its KMP write was not established.

Success is actual work plus an observed check and recorded state, not a role label or successful no-op handler. See the [agent workflow](agent-workflow.md).

## Disconnect MADE from AXLR

1. Finish or interrupt active ceremonies and AXLR turns. Record the current SQLite path, policy ID and host ID so the same state can be reconnected later.
2. Stop AXLR and back up its private `mcp.json`. Remove only the `plugins` array entry whose manifest is your MADE manifest. Preserve other entries and the private file mode. Remove any one-launch `--plugin` flag or duplicate package registration that points to MADE.
3. Restart AXLR and verify that `/mcp` no longer lists `made`. The built-in `/plugin` catalogue row can remain. AXLR session transcripts remain, but ceremony calls are unavailable. The MADE SQLite store and authorization policy are untouched.
4. If removing the machine's engine as well, follow MADE's lifecycle for the installation method you used after checking other hosts. Archive the SQLite store and cursor key together if future restoration is possible. Disconnecting AXLR does not delete them.

## Recovery

| Symptom | Action |
|:--|:--|
| Discovery fails | Check absolute executable/launcher, explicit environment, store, policy, cursor key and stderr |
| `P` reports unsupported or remote | Use the operator route; do not remove a deliberate identity to gain privileges |
| Definition absent | Prepare/publish the exact pinned driver definition; the agent catalogue does not satisfy a different driver pin |
| Digest or grant conflict | Compare the stored identity and action/content set; never overwrite an immutable identity |
| Lost completion/transition response | Inspect the existing instance, active claim/fence and enabled transitions before continuing |
| Multiple enabled transitions or stale claim | Resolve the state through the authorized MADE operator; do not start a duplicate instance |
| `BLOCKED` after checks | Preserve failing evidence; revisit the task/check deliberately rather than silently resetting the loop |

The [recovery runbook](recovery.md) covers restart, backups and uncertain effects. Keep the store and key together for restoration; reconnecting an empty database is not recovery.
