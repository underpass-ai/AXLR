# AXLR provenance

AXLR is Underpass's focused rewrite of the execution layer in [underpass-ai/underpass-runtime](https://github.com/underpass-ai/underpass-runtime). The source baseline for that decision was commit [`c16098cc7e863468f244963d5874544f8ce12f61`](https://github.com/underpass-ai/underpass-runtime/tree/c16098cc7e863468f244963d5874544f8ce12f61). This page records responsibility and contract lineage; it is not a compatibility promise or a claim that old packages were copied.

| Earlier concept | AXLR decision |
|:--|:--|
| [`InvokeToolRequest.CorrelationID`](https://github.com/underpass-ai/underpass-runtime/blob/c16098cc7e863468f244963d5874544f8ce12f61/internal/app/types.go) | Keep correlation as `request_id`. It does not provide session identity or deduplication. |
| `CommandSpec`, `CommandResult` and `CommandRunner` in the same file | Use typed domain commands, application ports and explicit results. |
| [`LocalCommandRunner`](https://github.com/underpass-ai/underpass-runtime/blob/c16098cc7e863468f244963d5874544f8ce12f61/internal/adapters/tools/runner.go) | Bound process-output capture while reading and count discarded bytes; the old `CombinedOutput()` then truncate pattern did not bound capture memory. |
| Service, policy, session, registry, store and infrastructure layers | Keep the root execution core small. The current console owns its session and approval UI; a planned HTTP service will reuse the same agent cases and add its own transport boundary. |

Pi was a conceptual reference for a small direct local tool surface. AXLR does not use Pi code, packages, runtime or an adapter to it.

The resulting product has three responsibilities: AXLR performs agent execution; KMP governs durable memory; MADE governs orchestration. KMP and MADE are separate engines reached through explicitly configured MCP connections. The current worker does not serve HTTP; the [service specification](specs/axlr-service-api.md) records that planned interface.

See [Architecture](architecture.md) for the current package boundaries and [Worker contract](worker.md) for the current JSON surface.
