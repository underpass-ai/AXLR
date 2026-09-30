# Release status and plan

AXLR currently builds from source and its checked-in CI tests the two Go modules on Linux. There is no tagged cross-platform release workflow or Helm chart in this repository yet. Do not interpret a successful cross-compile as a tested runtime on that platform.

The [implementation plan](plans/axlr-service-helm-release.md#6-ci-y-versiones-tomando-made-como-referencia) defines the next release pipeline, based on MADE's separation between read-only build jobs and a tag-only publishing job. It covers six OS/architecture archives (Linux, macOS and Windows on amd64 and arm64), native smoke tests, SHA-256 checksums, a multiarch service image, a matching Helm chart and exact asset inventory checks. The service binary and chart join that pipeline only when their implementation and tests exist.

Until then, use the [getting started](getting-started.md) source-build commands and verify both Go modules locally. The [service API specification](specs/axlr-service-api.md) is future design, not a released endpoint.
