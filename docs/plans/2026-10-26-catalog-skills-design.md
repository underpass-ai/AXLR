# Skills in the catalog: versioned, signed, pinned per execution

Status: proposed (26 Oct 2026). Phase 3 of the [master plan](2026-10-23-master-plan.md) ("skills versioned in the catalog"); the vision's skill is "a reusable procedure, versioned and, preferably, verifiable". It extends the [registry design](2026-10-16-tool-registry-design.md), whose D8 left skills out because a protocol the model must follow by reading cost more than models could pay: a skill here is not a protocol the console drives, it is guidance the model opens on demand through `axlr_skill`, as the built-in and plugin skills already are. The write side is in `axlr-registry` (`docs/domain.md`, "Skills").

## What a skill entry is

A skill is a bundle like a tool's, with a manifest of another kind:

```json
{
  "kind": "skill",
  "name": "incident_triage",
  "description": "Triage a production incident from its logs: timeline, blast radius, first hypothesis, next check.",
  "files": ["SKILL.md", "checklist.md"],
  "tools": ["local_read", "local_search", "log_summarise"],
  "catalog": {"version": "1.0.0", "category": "programming", "tags": ["sre", "incident"], "license": "MIT", "source": "original"}
}
```

| Field | Rule |
|:--|:--|
| `kind` | `skill`; a manifest without `kind` is a tool, so every tool published so far keeps its meaning |
| `name` | The tool-name rule, `[a-z][a-z0-9_]{1,47}`, in the same name space as tools: a name is owned by one publisher for life, and is a tool or a skill for life. Namespaces (`acme_*`, `*`) apply unchanged |
| `description` | 1–1024 characters: what the console lists next to the name so the model knows when to open it |
| `files` | One to eight flat text files, at most 256 KiB each, `SKILL.md` among them. A skill carries no program: executable logic belongs in a catalog tool, which has its own card and is verified at every run |
| `tools` | Zero to 32 tools the procedure expects, as the model calls them (`local_read`, a catalog tool's name, an MCP tool as `server/tool`). Informative: the install card marks the ones this console lacks; nothing is installed with the skill |
| `catalog` | `version` (semver), `category`, `tags`, `license`, `source`, as a tool's; `requires`, `network` and `example` are refused |

`SKILL.md` may open with front matter (`name`, `description`, as the Agent Skills format writes it); when it does, its `name` must be the manifest's. Every relative link in `SKILL.md` must name one of the bundle's files.

**Signatures.** Exactly a tool's: the publisher signs `axlr-tool-bundle-v1\n<bundle digest>`, where the bundle digest covers the manifest (and with it `kind`) and every file; the registry signs the index with its minisign key. A manifest is one kind or none (unknown fields are refused), so a tool's signature cannot be replayed as a skill's.

**Test harness.** `tests.json` travels beside the bundle as for tools (the registry's evidence, not installed, not signed), and is optional. For a skill it is a list of model cases:

```json
[{"name": "starts_from_logs", "prompt": "The API answers 502 since 10:00; logs are in logs/api.log", "expect": {"tools": ["local_read"], "answer_contains": ["timeline"]}}]
```

The registry's runner has no network and no model, so it cannot run them. What it runs is deterministic and becomes the version's test report, which approval requires to pass: `SKILL.md` is present and UTF-8, its front matter agrees with the manifest, its relative links resolve to bundled files, every `tools` entry is a well-formed name, and `tests.json`, when present, is well formed (distinct names, a prompt, known `expect` keys). A skill without `tests.json` is approvable on those checks alone (a tool is not: its tests are its only evidence that it runs). Running the cases against a model is Phase 4 ("skills with test harnesses"): the registry will publish `tests.json` beside the files, and the console will run each case as an `--exec` with the skill pinned.

## Registry side (axlr-registry)

- The aggregate keeps its name (`Tool`, "a catalog entry"); `Kind` is read from the manifest, and `Tool::add` refuses a version of the other kind.
- Policy scan for a skill: the obfuscation rule (long base64 or hex blobs, `block`); there is no program, so the program rules do not apply. Binary and oversized files are refused by bundle validation as for tools.
- Storage: the same DynamoDB items and quarantine prefixes; published files under `published/skills/<name>/<version>/`.
- Index: `skills[]` beside `tools[]`, each entry the tool entry's provenance fields (`publisher`, `publisher_key_id`, `bundle_sha256`, `signature`, `reviewed_at`, `reviewer`, `warnings`) plus `tools`, without `program`, `requires` and `network`; `manifest` is `skills/<name>/<version>/manifest.json`. Yanks go to the shared `revocations` (a name is unique across kinds). Consoles before this change ignore `skills`.
- Routes: no new one. Publishers submit with `POST /v1/versions` and read their version with `GET /v1/versions/{name}/{version}` (which now says `kind`); reviewers list, show, approve, reject and yank as for tools; the console reads the static `index.json`, its signature, and `skills/<name>/<version>/…`.
- A local harness, `axlr-registry-local`, serves the publish API and the published files from memory on the loopback, runs the checks and tests inline, approves as a local reviewer (or waits for `POST /local/approve/{name}/{version}`) and signs each index with a key it generates, for end-to-end runs without AWS. It does not serve the admin API.

## Console side (AXLR)

**Installed skills** live per user, not per workspace (a procedure is not the workspace's code), every version side by side: `<state>/axlr/skills/<catalog>/<name>/<version>/` holds the files and `skill.json`, the record of the install (origin, index time, manifest, file and bundle digests, the tools it expects). Every read checks the file against the recorded digest, as a forged tool's run does.

| Person (console) | Program (`axlr-tui skills …`) | What it does |
|:--|:--|:--|
| `/skills` | `list` | Installed catalog skills with their versions, the catalogs' skills, and the newer versions available |
| `/skills search <terms>` | `search <terms>` | Name, tags, category and summary |
| `/skills install <name>[@version] [--from <catalog>]` | `install … --yes` | Stage (index signature, manifest and file digests), show the card (origin, publisher, reviewer, the tools it expects and which are missing, every file), install on approval |
| `/skills update [name]` | `update [name] --yes` | Install each newer version beside the old one; nothing is updated without a card |
| `/skills remove <name>[@version]` | `remove <name>[@version]` | One version, or every version |
| `/skills publish <name>` | `publish <name>` | Build the bundle from the workspace's `.axlr/skills/<name>/` (`SKILL.md` front matter for `description` and `tools`, `catalog.json` for the catalog block, `tests.json` beside), sign it with the client key and upload it; poll until scanned and tested |

The program's `install` has no card to approve, so it refuses without `--yes` and prints what it would install: the control plane's path to give a bot its skills.

**Pinning.** An execution names its skills, as it names its policy, budget and sandbox: `axlr-tui --exec --skills <file.json>` (and `--decide --skills`), `CreateSession` member `skills`, shown by `GetSession`.

```json
{"skills": [{"name": "incident_triage", "version": "1.0.0", "catalog": "axlr"}]}
```

- With `skills`, the execution's catalog skills are exactly those, at those versions; without it, every installed catalog skill at its newest installed version. Built-in and plugin skills are not catalog-versioned and stay as they are.
- `catalog` may be left out when one catalog's install has the name. A pin that is not installed ends the execution before its first request (`--exec` exits 1 with `the execution's skills are unavailable here: …`; `CreateSession` answers `422 skills_unavailable`): an execution never installs.
- The session keeps the pins; successors carry them; a new `--skills` replaces them.
- The receipt records them: `"skills": {"sha256": …, "pins": [{"name", "version", "catalog", "bundle_sha256"}]}`, the `sha256` over their compact JSON; absent without pins. Every `axlr_skill` call is in the receipt's `tool_calls`, and its result names the version read.

**How the model finds and reads one.** The system prompt's skill index, which lists the built-in and plugin skills today, lists the session's catalog skills after them: `- catalog axlr: incident_triage 1.0.0 (Triage a production incident…)`. The model opens one with `axlr_skill` naming `catalog` instead of `plugin`: `{"catalog": "axlr", "skill": "incident_triage", "path": "SKILL.md"}`; the version is the session's pin, or the newest installed, never the model's choice. The index text is a pure function of the installed skills and the pins, so the prompt cache holds across turns.

## Not done here

Running the harness against a model (Phase 4); the model installing a skill (`axlr_install_skill`: the person or the program does it); revocations of installed skills (open mechanism 5 of the registry design, shared with tools); skills that bundle scripts.
