#!/usr/bin/env bash
# Guards the nightly's symbol-level jq filter against silently matching nothing
# (positive control) or matching module-level findings (negative control).
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
out="$(jq -r -f "$here/govuln-symbol-filter.jq" "$here/testdata/govuln-symbol-hit.json" | sort -u)"
if [ "$out" != "GO-2099-0001" ]; then
  echo "::error::govuln symbol filter expected exactly GO-2099-0001, got: '${out}'"
  exit 1
fi
echo "govuln symbol filter: positive and module-level controls OK"
