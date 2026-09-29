# AXLR minimal runtime implementation

Source: `underpass-runtime-minimal-plan.zip`, treated as a design proposal. This branch implements the new AXLR repository, not the previous Underpass runtime.

The architecture and quality requirements added during implementation are fixed in [2026-09-29-hexagonal-design.md](2026-09-29-hexagonal-design.md): hexagonal layers, value objects, one principal Go type per file, at least 80% aggregate test coverage, and minimal CI.

1. Define one strict, bounded JSON request and response contract and four typed argument forms.
2. Implement a serial library executor for `read`, `write`, `edit`, and `exec` in the Linux trusted-local profile.
3. Anchor file operations to the host workspace, publish writes from same-directory temporary files, and bound all input and output.
4. Add a one-request CLI worker with explicit root, profile and child environment.
5. Cover validation, file conflicts, UTF-8 pagination, process exit, output limits, timeout, cancellation and CLI framing with Go tests.
6. Verify format, tests, race detector, vet, static build and offline dependency graph; document the contract and limitations.

The Go package is the library API; `cmd/axlr` is the worker. The first release does not claim sandbox isolation or exactly-once effects. Pi integration and comparative measurements require the consuming repository and a real pilot, so they follow the core contract.
