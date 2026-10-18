#!/usr/bin/env python3
"""Write catalog/tools/programming/gitignore_gen/templates.json: the .gitignore templates the
tool combines, fetched once from github/gitignore at a pinned commit (this generator uses the
network; the tool never does).

Source: https://github.com/github/gitignore, licence CC0-1.0 (the repository's LICENSE),
commit 0e5d690153ca3da8a4a1aef2d053406f408f531c (committed 2026-10-02), raw files from
  https://raw.githubusercontent.com/github/gitignore/<commit>/<path>
Fetched on 2026-10-18. Templates live at the top level (languages, frameworks), in Global/
(editors, operating systems, tools) and in community/; STACKS maps each public name to its
exact path, and a missing path stops the generator.

Usage: python3 tools/catalog/data/gitignore_gen.py [--cache DIR]
"""
import json
import pathlib
import sys
import urllib.request

COMMIT = "0e5d690153ca3da8a4a1aef2d053406f408f531c"
COMMIT_DATE = "2026-10-02"
FETCHED = "2026-10-18"
RAW = f"https://raw.githubusercontent.com/github/gitignore/{COMMIT}/"
OUT = pathlib.Path(__file__).resolve().parents[3] / "catalog/tools/programming/gitignore_gen/templates.json"

# public name: (path in github/gitignore, kind, aliases)
STACKS = {
    "Python": ("Python.gitignore", "language", ["py", "python3", "django", "flask"]),
    "Node": ("Node.gitignore", "language", ["nodejs", "javascript", "js", "typescript", "ts", "npm", "yarn"]),
    "Deno": ("Deno.gitignore", "language", []),
    "Go": ("Go.gitignore", "language", ["golang"]),
    "Rust": ("Rust.gitignore", "language", ["cargo"]),
    "Java": ("Java.gitignore", "language", []),
    "Kotlin": ("Kotlin.gitignore", "language", []),
    "Scala": ("Scala.gitignore", "language", []),
    "Maven": ("Maven.gitignore", "build", ["mvn"]),
    "Gradle": ("Gradle.gitignore", "build", []),
    "C": ("C.gitignore", "language", []),
    "C++": ("C++.gitignore", "language", ["cpp", "cplusplus", "cxx"]),
    "CMake": ("CMake.gitignore", "build", []),
    "Dotnet": ("Dotnet.gitignore", "language", [".net", "dotnet-core", "csharp", "c#", "fsharp"]),
    "VisualStudio": ("VisualStudio.gitignore", "editor", ["vs", "visual-studio"]),
    "Swift": ("Swift.gitignore", "language", []),
    "Objective-C": ("Objective-C.gitignore", "language", ["objc"]),
    "Ruby": ("Ruby.gitignore", "language", ["gem", "bundler"]),
    "Rails": ("Rails.gitignore", "framework", ["rubyonrails", "ruby-on-rails"]),
    "Elixir": ("Elixir.gitignore", "language", []),
    "Haskell": ("Haskell.gitignore", "language", []),
    "R": ("R.gitignore", "language", ["rstats"]),
    "Julia": ("Julia.gitignore", "language", []),
    "Dart": ("Dart.gitignore", "language", []),
    "Flutter": ("Flutter.gitignore", "framework", []),
    "Zig": ("Zig.gitignore", "language", []),
    "Android": ("Android.gitignore", "platform", []),
    "Unity": ("Unity.gitignore", "framework", ["unity3d"]),
    "UnrealEngine": ("UnrealEngine.gitignore", "framework", ["unreal", "ue4", "ue5"]),
    "Terraform": ("Terraform.gitignore", "tool", ["tf"]),
    "Laravel": ("Laravel.gitignore", "framework", []),
    "Nextjs": ("Nextjs.gitignore", "framework", ["next", "next.js"]),
    "TeX": ("TeX.gitignore", "language", ["latex"]),
    "JupyterNotebooks": ("community/Python/JupyterNotebooks.gitignore", "tool", ["jupyter", "ipynb"]),
    "macOS": ("Global/macOS.gitignore", "os", ["mac", "osx", "darwin"]),
    "Windows": ("Global/Windows.gitignore", "os", ["win"]),
    "Linux": ("Global/Linux.gitignore", "os", []),
    "JetBrains": ("Global/JetBrains.gitignore", "editor", ["intellij", "idea", "pycharm", "webstorm", "goland", "clion", "rider"]),
    "VisualStudioCode": ("Global/VisualStudioCode.gitignore", "editor", ["vscode", "code"]),
    "Vim": ("Global/Vim.gitignore", "editor", ["neovim", "nvim"]),
    "Emacs": ("Global/Emacs.gitignore", "editor", []),
    "Eclipse": ("Global/Eclipse.gitignore", "editor", []),
    "Xcode": ("Global/Xcode.gitignore", "editor", []),
    "VirtualEnv": ("Global/VirtualEnv.gitignore", "tool", ["venv", "virtualenv"]),
}


def fetch(path, cache):
    key = path.replace("/", "__")
    if cache and (cache / key).exists():
        return (cache / key).read_text(encoding="utf-8")
    request = urllib.request.Request(RAW + path, headers={"User-Agent": "AXLR-catalog data generator"})
    with urllib.request.urlopen(request, timeout=60) as response:
        text = response.read().decode("utf-8")
    if cache:
        (cache / key).write_text(text, encoding="utf-8")
    return text


def main():
    cache = None
    if len(sys.argv) == 3 and sys.argv[1] == "--cache":
        cache = pathlib.Path(sys.argv[2])
        cache.mkdir(parents=True, exist_ok=True)
    templates = {}
    for name, (path, kind, aliases) in STACKS.items():
        text = fetch(path, cache)  # raises on a missing path: the mapping must be fixed
        lines = [line.rstrip("\r") for line in text.split("\n")]
        while lines and not lines[-1].strip():
            lines.pop()
        templates[name] = {"path": path, "kind": kind, "aliases": aliases, "lines": lines}
    doc = {
        "source": "github/gitignore",
        "repository": "https://github.com/github/gitignore",
        "commit": COMMIT,
        "commit_date": COMMIT_DATE,
        "license": "CC0-1.0",
        "fetched": FETCHED,
        "raw_url": RAW + "<path>",
        "templates": templates,
    }
    text = json.dumps(doc, indent=1, ensure_ascii=False) + "\n"
    OUT.write_text(text, encoding="utf-8")
    print(f"wrote {OUT} ({len(text.encode())} bytes, {len(templates)} templates)")


if __name__ == "__main__":
    main()
