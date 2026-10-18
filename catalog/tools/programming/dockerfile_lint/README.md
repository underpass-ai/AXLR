# dockerfile_lint

Lints a Dockerfile given inline. The rules are modelled on
[hadolint](https://github.com/hadolint/hadolint) and use its ids, severities and messages
where a rule corresponds; three rules of our own have `AXL` ids.

| Rule | Severity | Fires on |
|:--|:--|:--|
| DL3000 | error | `WORKDIR` with a relative path |
| DL3002 | warning | the last `USER` of the final stage is `root` or `0` |
| DL3004 | error | `sudo` in a `RUN` |
| DL3006 | warning | `FROM` an image without a tag (not `scratch`, a stage alias, a `$VARIABLE` or a `@sha256:` digest) |
| DL3007 | warning | `FROM` an image tagged `latest` |
| DL3008 | warning | `apt-get install` of a package without `=version` |
| DL3009 | info | `apt-get install` in a `RUN` that does not remove the apt lists directory (or use a cache mount on it), in the final stage or a stage used as a base |
| DL3014 | warning | `apt-get install` without `-y`, `--yes`, `--assume-yes` or `-qq` |
| DL3015 | info | `apt-get install` without `--no-install-recommends` |
| DL3020 | error | `ADD` of local files that are not archives (`COPY` does that); URLs and archives are fine |
| DL3025 | warning | `CMD` or `ENTRYPOINT` in shell form instead of a JSON array |
| DL3027 | warning | `apt install`/`update`/… instead of `apt-get` |
| DL4000 | error | `MAINTAINER` (deprecated; use `LABEL`) |
| DL4003 | warning | a second `CMD` in a stage |
| DL4004 | error | a second `ENTRYPOINT` in a stage |
| AXL001 | warning | the final stage has no `USER` at all, so the container runs as root (hadolint only checks an explicit root `USER`) |
| AXL002 | error | an `ENV` or `ARG` whose name looks like a secret (`PASSWORD`, `SECRET`, `TOKEN`, `API_KEY`, `PRIVATE_KEY`, `ACCESS_KEY`, `CREDENTIAL`…): its value ends up in the image or its build history (Docker's build check `SecretsUsedInArgOrEnv` makes the same point); the value is never printed |
| AXL003 | warning | `curl` or `wget` piped into `sh`/`bash`/`zsh`/`dash`/`ksh` |

The parser follows the Dockerfile reference: instructions are case-insensitive, lines ending
with the escape character (`\`, or `` ` `` after a `# escape=` directive) continue, comment and
blank lines inside a continuation are skipped, heredocs (`RUN <<EOF … EOF`) are read as part
of the instruction, and `FROM … AS name` stages are tracked so `FROM name` is not an untagged
image. Shell commands are split on `&&`, `||`, `;`, `|` and newlines and tokenised with
`shlex`. `# hadolint ignore=DL3008,DL3009` on the line before an instruction silences those
rules for it, and `ignore` in the input silences rules everywhere.

The output lists each finding with its line, rule, severity, message and detail, counts by
severity, the stages, and `passed` (no error or warning).

The tests take the "problematic code" and "correct code" examples of hadolint's wiki pages
for DL3000, DL3002, DL3004, DL3006, DL3007, DL3008, DL3009, DL3014, DL3015, DL3020, DL3027,
DL4000 and DL4003, and check that the rule fires on the first and not on the second.

Limits: this is not hadolint. There is no ShellCheck pass over `RUN` (the SC rules), no
rules for pip, npm, apk, gem or yum pinning, no registry allow-list (DL3026), no label
schema; `ONBUILD` contents are not inspected, and the shell splitting does not understand
subshells or quoting across `&&`.
