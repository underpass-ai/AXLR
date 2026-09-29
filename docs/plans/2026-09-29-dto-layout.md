# Source package layout

No Go source should live at the repository root. The one-request JSON worker contract belongs to an adapter boundary. Move its ten request, argument, output and failure DTO types into a top-level `dto/` package with one primary type per file. Move the remaining root Go files, including tests, into `runtime/`.

The `runtime/` package owns orchestration, codecs and mappers. Its public method signatures name `dto.Request` and `dto.Response`; the CLI imports `runtime` and `dto` directly. `domain/` and `application/` remain independent of wire types. The JSON field names and runtime behavior must stay byte-for-byte compatible. As AXLR has no released module version yet, avoid root type aliases that would leave Go source in the root.

Verification: a DTO wire-contract test first, then the existing root tests, worker tests, race detector, aggregate coverage above 80%, static build, MCP module tests, and fast CI on a PR based on current `main`.
