# Runbook: connect or remove MADE orchestration

MADE governs AXLR's orchestration layer: ceremony definitions, claims, state transitions and human decisions. AXLR executes the local work and calls MADE over MCP. The built-in MADE row in `/plugin` does not install the engine or prove connectivity; `/mcp` shows the actual server.

This runbook uses a directly managed embedded `made-mcp` stdio process. Follow the [MADE local engine guide](https://github.com/underpass-ai/made/blob/main/docs/embedded/README.md) for release-matched binaries, authorization and store lifecycle. Use one MADE registration per AXLR host.

AXLR bundles a routing skill and seven [default ceremonies](../ceremonies.md), including accountable handoff. They are readable before connection, and do not automatically publish into this store. After connecting, request catalogue installation to validate and publish the exact pinned definitions. Keep guard-approval and grant-administration capabilities separate from the work-agent principal; see the permission boundary in that guide.

## Install and connect

1. Install the release-matched `made-mcp` binary for your platform and verify `made-mcp --version`. The published release checked on 1 October 2026 is [0.9.1](https://github.com/underpass-ai/made/releases/tag/v0.9.1), also available through `cargo install made-mcp --version 0.9.1 --locked`. The upstream local guide describes the 0.9.0 setup contract; use the matching 0.9.1 binary and its checksummed release assets. Recheck the release instructions when upgrading.
2. Choose an absolute SQLite store path and stable, nonempty policy, trusted-host and store IDs. Generate a private 32-byte cursor HMAC key as 64 hexadecimal characters. Keep the key stable across restarts, in a secret manager or environment variable; do not commit it. Bootstrap authorization for the **same store and IDs** with the matching binary:

```bash
made-mcp bootstrap-authorization /absolute/path/to/ceremonies.sqlite3 \
  --policy-id my-policy \
  --trusted-host-id my-local-host
```

Bootstrap creates the administrative owner; it does not grant every ceremony action. Issue only the required grants through MADE's authorization API.

3. Save this manifest as a private file at an absolute path, for example `$HOME/.config/axlr/made.json`, replacing the command path:

```json
{
  "manifest_version": 1,
  "id": "made",
  "command": "/absolute/path/to/made-mcp",
  "args": [],
  "allow_tools": ["*"]
}
```

4. Add one entry to AXLR's private `$XDG_CONFIG_HOME/axlr/mcp.json` (or `$HOME/.config/axlr/mcp.json`) `plugins` array, preserving existing entries and file mode `0600`. Use `{"version":1,"plugins":[...]}` as the top-level shape if this is the first server:

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
    "MADE_AUTH_TRUSTED_HOST_ID": "my-local-host",
    "MADE_CEREMONY_STORE_ID": "my-local-store"
  },
  "env_from": {
    "MADE_CEREMONY_SEARCH_CURSOR_HMAC_KEY": "AXLR_MADE_CURSOR_HMAC_KEY"
  }
}
```

`AXLR_MADE_CURSOR_HMAC_KEY` must be present in the AXLR host environment when it starts. `env` and `env_from` form the complete environment of the child process. The placeholder IDs and paths above must match the authorization bootstrap. AXLR rejects duplicate server IDs; do not also register MADE from a package.

5. Restart AXLR, open `/mcp` and verify the `made` tools. Keep manual approval until you have reviewed the exact tools and grants. A discovered MADE step handler still requires real host execution capability in AXLR; discovering it does not perform the step.

6. Give AXLR its own work identity. With the embedded launcher, select the MADE row in `/mcp` and press `P` (prepare for AXLR). AXLR starts MADE once as the store's trusted host and issues grant `axlr-default-work-v1` to a work identity that can run, inspect and resume published ceremonies but cannot approve human guards, publish definitions or change grants. If the MADE entry had no `MADE_AUTH_TRUSTED_HOST_ID`, AXLR was acting as the trusted host itself: preparation writes an `axlr-work-…` identity to the entry and asks you to restart. Running it again is harmless. With a remote (gRPC) engine or an entry that names an explicit trusted host, as in the example above, AXLR changes nothing; issue the same grant as that operator.

## Disconnect MADE from AXLR

1. Finish or interrupt active ceremonies and AXLR turns. Record the current SQLite path, policy ID and host ID so the same state can be reconnected later.
2. Stop AXLR and back up its private `mcp.json`. Remove only the `plugins` array entry whose manifest is your MADE manifest. Preserve other entries and the private file mode. Remove any one-launch `--plugin` flag or duplicate package registration that points to MADE.
3. Restart AXLR and verify that `/mcp` no longer lists `made`. The built-in `/plugin` catalogue row can remain. AXLR session transcripts remain, but ceremony calls are unavailable. The MADE SQLite store and authorization policy are untouched.
4. If removing the machine's engine as well, follow MADE's lifecycle for the installation method you used after checking other hosts. Archive the SQLite store and cursor key together if future restoration is possible. Disconnecting AXLR does not delete them.

## Recovery

If discovery fails, check the binary version, absolute store path, exact bootstrap policy and trusted-host IDs, stable cursor key, grants and process diagnostics on stderr. MADE refuses startup with a missing policy or unbootstrapped store; do not point it at a new empty database to hide that failure. Retry a ceremony step only after checking its recorded state, because a timeout cannot prove that the step did not run.
