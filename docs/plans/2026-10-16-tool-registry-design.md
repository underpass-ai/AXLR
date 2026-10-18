# Pre-forged tools and downloadable ceremonies: registry, web and install design

Status: proposed (16 Oct 2026). Companion catalogs: [pre-forged tools](2026-10-16-prebuilt-tools-catalog.md) and [downloadable ceremonies](2026-10-16-ceremony-catalog.md). Nothing here was implemented when it was written; the reference entries under [`catalog/`](../../catalog/) proved the manifest shapes only.

Update, 18 Oct 2026 (v0.11.0): decisions D1 to D4 are implemented in the console (#137): signed-index install, `/tools`, `axlr_catalog`, `axlr_install_tool` and publishing to the registry, documented in [installing catalog tools](../console.md#installing-catalog-tools) and [publishing a tool to a catalog](../console.md#publishing-a-tool-to-a-catalog). The official catalog at `https://registry.underpassai.com/index.json` serves the 159 tools under [`catalog/tools/`](../../catalog/tools/). Not implemented yet: installing ceremonies from a catalog (D5; the console ignores an index's `ceremonies`), and open mechanism 5, honouring an index's `revocations` and `revoked_keys` and verifying the publisher's signature on install. The text below is the design as proposed.

## Goal

A public AXLR catalog site serves two kinds of content a person installs into their console from the console itself:

- **Pre-forged tools**: small programs in the exact shape of a [forged tool](../console.md#forged-tools) (`name`, `description`, `input_schema`, `program`, `args`, one to eight `files`), written once, tested and published, so the model does not have to write D'Hondt seat allocation or business-day arithmetic for the hundredth time. They are not only programming tools: the catalog covers mathematics, research, writing, politics and civics, administration, finance, law, education, language, geography, health and media.
- **Downloadable ceremonies**: MADE definitions (the YAML the console already pins under `tui/adapters/ceremonyhost/definitions/`) that package state-of-the-art agentic orchestrations and high-value procedures, driven by the existing generic loop of [`/ceremony`](../ceremonies.md#any-published-definition-ceremony).

Both are **registered, not scanned**, exactly as forged tools are today: a download changes nothing until the person approves a card that shows what will run, and the console records what it approved.

## What already exists and is reused

| Existing piece | Reused for |
|:--|:--|
| `ForgedTool` (`tui/application/forged_tool.go`) and the per-workspace registry `<state>/axlr/forged-tools/<digest>.json` with the SHA-256 of each file | An installed tool is a forged tool with an `origin`; it runs through `axlr_run_tool` unchanged and `axlr_tools` lists it |
| The forge card (`approval_forged.go`): command, description, schema and every file as code; approved as `local_write` | The install card is the forge card plus the origin line and the verified digests |
| `/update` with `distribution/engines.lock.json`: pinned URL + SHA-256 per artifact, verified before anything changes | The catalog index is the same idea generalised: one signed index, per-file digests |
| `/ceremony publish <file.yaml>` under the five-minute install grant, and the generic loop (`ceremony_generic.go`) that drives any published definition | A ceremony install is a verified download followed by the same publication |
| `tools/ceremonies/check_pins.py` and `spike_drafts.py` | The catalog repository's CI validates and walks every ceremony it serves |
| `docs/brand.md` palette and the pixel wordmark renderer | The site's identity |

## Decisions

### D1. The registry is a static, signed index, not a service

The catalog is a Git repository (`underpass-ai/axlr-catalog`) published as a static site. The console reads one file, `index.json`, plus the files of what it installs. There is no API, no account and no telemetry: a static index can be mirrored, pinned to a commit, served from an intranet and audited by reading it. Homebrew taps, Go module proxies and the MCP registry's `server.json` all converged on "manifest plus content hash", and `engines.lock.json` already follows it.

The index is signed with an Ed25519 key in minisign format. The service that produces it (publisher keys and scoped tokens, upload, policy scan, sandboxed tests, human review, yanks and the signed index) is the sibling project `axlr-registry`, in its own repository; its `docs/domain.md` is the write-side design, and this document stays the read side. The two share only the index format. AXLR ships the public key of the official catalog; a settings entry adds other catalogs with their own keys:

```json
"catalogs": [
  {"name": "axlr", "url": "https://registry.underpassai.com/index.json", "public_key": "RWQ…"},
  {"name": "acme", "url": "https://tools.intranet.acme/index.json", "public_key": "RWQ…"}
]
```

The official entry is built in and cannot be removed, only disabled. A catalog without a key is refused: unsigned indexes are how a compromised CDN becomes a compromised workspace.

### D2. Index schema

```json
{
  "schema_version": 1,
  "catalog": "axlr",
  "generated_at": "2026-10-16T10:00:00Z",
  "tools": [
    {
      "name": "seat_allocation",
      "version": "1.0.0",
      "category": "politics_civics",
      "tags": ["elections", "dhondt", "sainte-lague", "hare"],
      "summary": "Allocate seats among parties by D'Hondt, Sainte-Laguë or Hare quota, with an optional threshold.",
      "program": "python3",
      "requires": {"program_min_version": "3.10", "packages": []},
      "network": false,
      "license": "MIT",
      "manifest": "tools/politics_civics/seat_allocation/manifest.json",
      "manifest_sha256": "…",
      "files": {"seat_allocation.py": "…sha256…"},
      "tested_with": ["claude-haiku-5.5", "qwen3.8-27b"],
      "updated_at": "2026-10-16"
    }
  ],
  "ceremonies": [
    {
      "name": "draft_critique_revise",
      "version": "1.0",
      "domain": "writing",
      "pattern": "maker_checker",
      "summary": "Write, critique against named criteria and revise, at most three rounds, then deliver or escalate.",
      "definition": "ceremonies/writing/draft_critique_revise-1.0.yaml",
      "sha256": "…",
      "inputs": {"required": ["brief"], "optional": ["memory_about"]},
      "guards": ["step_status", "output_field", "step_repeat_exhausted"],
      "approvals": "none",
      "made_min_version": "0.10.0"
    }
  ]
}
```

Rules:

- `name` follows the forged-tool name rules (`[a-z][a-z0-9_]{1,63}`) and is unique per catalog. A tool installed from catalog `acme` that collides with a local forged tool is refused; the person removes one first.
- `version` is semver for tools and `major.minor` for ceremonies (MADE's own rule).
- `files` carries the SHA-256 of every file the manifest names; the manifest itself has a digest; the index is signed. Three layers so that a single tampered file, manifest or index is caught.
- `requires.packages` is informative. Tier 1 of the catalog is stdlib-only (Python 3.10+, bash with coreutils, Node 22 with no npm packages) because a forged tool runs with only `PATH` and `HOME`; a tool that needs a package says so and the install card warns when `program -c "import x"` fails.
- `network: true` marks a tool that reaches the network; under the exec sandbox it may not work and the card says so.
- Secrets are **not** supported: the forged-tool environment has no way to pass an API key, and a key in a file is a key in the forge card. Tools that need one wait for a `secrets` design (see [open mechanisms](#open-mechanisms)).

### D3. Per-tool manifest

`tools/<category>/<name>/manifest.json` is a `ForgedTool` with catalog metadata, so a console can register it without transformation:

```json
{
  "name": "seat_allocation",
  "description": "Allocate seats among parties …",
  "input_schema": {"type": "object", "required": ["seats", "votes"], "properties": {…}},
  "program": "python3",
  "args": ["seat_allocation.py"],
  "files": ["seat_allocation.py"],
  "catalog": {
    "version": "1.0.0", "category": "politics_civics", "tags": […], "license": "MIT",
    "requires": {"program_min_version": "3.10", "packages": []}, "network": false,
    "example": {"input": {…}, "output": {…}},
    "source": "original; method references in README.md"
  }
}
```

Beside it: the files, a `README.md` (what it does, the method, its limits) and `tests.json`, a list of `{input, expect: {exit, stdout_json | stdout_contains}}` cases the catalog's CI runs with the same stdin contract as `axlr_run_tool`.

### D4. Installing a tool: two entry points, one card

- **The person**: `/tools` lists installed and catalog tools; `/tools search <terms>`; `/tools install <name>[@version] [--from <catalog>]`; `/tools update [name]`; `/tools remove <name>` (same as `axlr_remove_tool`). Spanish aliases as the other commands.
- **The model**: `axlr_catalog` (read-only host tool, no approval: `search` and `describe` over the cached index, so the model looks in the catalog before forging) and `axlr_install_tool` `{name, version?, catalog?}`, judged and approved as `local_write` like `axlr_forge_tool`. The model installs only on a card; it never forges a copy of a catalog tool to skip the card.

The flow, whoever starts it:

1. Fetch `index.json` if the cache under `<state>/axlr/catalog/<catalog>/` is older than a day (or on `/tools update`); verify the signature; refuse on failure and keep the old cache.
2. Resolve name and version; fetch the manifest and its files into a temporary directory under `<state>`, never the workspace; verify every digest against the index.
3. Check `program` on `PATH` and `requires`; collect warnings.
4. Show the **install card**: the forge card with the origin (`catalog axlr · seat_allocation 1.0.0 · verified`), the warnings, and every file as code. The person reads what will run, exactly as with a forged tool.
5. On approval: write the files to `.axlr/tools/<name>/`, register the tool with `origin: {catalog, version, index_generated_at}` and the file digests, in the same per-workspace registry. Every later run is a `local_exec` as today; a changed file breaks it as today.

`/tools update` lists the installed tools whose catalog version is newer and installs each one on its own card. There is no automatic update: an update is new code, and new code is approved.

### D5. Installing a ceremony

`/ceremony install <name>[@version]` and the model's `axlr_install_ceremony` (approved as `local_write`): fetch, verify the digest, show the definition (the YAML as code, plus the summary of states, steps, repeat caps and guard types), and on approval publish it through the same path as `/ceremony publish`, under the same five-minute install grant. The published definition then appears in `/ceremony` and runs through the generic loop. `/ceremony` marks catalog definitions with their origin and version; a newer catalog version is a new publication, never an in-place change, because MADE definitions are immutable once published.

Catalog ceremonies must run inside what the generic loop supports today:

- Every step is a `host_callback` the session's model works; `num_agents` above one is not fanned out by the console (it works the step once), so the catalog says `num_agents: 1` and expresses independent voices as sequential steps with `see_prior: false`, all worked by the same model. Every definition declares the `AXLR` role with its steps and triggers, or MADE refuses the first claim.
- Guards are automated: `step_status`, `output_field` and `step_repeat_exhausted`. A `type: human` guard makes the generic loop wait for an approval the console cannot give, so catalog definitions model a decision as a step whose prompt tells the model to put the question to the person and hand back `decision`. This is weaker than the console's approval card and is stated on each ceremony's page.
- Check commands are not run by the generic loop; a ceremony that needs "a command exits zero" asks the model to run it with `local_exec` and report, which the person approves as usual.

Those three limits are the first console work the catalog asks for (see [open mechanisms](#open-mechanisms)).

### D6. The site

A static site generated from the repository on every merge (the pixel wordmark renderer exports the logos; the palette is `brand.md`'s): a home page with the categories, a page per category, a page per tool (summary, method, schema rendered as a form-like table, example input and output, the files as code, the `/tools install` line, versions, license, tested models) and a page per ceremony (summary, a state diagram generated from the YAML, the steps and their prompts, guard types, where the person decides, the `/ceremony install` line). Client-side search over the index. No accounts, no comments; feedback is an issue on the repository.

### D7. Contribution and CI

A tool or ceremony is a pull request to the catalog repository. CI:

- validates every manifest against the schema and the forged-tool name and `args` rules;
- runs every `tests.json` under bubblewrap with only `PATH` and `HOME`, for each `program` the manifest allows;
- rejects a tool whose files import a package not in `requires.packages`, and any tool with `network: false` that opens a socket in tests;
- validates and publishes each ceremony into a disposable `made-mcp` store and walks its happy and blocked paths (`tools/catalog/walk_ceremonies.py`, built on `check_pins.py` and `spike_drafts.py`);
- rebuilds `index.json`, signs it with the release key held in CI, and publishes the site.

A tool is accepted with at least three test cases, a README that names the method and its limits, and a licence compatible with MIT or Apache-2.0.

### D8. What is deliberately not done

- No execution on the server, no "run this tool in the browser".
- No ranking, stars or download counts in the first version.
- No automatic installation when the model finds a matching tool: the model proposes, the person approves.
- No skills: the retired `axlr-ceremonies` skill catalogue (5 Oct 2026) showed that a protocol the model must follow by reading costs more than real models can pay; a downloaded ceremony is driven by the console.

## Open mechanisms

These are console changes the catalog depends on, in order of value:

1. **Install registration path** (D4 steps 4–5): `axlr_install_tool`, `/tools`, the install card, `origin` in the registry entry. Without it there is no install; the fallback is the person reading the site and the model forging the files by hand, which is what the catalog exists to avoid.
2. **Signed index verification and the catalog cache**: minisign verification in Go (an Ed25519 signature over the index bytes), the cache directory, `catalogs` in settings.
3. **Generic-loop guards for catalog ceremonies**: an `x-axlr` step annotation the console honours in the generic loop: `approval: true` shows the approval card and records `decision` itself; `check: true` runs the model's `check_command` and records `passed`; `fan_out: n` works a step `n` times with fresh context and hands the outputs to the next step. Until then catalog ceremonies are written within D5's limits.
4. **Secrets for tools**: a `secrets: ["NAME"]` manifest field resolved from `credentials.json` and passed as environment to that run only, shown by name on the run card. Tier 2 of the catalog (anything that calls an API) waits for it.
5. **Registry revocations on the console**: honour `revocations` and `revoked_keys` in an index (mark the installed version `broken` with the reason, refuse to run it, offer the newer one), show the publisher and reviewer on the install card, and verify the publisher signature over the bundle digest beside the index signature. Until then the registry's guarantees stop at the index.
6. **Per-tool always-allow**: already listed as not done in the forged-tools design; catalog tools, being reviewed, are the natural first beneficiaries.
