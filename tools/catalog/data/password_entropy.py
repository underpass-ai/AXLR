#!/usr/bin/env python3
"""Write catalog/tools/security/password_entropy/common_passwords.txt: the most common
passwords, in rank order, that the tool penalises as dictionary words.

Source: SecLists by Daniel Miessler et al., MIT License,
  Passwords/Common-Credentials/10k-most-common.txt
  https://raw.githubusercontent.com/danielmiessler/SecLists/master/Passwords/Common-Credentials/10k-most-common.txt
fetched once by this generator (it uses the network; the tool never does). The first 2000
entries are kept, lower-cased, without duplicates and without entries shorter than 4
characters; the rank is the line number in the original list. Fetched 2026-10-18.

Usage: python3 tools/catalog/data/password_entropy.py [--from FILE]
  --from FILE  read a local copy of the list instead of fetching it
"""
import datetime as dt
import pathlib
import sys
import urllib.request

URL = "https://raw.githubusercontent.com/danielmiessler/SecLists/master/Passwords/Common-Credentials/10k-most-common.txt"
OUT = pathlib.Path(__file__).resolve().parents[3] / "catalog/tools/security/password_entropy/common_passwords.txt"
KEEP = 2000


def main():
    if len(sys.argv) == 3 and sys.argv[1] == "--from":
        text = pathlib.Path(sys.argv[2]).read_text(encoding="utf-8", errors="replace")
    else:
        request = urllib.request.Request(URL, headers={"User-Agent": "AXLR-catalog data generator"})
        with urllib.request.urlopen(request, timeout=60) as response:
            text = response.read().decode("utf-8", errors="replace")
    seen, rows = set(), []
    for rank, line in enumerate(text.splitlines(), 1):
        word = line.strip().lower()
        if len(word) < 4 or word in seen or len(rows) >= KEEP or word.startswith("#") or len(word) > 64:
            continue
        seen.add(word)
        rows.append(f"{rank}\t{word}")
    header = [
        "# Most common passwords (rank<TAB>password, lower-cased), first 2000 of length >= 4.",
        "# Source: SecLists Passwords/Common-Credentials/10k-most-common.txt, MIT License,",
        f"# {URL}",
        f"# Fetched {dt.date.today().isoformat()} by tools/catalog/data/password_entropy.py",
    ]
    OUT.write_text("\n".join(header + rows) + "\n", encoding="utf-8")
    print(f"wrote {OUT} ({len(rows)} entries)")


if __name__ == "__main__":
    main()
