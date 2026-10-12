#!/usr/bin/env bash
# FIXTURE: verbatim copy of nself-org/homebrew-nself .github/scripts/update-formula.sh at origin/main 6cb01eac50ce37bb3cdf9383c4d8e53522390f06.
# Used by scripts/release/test.sh only. Refresh it by re-copying; do not edit.
#
# update-formula.sh — write a CLI release's per-architecture sha256 values into
# the dual-arch Homebrew formula.
#
# Purpose:   Resolve the darwin-arm64 and darwin-amd64 sha256 values for a given
#            CLI release from that release's checksums.txt, then write each one
#            into its own on_arm / on_intel block in Formula/nself.rb.
# Inputs:    --version <vX.Y.Z|X.Y.Z>   release tag to update to (required)
#            --formula <path>           formula file (default: Formula/nself.rb)
#            --repo    <owner/name>     release source (default: nself-org/cli)
#            --checksums-file <path>    optional; read checksums.txt from this
#                                       local file instead of fetching it from
#                                       the release. Lets the formula be written
#                                       from the build run's checksums BEFORE
#                                       the release is published (cli release.yml
#                                       needs the formula first). The source
#                                       archive guard (step 3) still runs.
#            --github-output <path>     optional; appends version/sha outputs
# Outputs:   Formula rewritten in place; summary on stdout; non-zero exit on any
#            validation failure. Idempotent — a formula already carrying the
#            correct version and hashes is left untouched.
# Constraints:
#   - The formula MUST be dual-arch shaped: a `version "X.Y.Z"` line plus a
#     sha256 line inside each of an on_arm and an on_intel block.
#   - Requires curl and awk. No Homebrew, no Ruby, no network writes.
#
# WHY THIS EXISTS (do not collapse back into a sed one-liner):
#   The previous implementation computed ONE sha256 — of the GitHub *source*
#   archive (archive/refs/tags/vX.tar.gz) — and sed'd it into the formula with
#   `s|sha256 "[a-f0-9]*"|sha256 "$SHA"|`. sed without /g still substitutes once
#   per LINE, and a dual-arch formula has two sha256 lines, so BOTH blocks were
#   overwritten with the same source-archive hash. Neither arch's hash was
#   correct, and `brew install nself` would fail the checksum for every user on
#   every platform. The same flaw applied to the url line.
#
#   This script is the workflow-side twin of update_homebrew_formula() in
#   nself-org/cli scripts/bump-version.sh (--homebrew mode). Keep them in sync.
#
# SPORT: homebrew-nself / release automation / formula writer

set -euo pipefail

VERSION=""
FORMULA="Formula/nself.rb"
SOURCE_REPO="nself-org/cli"
GH_OUTPUT="${GITHUB_OUTPUT:-}"
CHECKSUMS_FILE=""

die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }
note() { printf '  %s\n' "$*"; }

while [ $# -gt 0 ]; do
  case "$1" in
    --version)       VERSION="${2:-}"; shift 2 ;;
    --formula)       FORMULA="${2:-}"; shift 2 ;;
    --repo)          SOURCE_REPO="${2:-}"; shift 2 ;;
    --checksums-file) CHECKSUMS_FILE="${2:-}"; shift 2 ;;
    --github-output) GH_OUTPUT="${2:-}"; shift 2 ;;
    *) die "unknown argument: $1" ;;
  esac
done

[ -n "$VERSION" ] || die "--version is required (dispatch payload or workflow input was empty)"
[ -f "$FORMULA" ] || die "formula not found: $FORMULA"

# Normalise to both forms: TAG carries the leading v, PLAIN does not.
case "$VERSION" in v*) TAG="$VERSION" ;; *) TAG="v${VERSION}" ;; esac
PLAIN="${TAG#v}"

printf '%s' "$PLAIN" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$' \
  || die "version '$VERSION' is not a plain X.Y.Z semver tag"

REL_BASE="https://github.com/${SOURCE_REPO}/releases/download/${TAG}"
ASSET_ARM="nself-${PLAIN}-darwin-arm64.tar.gz"
ASSET_AMD="nself-${PLAIN}-darwin-amd64.tar.gz"

# ---------------------------------------------------------------------------
# 1. Get checksums.txt for this release: from --checksums-file when given
#    (build-run output, release not yet published), otherwise fetched.
#    The fetching dispatch can fire seconds after the upload, so retry rather
#    than fail a release on an upload race.
# ---------------------------------------------------------------------------
CHECKSUMS=""
if [ -n "$CHECKSUMS_FILE" ]; then
  [ -f "$CHECKSUMS_FILE" ] || die "--checksums-file not found: $CHECKSUMS_FILE"
  [ -s "$CHECKSUMS_FILE" ] || die "--checksums-file is empty: $CHECKSUMS_FILE"
  CHECKSUMS="$(cat "$CHECKSUMS_FILE")"
  note "checksums read from ${CHECKSUMS_FILE}"
else
  for attempt in 1 2 3 4 5; do
    if CHECKSUMS="$(curl -fsSL --max-time 60 "${REL_BASE}/checksums.txt")"; then
      break
    fi
    CHECKSUMS=""
    note "checksums.txt not available yet (attempt ${attempt}/5) — retrying in $((attempt * 10))s"
    sleep $((attempt * 10))
  done
fi

if [ -z "$CHECKSUMS" ]; then
  die "checksums.txt not found for ${TAG} at ${REL_BASE}/checksums.txt
  The release must be published with its darwin assets before the formula can
  be given real hashes. Nothing was written to ${FORMULA}."
fi

# ---------------------------------------------------------------------------
# 2. Extract each arch's hash BY FILENAME.
#    checksums.txt is `<64-hex>  <asset-filename>` per line. Matching on the
#    filename (not on line order, and not on a bare hex grep) is what keeps the
#    arm value out of the intel block.
# ---------------------------------------------------------------------------
SHA_ARM="$(printf '%s\n' "$CHECKSUMS" | awk -v a="$ASSET_ARM" '$2 == a { print $1; exit }')"
SHA_AMD="$(printf '%s\n' "$CHECKSUMS" | awk -v a="$ASSET_AMD" '$2 == a { print $1; exit }')"

printf '%s' "$SHA_ARM" | grep -qE '^[0-9a-f]{64}$' \
  || die "no valid sha256 for ${ASSET_ARM} in ${TAG} checksums.txt"
printf '%s' "$SHA_AMD" | grep -qE '^[0-9a-f]{64}$' \
  || die "no valid sha256 for ${ASSET_AMD} in ${TAG} checksums.txt"

# Two architectures cannot legitimately share a hash. If they do, something
# upstream collapsed them — refuse rather than publish a formula that installs
# the wrong binary (or fails checksum) on one of the two platforms.
[ "$SHA_ARM" != "$SHA_AMD" ] \
  || die "darwin-arm64 and darwin-amd64 report an identical sha256 (${SHA_ARM}).
  That cannot be correct for two different binaries — check the cli release build."

# ---------------------------------------------------------------------------
# 3. Guard against the exact regression this script replaces: the source
#    archive's hash must never end up in an arch block.
#    Best-effort — a network failure here warns, a MATCH always fails.
# ---------------------------------------------------------------------------
SRC_URL="https://github.com/${SOURCE_REPO}/archive/refs/tags/${TAG}.tar.gz"
# sha256sum on Linux runners, shasum -a 256 on macOS — keep the script runnable
# by a maintainer locally as well as by the workflow.
if command -v sha256sum >/dev/null 2>&1; then
  sha256_of() { sha256sum | awk '{print $1}'; }
else
  sha256_of() { shasum -a 256 | awk '{print $1}'; }
fi
if SRC_SHA="$(curl -fsSL --max-time 60 "$SRC_URL" | sha256_of)" \
   && printf '%s' "$SRC_SHA" | grep -qE '^[0-9a-f]{64}$'; then
  if [ "$SHA_ARM" = "$SRC_SHA" ] || [ "$SHA_AMD" = "$SRC_SHA" ]; then
    die "a resolved arch hash equals the SOURCE archive hash (${SRC_SHA}).
  The formula points at pre-built darwin binaries, not the source tarball.
  This is the failure mode the sed-based updater had — refusing to write it."
  fi
  note "source-archive hash differs from both arch hashes (guard passed)"
else
  printf '::warning::could not fetch %s to cross-check the source hash; continuing\n' "$SRC_URL"
fi

# ---------------------------------------------------------------------------
# 4. Read current state; skip if already correct.
# ---------------------------------------------------------------------------
formula_sha() {
  # $1 = arm|intel — read the sha256 from inside that block only.
  awk -v want="$1" '
    /^[[:space:]]*on_arm do/        { block = "arm" }
    /^[[:space:]]*on_intel do/      { block = "intel" }
    /^[[:space:]]*end[[:space:]]*$/ { block = "" }
    block == want && /^[[:space:]]*sha256[[:space:]]*"/ {
      s = $0; sub(/.*sha256[[:space:]]*"/, "", s); sub(/".*/, "", s); print s; exit
    }
  ' "$FORMULA"
}

OLD_VER="$(grep -E '^[[:space:]]*version[[:space:]]*"[0-9]+\.[0-9]+\.[0-9]+"' "$FORMULA" \
           | head -1 | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1)"
OLD_ARM="$(formula_sha arm)"
OLD_AMD="$(formula_sha intel)"

[ -n "$OLD_VER" ] && [ -n "$OLD_ARM" ] && [ -n "$OLD_AMD" ] || die \
  "${FORMULA} does not match the dual-arch shape.
  Expected a version \"X.Y.Z\" line plus a sha256 line inside each of an
  on_arm and an on_intel block. Refusing to guess."

if [ "$OLD_VER" = "$PLAIN" ] && [ "$OLD_ARM" = "$SHA_ARM" ] && [ "$OLD_AMD" = "$SHA_AMD" ]; then
  note "SKIP ${FORMULA} — already at ${PLAIN} with matching per-arch hashes"
  CHANGED=false
else
  # -------------------------------------------------------------------------
  # 5. Write each hash into its own block.
  #    A block-tracking pass, not a global substitution: `sub()` fires only
  #    while the state machine says we are inside the matching arch block.
  # -------------------------------------------------------------------------
  TMP="$(mktemp)"
  trap 'rm -f "$TMP"' EXIT
  awk -v newv="$PLAIN" -v arm="$SHA_ARM" -v amd="$SHA_AMD" '
    /^[[:space:]]*version[[:space:]]*"[0-9]+\.[0-9]+\.[0-9]+"/ {
      sub(/"[0-9]+\.[0-9]+\.[0-9]+"/, "\"" newv "\"")
    }
    /^[[:space:]]*on_arm do/        { block = "arm" }
    /^[[:space:]]*on_intel do/      { block = "intel" }
    /^[[:space:]]*end[[:space:]]*$/ { block = "" }
    block == "arm"   && /^[[:space:]]*sha256[[:space:]]*"/ { sub(/"[0-9a-f]*"/, "\"" arm "\"") }
    block == "intel" && /^[[:space:]]*sha256[[:space:]]*"/ { sub(/"[0-9a-f]*"/, "\"" amd "\"") }
    { print }
  ' "$FORMULA" > "$TMP"
  cat "$TMP" > "$FORMULA"
  CHANGED=true

  # -----------------------------------------------------------------------
  # 6. Verify the write actually landed where intended. A silently no-op
  #    substitution is how a stale hash ships, so assert rather than assume.
  # -----------------------------------------------------------------------
  NEW_ARM="$(formula_sha arm)"
  NEW_AMD="$(formula_sha intel)"
  [ "$NEW_ARM" = "$SHA_ARM" ] || die "arm64 hash did not land in the on_arm block (got '${NEW_ARM}')"
  [ "$NEW_AMD" = "$SHA_AMD" ] || die "amd64 hash did not land in the on_intel block (got '${NEW_AMD}')"
  [ "$NEW_ARM" != "$NEW_AMD" ] || die "both blocks ended up with the same hash — the regression recurred"

  note "${FORMULA} version  ${OLD_VER} -> ${PLAIN}"
  note "${FORMULA} arm64    ${OLD_ARM:0:12}... -> ${SHA_ARM:0:12}..."
  note "${FORMULA} amd64    ${OLD_AMD:0:12}... -> ${SHA_AMD:0:12}..."
fi

# ---------------------------------------------------------------------------
# 7. Keep README.md's "Current Version" line in step with the formula.
#
#    This updater bumped Formula/nself.rb on every release but never touched
#    the README, so the README sat at v1.1.0 while the formula tracked five
#    further releases (found 2026-09-13: formula 1.3.6, README v1.1.0). A
#    hand-maintained duplicate of a generated value drifts by construction, so
#    the generator owns both and hygiene.yml fails the build if they disagree.
#
#    Deliberately tolerant: a repo without the marker line is left alone rather
#    than failing the release. The hygiene gate is what makes drift loud.
# ---------------------------------------------------------------------------
README="${README:-README.md}"
if [ -f "$README" ] && grep -qE '^- \*\*v[0-9]+\.[0-9]+\.[0-9]+\*\* - Current release' "$README"; then
  OLD_README_VER="$(grep -oE '^- \*\*v[0-9]+\.[0-9]+\.[0-9]+\*\*' "$README" | head -1 | tr -d '* v-')"
  if [ "$OLD_README_VER" != "$PLAIN" ]; then
    RTMP="$(mktemp)"
    sed -E "s|^- \*\*v[0-9]+\.[0-9]+\.[0-9]+\*\* - Current release|- **v${PLAIN}** - Current release|" \
      "$README" > "$RTMP"
    cat "$RTMP" > "$README"
    rm -f "$RTMP"
    # Assert the write landed — a silent no-op here is exactly how the drift
    # above went unnoticed for five releases.
    NEW_README_VER="$(grep -oE '^- \*\*v[0-9]+\.[0-9]+\.[0-9]+\*\*' "$README" | head -1 | tr -d '* v-')"
    [ "$NEW_README_VER" = "$PLAIN" ] \
      || die "README version did not update (still '${NEW_README_VER}', wanted '${PLAIN}')"
    note "${README} version  ${OLD_README_VER} -> ${PLAIN}"
  else
    note "SKIP ${README} — already at ${PLAIN}"
  fi
fi

if [ -n "$GH_OUTPUT" ]; then
  {
    echo "version=${TAG}"
    echo "plain_version=${PLAIN}"
    echo "sha256_arm64=${SHA_ARM}"
    echo "sha256_amd64=${SHA_AMD}"
    echo "changed=${CHANGED}"
  } >> "$GH_OUTPUT"
fi
