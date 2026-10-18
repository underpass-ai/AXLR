# secrets_scan

Looks for credentials that should not be in a text: configuration files, source code, logs,
a diff. Every finding is **redacted**: the output shows the first 4 characters of the match,
asterisks and its length, never the rest.

## Rules

Adapted from gitleaks' default configuration (`config/gitleaks.toml`, MIT): each rule is a
regular expression, the group that holds the secret and, for most, a minimum Shannon entropy
`H = −Σ p·log2 p` over the characters of the match, which drops repetitive look-alikes.

| Rule | Matches |
|:--|:--|
| `aws-access-token` | `AKIA`, `ASIA`, `ABIA`, `ACCA` or `A3T?` + 16 of A–Z2–7 |
| `aws-secret-access-key` | 40 base64 characters after an `aws…secret/key` name |
| `github-pat`, `github-oauth`, `github-app-token`, `github-refresh-token` | `ghp_`, `gho_`, `ghu_`/`ghs_`, `ghr_` + 36 |
| `github-fine-grained-pat` | `github_pat_` + 82 |
| `gitlab-pat` | `glpat-` + 20 |
| `slack-bot-token`, `slack-user-token`, `slack-app-token`, `slack-legacy-token`, `slack-webhook-url` | `xoxb-…`, `xoxp-`/`xoxe-…`, `xapp-…`, `xoxa/o/r/s-…`, `hooks.slack.com/…` |
| `stripe-access-token` | `sk_`/`rk_` + `test`/`live`/`prod` |
| `gcp-api-key` | `AIza` + 35 |
| `npm-access-token` | `npm_` + 36 |
| `private-key` | `-----BEGIN … PRIVATE KEY-----` headers |
| `jwt` | `ey….ey….…` |
| `generic-api-key` | a value of 10–150 characters assigned (`=`, `:`, `:=`, `=>`…) to a name containing access, auth, api, credential, creds, key, password, secret or token, with entropy ≥ `entropy_threshold` (3.5 by default, as gitleaks) and at least one digit or capital |

A match already reported by a specific rule is not reported again by the generic one.
`looks_like_placeholder` marks values containing EXAMPLE, xxxx, fake, dummy, changeme,
`<…>` or template markers; `include_examples: false` drops them. Each finding has a
gitleaks-style `fingerprint` (`source:rule:line`) for allowlists.

## Input

`text` inline, `files` as `{name: content}`, and `paths` relative to the working directory
(absolute paths and `..` are refused). Up to 5 MB in total.

## Tests

The AWS documentation example credentials (`AKIAIOSFODNN7EXAMPLE` and
`wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY`, from the AWS CLI configuration guide), with
their line, column and redaction; the Shannon entropy of the key id worked by hand (3.684 bits
per character); the RFC 7519 example JWT; visibly fake GitHub, Slack and generic tokens; a
PEM header line without key material; the entropy threshold both ways; and the path and
usage errors. No real-looking secret is in the tests.

## Limits

Regular expressions over text, not a parser: secrets split across lines, encoded (base64,
URL-encoded) or in binary files are missed, and some random identifiers will be reported. No
git history scanning (rewrite history and rotate any secret found).
