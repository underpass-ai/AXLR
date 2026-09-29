# AXLR minimal runtime implementation

Source: our `underpass-ai/underpass-runtime` at commit `c16098cc7e863468f244963d5874544f8ce12f61` and `underpass-runtime-minimal-plan.zip`, treated as a design proposal. AXLR is a focused rewrite of our execution layer in a new repository. It does not promise compatibility with the old service API.

The architecture and quality requirements added during implementation are fixed in [2026-09-29-hexagonal-design.md](2026-09-29-hexagonal-design.md): hexagonal layers, value objects, one principal Go type per file, at least 80% aggregate test coverage, and minimal CI.

1. Define one strict, bounded JSON request and response contract and four typed argument forms.
2. Implement a serial library executor for `read`, `write`, `edit`, and `exec` in the Linux trusted-local profile.
3. Anchor file operations to the host workspace, publish writes from same-directory temporary files, and bound all input and output.
4. Add a one-request CLI worker with explicit root, profile and child environment.
5. Cover validation, file conflicts, UTF-8 pagination, process exit, output limits, timeout, cancellation and CLI framing with Go tests.
6. Verify format, tests, race detector, vet, static build and offline dependency graph; document the contract and limitations.

The Go package is the library API; `cmd/axlr` is the worker. The first release does not claim sandbox isolation or exactly-once effects. AXLR is Underpass's own runtime, with a selective contract lineage documented in [../provenance.md](../provenance.md). Comparative measurements require a separate pilot.
