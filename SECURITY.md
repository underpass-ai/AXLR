# Security policy

## Report a vulnerability

Write to **security@underpassai.com**. Please do not open a public issue for a vulnerability.

Include what you found, the AXLR version (`axlr-tui --version`), how to reproduce it and what an attacker could do with it. Encrypted mail is not required.

| Step | When |
|:--|:--|
| We confirm we received it | within 3 working days |
| We say whether we reproduce it and what we plan | within 10 working days |
| A fix is released | as soon as it is ready; we agree a disclosure date with you |
| A public advisory is published | with the fix, as a [GitHub security advisory](https://github.com/underpass-ai/AXLR/security/advisories), crediting you unless you ask us not to |

We do not run a paid bug bounty.

## Supported versions

AXLR is pre-1.0. Security fixes go into the latest release only; upgrade to receive them. The [changelog](CHANGELOG.md) lists every release.

| Version | Supported |
|:--|:--|
| latest 0.x release | yes |
| earlier releases | no |

## Scope

In scope: the `axlr`, `axlr-tui` and `axlr-serve` binaries, the container image, the Helm chart, the release archives and their checksums, and the signed tool catalog index at `registry.underpassai.com`.

Out of scope: behaviour that the [security and data](docs/security.md) page documents as a limit, such as a command the person approved, a program run with the exec sandbox off, or an MCP server's own side effects. Third-party model providers, MCP servers and plugins have their own policies.

## How AXLR protects you

[Security and data](docs/security.md) describes what leaves the machine, approvals and sandboxing, the prompt-injection posture and the audit records.
