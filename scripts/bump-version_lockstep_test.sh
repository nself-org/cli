#!/usr/bin/env bash
# Exercise the real bumper against a private sibling checkout layout.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BASE=$(mktemp -d)
trap 'rm -rf "$BASE"' EXIT
mkdir -p "$BASE/cli/scripts" "$BASE/admin/src/lib"
cp "$ROOT/scripts/bump-version.sh" "$BASE/cli/scripts/"
if [ -d "$ROOT/scripts/bump-version" ]; then cp -R "$ROOT/scripts/bump-version" "$BASE/cli/scripts/"; fi
for f in .github/VERSION internal/version/version.go sdk/go/doc.go sdk/ts/package.json sdk/ts-sdk/package.json sdk/ts-cli-api/package.json sdk/py/pyproject.toml; do
  mkdir -p "$BASE/cli/$(dirname "$f")"
  cp "$ROOT/$f" "$BASE/cli/$f"
done
printf '{"version":"1.0.0"}\n' > "$BASE/admin/package.json"
printf "export const CLI_VERSION = '1.0.0'\n" > "$BASE/admin/src/lib/cli-version.ts"
printf 'ARG NSELF_VERSION=1.0.0\nENV ADMIN_VERSION=1.0.0\nLABEL org.opencontainers.image.version="1.0.0"\n' > "$BASE/admin/Dockerfile"
for mode in --dry-run --no-goldens --no-goldens; do
  bash "$BASE/cli/scripts/bump-version.sh" 9.8.7 "$mode" > "$BASE/output"
  for pkg in ts ts-sdk ts-cli-api; do
    actual=$(jq -r .version "$BASE/cli/sdk/$pkg/package.json")
    if [ "$mode" = --dry-run ]; then
      test "$actual" = "$(jq -r .version "$ROOT/sdk/$pkg/package.json")"
    else
      test "$actual" = 9.8.7 || { echo "FAIL $pkg: $actual"; exit 1; }
    fi
  done
done
echo 'PASS real bumper: SDK packages, dry run and idempotence'
