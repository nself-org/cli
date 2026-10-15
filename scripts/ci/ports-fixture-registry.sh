#!/usr/bin/env bash
# Purpose: build the merged command-plugin ports (ci, functions, infra, plugin-dev) into a
#          signed fixture registry the CLI installs from, so TestBreakoutAutoAddFixturePorts
#          (P7-CANON-13, readiness R1) proves the v1.5 auto-add against the real ports, not stubs.
# Usage:   ports-fixture-registry.sh <plugins-checkout> <outdir>
# Inputs:  a checkout of the plugins repo (its origin/main is built in a detached worktree,
#          never the checkout itself); an output directory.
# Outputs: <outdir>/registry.json    the catalogue the installer reads: one free cli entry per
#                                    port with its sha256, a release_signature and repository
#                                    "@BASE@" (the test replaces it with its server URL);
#          <outdir>/tarballs/<slug>.tar.gz   plugin.json plus the host-platform binary, served at
#                                    <registry>/plugins/<slug>/tarball;
#          <outdir>/key.pub          standard base64 of the 32-byte ed25519 public key that
#                                    verifies every entry. The private key lives only inside
#                                    the signer process and is never written.
#          Exit 0 on success; 1 when a build or step fails; 2 on a usage error or missing tool.
# Constraints: bash 3.2 compatible; needs git, go, jq, tar and a sha256 tool; builds with
#          CGO_ENABLED=0 for the host platform (-mod=vendor when the port vendors); scratch
#          goes to $TMPDIR and the worktree is removed on exit; touches no production key.
set -euo pipefail

die() { echo "ports-fixture-registry: $*" >&2; exit "${2:-2}"; }
[ "$#" -eq 2 ] || die "usage: ports-fixture-registry.sh <plugins-checkout> <outdir>"
PLUGINS="$1"; OUT="$2"
[ -d "$PLUGINS/.git" ] || [ -f "$PLUGINS/.git" ] || die "$PLUGINS is not a git checkout"
for t in git go jq tar; do command -v "$t" >/dev/null 2>&1 || die "needs $t"; done
sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi
}

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/ports-fixture.XXXXXX")"
WT="$WORK/plugins"
cleanup() {
  git -C "$PLUGINS" worktree remove --force "$WT" >/dev/null 2>&1 || true
  git -C "$PLUGINS" worktree prune >/dev/null 2>&1 || true
  rm -rf "$WORK"
}
trap cleanup EXIT

git -C "$PLUGINS" fetch -q origin main 2>/dev/null || echo "ports-fixture-registry: fetch failed, using the local origin/main" >&2
git -C "$PLUGINS" worktree add -q --detach "$WT" origin/main || die "cannot make a worktree of $PLUGINS origin/main" 1

# $OUT is wiped, so refuse anything that could be a real directory: it must be
# new, empty, or a directory this script made before (marker file).
case "$OUT" in ""|/|.|..|"$HOME"|"$HOME"/) die "refusing to use '$OUT' as the output directory" ;; esac
if [ -e "$OUT" ] && [ ! -f "$OUT/.ports-fixture" ] && [ -n "$(ls -A "$OUT" 2>/dev/null)" ]; then
  die "$OUT exists and is not a previous ports-fixture output; choose a new directory"
fi
rm -rf "$OUT"
mkdir -p "$OUT/tarballs"
: > "$OUT/.ports-fixture"
OUT="$(cd "$OUT" && pwd)"
ENTRIES="$WORK/entries"
: > "$ENTRIES"

for slug in ci functions infra plugin-dev; do
  dir="$WT/free/$slug"
  [ -f "$dir/plugin.json" ] || die "free/$slug/plugin.json is missing at plugins origin/main" 1
  bin="$(jq -r '.binaryName // empty' "$dir/plugin.json")"
  [ "$bin" = "nself-$slug" ] || die "free/$slug: binaryName '$bin' is not nself-$slug" 1
  stage="$WORK/stage/$slug"
  mkdir -p "$stage"
  mod=""
  if [ -d "$dir/vendor" ]; then mod="-mod=vendor"; fi
  # shellcheck disable=SC2086 # $mod is one option or empty
  (cd "$dir" && CGO_ENABLED=0 go build $mod -o "$stage/$bin" ./cmd) || die "free/$slug does not build" 1
  cp "$dir/plugin.json" "$stage/plugin.json"
  tar -czf "$OUT/tarballs/$slug.tar.gz" -C "$stage" plugin.json "$bin"
  jq -n --arg name "$slug" --arg bin "$bin" --arg sum "$(sha256_of "$OUT/tarballs/$slug.tar.gz")" \
    --slurpfile m "$dir/plugin.json" '{
      name: $name, version: $m[0].version, description: ($m[0].description // $name),
      category: ($m[0].category // "development"), tier: "free", license: "MIT", status: "stable",
      pluginType: "cli", binaryName: $bin, checksum: $sum, repository: "@BASE@"
    }' >> "$ENTRIES"
  echo "built $slug $(jq -r .version "$dir/plugin.json")"
done

jq -s '{plugins: .}' "$ENTRIES" > "$OUT/registry.json"
cd "$ROOT"
CGO_ENABLED=0 go run -mod=vendor ./scripts/ci/testdata/breakout/signer "$OUT/registry.json" "$OUT/key.pub" || die "signing failed" 1
jq -e '[.plugins[] | select(.release_signature.sig != null)] | length == 4' "$OUT/registry.json" >/dev/null || die "not every entry was signed" 1
echo "ports-fixture-registry: wrote $OUT (4 signed entries)"
