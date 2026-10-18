# semver_tools

Works with [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html) versions and npm
version ranges, with the behaviour of npm's `node-semver`.

Operations:

- `parse`: the strict SemVer 2.0.0 grammar (`MAJOR.MINOR.PATCH[-PRERELEASE][+BUILD]`, no leading
  zeros in numeric parts or numeric prerelease identifiers). A leading `v` or `=` is removed
  and the output says so. Returns the parts, the prerelease identifiers (numbers as numbers)
  and the build identifiers.
- `compare` and `sort`: precedence per section 11 of the spec. Major, minor and patch compare
  numerically; a version with a prerelease is lower than the same version without one;
  prerelease identifiers compare left to right, numeric ones by value, alphanumeric ones in
  ASCII order, numeric lower than alphanumeric, and a longer list wins when all earlier
  identifiers are equal. Build metadata is ignored (two versions that differ only in build
  compare equal; `sort` is stable, so they keep their input order).
- `bump`: node-semver's `inc()`. `major`, `minor` and `patch` of a prerelease of that exact
  level drop the prerelease (`1.0.0-5` major is `1.0.0`); `premajor`, `preminor` and
  `prepatch` bump and start a prerelease at `0` (or `<preid>.0`); `prerelease` increments the
  last numeric identifier (appending `.0` when there is none) and bumps the patch first on a
  stable version; `release` drops the prerelease. Build metadata is dropped.
- `match`: desugars the range the way node-semver does and tests each version. `^` allows
  changes that keep the left-most non-zero part, `~` allows patch changes (minor changes when
  only a major is given), `1.x`/`1.2.*`/partial versions are x-ranges, `A - B` is inclusive
  with a partial upper end meaning "anything starting with it", `<1.2` is `<1.2.0-0`, `>1`
  is `>=2.0.0`, and `||` joins comparator sets. Upper bounds are written with `-0` so no
  prerelease of the next version sneaks in. A prerelease version only satisfies a set in
  which some comparator has a prerelease on the same `major.minor.patch`
  (`include_prerelease: true` lifts that rule). The output gives the desugared range and the
  highest and lowest satisfying versions.

The tests check the precedence chain of section 11 of semver.org, its build-metadata rule,
and the examples of the node-semver README: the range `1.2.7 || >=1.2.9 <2.0.0`, the
`>1.2.3-alpha.3` prerelease example, every caret, tilde, x-range and hyphen desugaring listed
there, and its `inc` examples (`1.2.3` prerelease beta is `1.2.4-beta.0`, then `1.2.4-beta.1`,
then `release` is `1.2.4`).

Limits: no "loose" mode beyond a leading `v` or `=` (node-semver's loose parsing accepts more
malformed input); no `coerce`, `intersects` or range simplification; a prerelease `preid`
with `identifierBase` options of node-semver is not offered (always zero-based).
