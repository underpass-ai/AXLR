# Releasing AXLR

The repository now contains a tag-gated [release workflow](../.github/workflows/release.yml), a deterministic [archive packager](../scripts/release/package.py), a locked [engine adapter inventory](../distribution/engines.lock.json) and the [Helm chart](../charts/axlr/Chart.yaml). No published release is implied by these source files. The three binaries are `axlr`, `axlr-tui` and `axlr-serve`; each accepts `--version`.

To release, tag a commit of `main` with exactly `vMAJOR.MINOR.PATCH` or a semantic prerelease, and push the tag:

```bash
git tag vX.Y.Z <commit-on-main>
git push origin vX.Y.Z
```

No commit prepares a release: the tag is the version. The binaries, the image and the chart all take it from the tag; `Chart.yaml` keeps the placeholder `0.0.0` and `package_chart.py` sets the chart's `version` and `appVersion` when it packages it. GitHub writes the release notes from the merged pull requests. Until 8 November 2026 each release needed a "Prepare vX.Y.Z" pull request that raised the chart's version and added an entry to [the audit](documentation-audit.md); the audit now records documentation reviews only. The preflight checks the tag, the engine lock, commit ancestry and a clean checkout, then freezes the expected inventory. Running the workflow by hand builds a development version without publication.

Six native runners test and package Linux, macOS and Windows on amd64 and arm64. The Windows ARM64 runner is `windows-11-arm`; `go test -race` is omitted there because that Go target does not support the race detector. Every archive contains three binaries, `VERSION`, `LICENSE`, `NOTICE`, third-party license texts and a short installation README, with a sibling `.sha256`. The license bundle is generated across all six targets and both Go modules with `go-licenses` v2.0.1; CI checks that it matches the dependency graph. The Linux image contains SHA-256-verified `kmp-mcp` and `made-mcp` binaries, the license bundle, and the KMP and MADE license and notice files from the locked release versions. The chart also contains AXLR `LICENSE` and `NOTICE`, checked against the repository copies before packaging. The chart packages to OCI. Only the final tag job has write permissions. It downloads immutable artifacts, compares the exact inventory and hashes, then publishes by version. Repeated publication compares existing bytes and rejects a differing release.

Locally, `python3 scripts/release/preflight.py --ref refs/heads/main --sha "$(git rev-parse HEAD)" --output /tmp/axlr-expected.json` checks development identity. After a clean commit, `bash scripts/release/package.sh 0.1.0 linux amd64 --output /tmp/axlr-dist` builds a native archive. Use `helm lint charts/axlr -f charts/axlr/values.example.yaml` and `helm template test charts/axlr -f charts/axlr/values.example.yaml` to inspect the chart without installing it. The example names and digest are placeholders.

When Go dependencies change, install `go-licenses` v2.0.1 and run `python3 scripts/release/licenses.py --write`. Review changes under `distribution/third_party/` before committing. Keep `distribution/engine_licenses/` in sync with the two locked adapter releases. `package_chart.py` normalizes Helm tar/gzip metadata so rerunning an unchanged release produces the same chart bytes.

The workflow checks image `livez`, client-certificate rejection and failing `readyz` without engines on both native Linux architectures. Before announcing a tagged release, download and run each of the six archives on its native architecture, then verify a real KMP/MADE test deployment. A cross-build alone does not establish runtime support. The [implementation plan](plans/axlr-service-helm-release.md) remains the acceptance checklist.
