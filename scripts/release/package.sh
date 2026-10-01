#!/usr/bin/env bash
set -euo pipefail
if [[ $# -lt 3 ]]; then
  echo "usage: package.sh VERSION GOOS GOARCH [--output DIR]" >&2
  exit 2
fi
exec python3 "$(dirname "$0")/package.py" "$@"
