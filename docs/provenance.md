# AXLR provenance

AXLR is Underpass's own focused rewrite of the execution layer in [underpass-ai/underpass-runtime](https://github.com/underpass-ai/underpass-runtime). The source baseline reviewed for this first delivery is commit [`c16098cc7e863468f244963d5874544f8ce12f61`](https://github.com/underpass-ai/underpass-runtime/tree/c16098cc7e863468f244963d5874544f8ce12f61), which was still the source repository's `main` when this note was written.

| Prior runtime concept | AXLR decision |
| --- | --- |
| [`InvokeToolRequest.CorrelationID`](https://github.com/underpass-ai/underpass-runtime/blob/c16098cc7e863468f244963d5874544f8ce12f61/internal/app/types.go) | Preserve correlation as `request_id`; do not inherit session or approval fields. |
| `CommandSpec`, `CommandResult`, and `CommandRunner` in the same file | Preserve the command and result concept through a typed domain command, process port, and result. |
| [`LocalCommandRunner`](https://github.com/underpass-ai/underpass-runtime/blob/c16098cc7e863468f244963d5874544f8ce12f61/internal/adapters/tools/runner.go) | Reimplement local execution with a combined output budget applied during capture. The old `CombinedOutput()` then truncate approach did not bound capture memory. |
| The broader service, policy, session, registry, store, and infrastructure graph | Keep outside the AXLR core. Its host supplies workspace, authority, and business policy. |

This is **contract and responsibility lineage**, not a copy of the old packages or a compatibility claim. The new code was written for AXLR's smaller contract. Pi was a conceptual reference for keeping a direct four-tool surface; AXLR does not use Pi code, packages, runtime, or an adapter to it.
