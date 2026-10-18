# env_example

Makes a `.env.example` from a `.env` given inline, so the file of settings can be committed
without its values.

Each line is read the way python-dotenv and docker compose read it:

- blank lines and `#` comments are copied as they are;
- `KEY=value`, `KEY = value` and `export KEY=value` keep their key, spacing and `export`;
- a value in single, double or back quotes keeps its quotes (`KEY="<url>"`); a double-quoted
  value may span several lines (backslash escapes are honoured) and becomes one line;
- an inline comment after an unquoted value (` # …`) or after the closing quote is kept;
- a commented-out assignment (`# OLD_TOKEN=…`) is kept as a comment with its value replaced too;
- any other line is replaced by `# (line N removed: …)` because it might hold a value.

With `hints: true` (the default) the value becomes a type hint: `<url>`, `<number>`,
`<boolean>` (true/false/yes/no/on/off), `<email>`, `<host>` (an IP, `host:port` or
`localhost`), `<path>`, `<interpolated>` (it refers to `${OTHER}` or `$OTHER`), `<secret>` or
`<string>`. `<secret>` is used when the key looks like a secret (`PASSWORD`, `SECRET`,
`TOKEN`, `API_KEY`, `ACCESS_KEY`, `PRIVATE`, `AUTH`, `SALT`, a name ending in `_KEY`…), when
the value has the shape of a known credential (AWS access key id `AKIA…`/`ASIA…`, GitHub
`ghp_…`/`github_pat_…`, Slack `xox…-`, `sk-…` API keys, Stripe `sk_live_…`, Google `AIza…`, a
JWT, a PEM private key header, a URL with `user:password@`), or when it is a long
random-looking string (24 characters or more, Shannon entropy at least 4 bits per character).
An empty value stays empty. With `hints: false` every value is left empty.

The output gives the `example` text, each variable with its line, hint, quoting and flags,
the `duplicates` (keys defined more than once; most loaders keep the last), and the
`suspicious` entries. No field of the output contains a value from the input: hints, flags
and line numbers only.

The tests are hand-checked: the first one compares the whole generated file with the
expected text, which is how the suite shows that no value (a password in a URL, an API key,
AWS's documented example key id `AKIAIOSFODNN7EXAMPLE`, a commented-out token) survives,
since the runner can only check what is present, not what is absent.

Limits: the secret detection is heuristic, in both directions (a secret with an innocent
name and an ordinary-looking value gets `<string>`); a single-quoted value spanning lines is
read as multi-line like a double-quoted one; no variable expansion is performed.
