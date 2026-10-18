# gitignore_gen

Writes a `.gitignore` for a project from named stacks, using the templates of
[github/gitignore](https://github.com/github/gitignore) (CC0-1.0).

Stacks (44; `list_stacks: true` lists them with their paths and aliases): Python, Node, Deno,
Go, Rust, Java, Kotlin, Scala, Maven, Gradle, C, C++, CMake, Dotnet, VisualStudio, Swift,
Objective-C, Ruby, Rails, Elixir, Haskell, R, Julia, Dart, Flutter, Zig, Android, Unity,
UnrealEngine, Terraform, Laravel, Nextjs, TeX, JupyterNotebooks (from `community/Python/`),
and from `Global/`: macOS, Windows, Linux, JetBrains, VisualStudioCode, Vim, Emacs, Eclipse,
Xcode, VirtualEnv. Names are case-insensitive and have aliases (`node`/`javascript`/
`typescript`, `golang`, `cpp`, `csharp`/`.net`, `vscode`, `intellij`/`pycharm`, `mac`/`osx`,
`venv`, `jupyter`, `latex`…).

The file starts with a line naming the source commit, then one section per stack, in the
order given, headed `### Name ### (github/gitignore <path>)`, then a `### Custom ###` section
with the `extra` patterns, if any. `comments: false` drops the templates' own comment lines.

Deduplication (`dedupe`, on by default) removes a pattern that an earlier section already
emitted, but only when no negation (`!pattern`) was emitted since its first occurrence:
in a `.gitignore` the last matching pattern wins, so a pattern repeated after a negation can
re-ignore what the negation re-included, and it is kept. Negations themselves are never
removed. Each section reports how many patterns it kept and dropped.

`templates.json` is written by `tools/catalog/data/gitignore_gen.py`, which fetched the
templates on 18 October 2026 from github/gitignore at commit
`0e5d690153ca3da8a4a1aef2d053406f408f531c` (2 October 2026), mapping each stack to its exact
path and stopping on a missing one. The commit, date and licence are in every answer.
Rerunning it on a newer commit is a new version of the tool.

The tests check lines present in the upstream templates at that commit (`__pycache__/`,
`*.py[codz]` and `*$py.class` at the top of `Python.gitignore`; `.DS_Store` in
`Global/macOS.gitignore`; `node_modules/` in `Node.gitignore`), the aliases, the
deduplication of C and C++ (C++ repeats 19 of C's patterns) and the negation rule.

Limits: only the bundled templates; no merging of semantically equal patterns written
differently (`build/` and `/build/`); no detection of the stack from a project's files.
