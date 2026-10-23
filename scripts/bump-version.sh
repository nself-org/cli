#!/usr/bin/env bash
# bump-version.sh — Atomic version bump across all lockstep files.
#
# Usage:
#   ./scripts/bump-version.sh <new-version> [--dry-run] [--homebrew]
#
# <new-version>   Required. Target version like "1.0.14" or "v1.0.14" (v stripped).
# --dry-run       Print what would change; write nothing.
# --homebrew      Phase 2: update homebrew formula after GitHub release exists.
#                 Without this flag, the formula is skipped and a reminder printed.
# --no-goldens    Skip the golden regeneration below (the release tool sets it for the
#                 admin-only lockstep bump, which commits nothing from cli).
#
# Goldens (same run, after the files above): the four version-hash goldens hash
# `.nself/build-version`, so every bump changes them. They are regenerated through
# the tests' own UPDATE_GOLDEN=1 path; the script then checks that only
# `.nself/build-version` lines changed and that the plain tests pass:
#   cli/internal/build/testdata/plan-mode/golden-{dev-minimal,fronted,prod-plugins}.txt
#   cli/internal/reconcile/testdata/fixtures/dev-minimal/golden-tree.txt
# Skipped with a note when the golden tests are absent (fixture trees).
#
# Files updated (11 + 1 optional homebrew) — retired Dart SDK removed 2026-06-30 (#159);
# sdk/ts-sdk added 2026-07-06 (was missing, causing @nself/sdk version lag):
#   1.  cli/.github/VERSION
#   2.  cli/internal/version/version.go       Version string = "x.y.z" (type-annotated)
#   3.  cli/sdk/go/doc.go                     const Version = "x.y.z"
#   4.  cli/sdk/ts/package.json               "version": "x.y.z"   (via jq)
#   5.  cli/sdk/ts-sdk/package.json           "version": "x.y.z"   (via jq)
#   6.  cli/sdk/py/pyproject.toml             version = "x.y.z" under [project]
#   7.  admin/package.json                    "version": "x.y.z"   (via jq)
#   8.  admin/src/lib/cli-version.ts          CLI_VERSION = 'x.y.z'
#   9.  admin/Dockerfile                      ARG NSELF_VERSION=x.y.z
#   10. admin/Dockerfile                      ENV ADMIN_VERSION=x.y.z
#   11. admin/Dockerfile                      LABEL org.opencontainers.image.version="x.y.z"
# +12. homebrew-nself/Formula/nself.rb       version + darwin arm64/amd64 sha256
#                                             from release checksums.txt (--homebrew only)
#
# Exit codes:
#   0 — success (all changes applied, or all already at target in idempotent run)
#   1 — usage error, bad semver, missing file, or homebrew release not found
#
# Hard rules:
#   - Atomic writes: all file writes use tmp+rename, cleaned on EXIT
#   - Portable patterns: POSIX BRE only (no GNU \s); jq for JSON files
#   - Idempotent: skip files already at the target version
#   - shellcheck-clean (no echo -e, no ${var,,}, no declare -A)
#
# L-P98-06 fix (2026-05-13):
#   - Replaced GNU \s with POSIX [[:space:]] in all regex
#   - Routed JSON edits through jq to handle nested/contextual "version" fields
#   - Added Go type-annotated form: Version<spaces>string<spaces>=<spaces>"..."
#   - Added scripts/bump-version_test.sh with fixtures for all 11 line shapes

set -euo pipefail

# ── Argument parsing ───────────────────────────────────────────────────────────

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
NSELF_ROOT="$(cd "${REPO_ROOT}/.." && pwd)"

DRY_RUN=false
HOMEBREW=false
GOLDENS=true
RAW_VERSION=""

for arg in "$@"; do
  case "${arg}" in
    --dry-run)   DRY_RUN=true ;;
    --homebrew)  HOMEBREW=true ;;
    --no-goldens) GOLDENS=false ;;
    -*)
      printf 'Unknown flag: %s\n' "${arg}" >&2
      printf 'Usage: %s <version> [--dry-run] [--homebrew] [--no-goldens]\n' "$0" >&2
      exit 1
      ;;
    *)
      if [ -n "${RAW_VERSION}" ]; then
        printf 'Unexpected extra argument: %s\n' "${arg}" >&2
        exit 1
      fi
      RAW_VERSION="${arg}"
      ;;
  esac
done

# ── Semver validation ─────────────────────────────────────────────────────────

if [ -z "${RAW_VERSION}" ]; then
  printf 'Usage: %s <version> [--dry-run] [--homebrew] [--no-goldens]\n' "$0" >&2
  exit 1
fi

NEW_VERSION="${RAW_VERSION#v}"

if ! printf '%s' "${NEW_VERSION}" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$'; then
  printf 'ERROR: Invalid semver: "%s"\n' "${RAW_VERSION}" >&2
  printf '  Expected X.Y.Z (e.g. 1.0.14) or vX.Y.Z (e.g. v1.0.14)\n' >&2
  exit 1
fi

# ── Tool availability checks ──────────────────────────────────────────────────

if ! command -v jq >/dev/null 2>&1; then
  printf 'ERROR: jq is required for JSON file bumps (sdk/ts/package.json, sdk/ts-sdk/package.json, admin/package.json)\n' >&2
  printf '  Install: brew install jq  (macOS)  |  apt-get install jq  (Linux)\n' >&2
  exit 1
fi

# ── File paths ────────────────────────────────────────────────────────────────

VERSION_FILE="${REPO_ROOT}/.github/VERSION"
# The version before this run edits anything: the golden step needs it (see update_goldens).
OLD_VERSION="$(tr -d '[:space:]' < "${VERSION_FILE}" 2>/dev/null || true)"
VERSION_GO="${REPO_ROOT}/internal/version/version.go"
SDK_GO="${REPO_ROOT}/sdk/go/doc.go"
SDK_TS="${REPO_ROOT}/sdk/ts/package.json"
SDK_PY="${REPO_ROOT}/sdk/py/pyproject.toml"
SDK_TS_SDK="${REPO_ROOT}/sdk/ts-sdk/package.json"
# SDK_FLUTTER removed 2026-06-30 (#159, ASI Policy 2)
ADMIN_PKG="${NSELF_ROOT}/admin/package.json"
ADMIN_CLIVER="${NSELF_ROOT}/admin/src/lib/cli-version.ts"
ADMIN_DOCKERFILE="${NSELF_ROOT}/admin/Dockerfile"
HOMEBREW_FORMULA="${NSELF_ROOT}/homebrew-nself/Formula/nself.rb"

# ── Tmp file cleanup ───────────────────────────────────────────────────────────

TMPFILES=()
GOLD_BAK=""
cleanup_tmpfiles() {
  [ -z "${GOLD_BAK}" ] || rm -rf "${GOLD_BAK}"
  for f in "${TMPFILES[@]+"${TMPFILES[@]}"}"; do
    rm -f "${f}"
  done
}
trap cleanup_tmpfiles EXIT

# ── Helper functions ──────────────────────────────────────────────────────────

# atomic_write <dest> <content-via-stdin>
atomic_write() {
  local dest="$1"
  local tmp="${dest}.bumpver.tmp"
  TMPFILES+=("${tmp}")

  cat > "${tmp}"

  if [ "${DRY_RUN}" = "true" ]; then
    diff -u "${dest}" "${tmp}" || true
    rm -f "${tmp}"
    return 0
  fi

  mv "${tmp}" "${dest}"
}

# report <file> <old> <new>
report() {
  printf '  %-60s  %s -> %s\n' "$1" "$2" "$3"
}

# extract_semver <file>
#   Extract the first X.Y.Z occurrence on any line matching a given grep pattern.
#   Args: file, grep-pattern
extract_semver() {
  grep -E "$2" "$1" | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1
}

# update_json_version <file>
#   Top-level .version field replacement via jq. Atomic via tmp+rename.
update_json_version() {
  local f="$1"
  if [ ! -f "${f}" ]; then
    printf 'ERROR: Missing file: %s\n' "${f}" >&2; exit 1
  fi
  local old
  old="$(jq -r '.version // ""' "${f}")"
  if [ "${old}" = "${NEW_VERSION}" ]; then
    printf '  SKIP  %s  (already %s)\n' "${f}" "${NEW_VERSION}"
    return
  fi
  if [ -z "${old}" ]; then
    printf 'ERROR: %s has no top-level "version" field\n' "${f}" >&2
    exit 1
  fi
  jq --arg v "${NEW_VERSION}" '.version = $v' "${f}" | atomic_write "${f}"
  report "${f}" "${old}" "${NEW_VERSION}"
}

# ── Per-file update functions ─────────────────────────────────────────────────

update_version_file() {
  local f="${VERSION_FILE}"
  if [ ! -f "${f}" ]; then
    printf 'ERROR: Missing file: %s\n' "${f}" >&2; exit 1
  fi
  local old
  old="$(tr -d '[:space:]' < "${f}")"
  if [ "${old}" = "${NEW_VERSION}" ]; then
    printf '  SKIP  %s  (already %s)\n' "${f}" "${NEW_VERSION}"
    return
  fi
  printf '%s\n' "${NEW_VERSION}" | atomic_write "${f}"
  report "${f}" "${old}" "${NEW_VERSION}"
}

# version.go shape: `\tVersion   string = "1.0.13"` (tab/spaces, type annotation)
update_version_go() {
  local f="${VERSION_GO}"
  if [ ! -f "${f}" ]; then
    printf 'ERROR: Missing file: %s\n' "${f}" >&2; exit 1
  fi
  local old
  old="$(extract_semver "${f}" '^[[:space:]]+Version[[:space:]]+string[[:space:]]*=')"
  if [ -z "${old}" ]; then
    printf 'ERROR: %s has no matching "Version<sp>string<sp>=" line\n' "${f}" >&2
    exit 1
  fi
  if [ "${old}" = "${NEW_VERSION}" ]; then
    printf '  SKIP  %s  (already %s)\n' "${f}" "${NEW_VERSION}"
    return
  fi
  # POSIX BRE — explicit [[:space:]]+, no \s
  sed "s/^\([[:space:]]*Version[[:space:]][[:space:]]*string[[:space:]]*=[[:space:]]*\"\)[0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*/\1${NEW_VERSION}/" "${f}" \
    | atomic_write "${f}"
  report "${f}" "${old}" "${NEW_VERSION}"
}

# sdk/go/doc.go shape: `const Version = "2.0.0"`
update_sdk_go_doc() {
  local f="${SDK_GO}"
  if [ ! -f "${f}" ]; then
    printf 'ERROR: Missing file: %s\n' "${f}" >&2; exit 1
  fi
  local old
  old="$(extract_semver "${f}" '^const Version = ')"
  if [ -z "${old}" ]; then
    # doc.go may not declare a const Version; treat as soft-skip
    printf '  SKIP  %s  (no const Version declaration)\n' "${f}"
    return
  fi
  if [ "${old}" = "${NEW_VERSION}" ]; then
    printf '  SKIP  %s  (already %s)\n' "${f}" "${NEW_VERSION}"
    return
  fi
  sed "s/^const Version = \"[0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*\"/const Version = \"${NEW_VERSION}\"/" "${f}" \
    | atomic_write "${f}"
  report "${f}" "${old}" "${NEW_VERSION}"
}

update_sdk_ts_pkg() {
  update_json_version "${SDK_TS}"
}

# sdk/ts-sdk/package.json — the @nself/sdk consumer client (NselfClient),
# distinct from sdk/ts (@nself/plugin-sdk). Publishes in lockstep with the
# CLI per sdk-ts-sdk-publish.yml ("Triggers on: cli release tag (v*)").
# Added 2026-07-06 after #179 (missing from this bumper caused @nself/sdk
# to lag at 1.1.9 while VERSION moved to 1.2.4 — SDK Version Coherence fail).
update_sdk_ts_sdk_pkg() {
  update_json_version "${SDK_TS_SDK}"
}

# pyproject.toml shape: `version = "2.0.0"` under [project]
# Also handles Cargo.toml shape: `version = "X.Y.Z"` under [package]
# Uses awk to scope replacement to the FIRST matching version line after the
# expected [project] or [package] section header — safer than naive sed on
# files with [dependencies.foo] sub-tables containing their own version fields.
update_toml_version() {
  local f="$1"
  local section="${2:-project}"  # default [project] for pyproject; pass "package" for Cargo
  if [ ! -f "${f}" ]; then
    printf 'ERROR: Missing file: %s\n' "${f}" >&2; exit 1
  fi
  local old
  # Extract: first version = "X.Y.Z" line inside [section]…(next section or EOF)
  old="$(awk -v sect="[${section}]" '
    $0 == sect { in_section = 1; next }
    in_section && /^\[/ { in_section = 0 }
    in_section && /^[[:space:]]*version[[:space:]]*=[[:space:]]*"[0-9]+\.[0-9]+\.[0-9]+"/ {
      match($0, /[0-9]+\.[0-9]+\.[0-9]+/)
      print substr($0, RSTART, RLENGTH); exit
    }
  ' "${f}")"
  if [ -z "${old}" ]; then
    printf 'ERROR: %s has no "version = X.Y.Z" line under [%s]\n' "${f}" "${section}" >&2
    exit 1
  fi
  if [ "${old}" = "${NEW_VERSION}" ]; then
    printf '  SKIP  %s  (already %s)\n' "${f}" "${NEW_VERSION}"
    return
  fi
  awk -v sect="[${section}]" -v newv="${NEW_VERSION}" '
    $0 == sect { in_section = 1; print; next }
    in_section && /^\[/ { in_section = 0 }
    in_section && !replaced && /^[[:space:]]*version[[:space:]]*=[[:space:]]*"[0-9]+\.[0-9]+\.[0-9]+"/ {
      sub(/[0-9]+\.[0-9]+\.[0-9]+/, newv)
      replaced = 1
    }
    { print }
  ' "${f}" | atomic_write "${f}"
  report "${f}" "${old}" "${NEW_VERSION}"
}

update_sdk_py_pyproject() {
  update_toml_version "${SDK_PY}" project
}

update_admin_pkg() {
  update_json_version "${ADMIN_PKG}"
}

update_admin_cliver() {
  local f="${ADMIN_CLIVER}"
  if [ ! -f "${f}" ]; then
    printf 'ERROR: Missing file: %s\n' "${f}" >&2; exit 1
  fi
  local old
  old="$(extract_semver "${f}" "^export const CLI_VERSION = '")"
  if [ -z "${old}" ]; then
    printf 'ERROR: %s has no matching "export const CLI_VERSION = " line\n' "${f}" >&2
    exit 1
  fi
  if [ "${old}" = "${NEW_VERSION}" ]; then
    printf '  SKIP  %s  (already %s)\n' "${f}" "${NEW_VERSION}"
    return
  fi
  sed "s/^export const CLI_VERSION = '[0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*'/export const CLI_VERSION = '${NEW_VERSION}'/" "${f}" \
    | atomic_write "${f}"
  report "${f}" "${old}" "${NEW_VERSION}"
}

update_admin_dockerfile() {
  local f="${ADMIN_DOCKERFILE}"
  if [ ! -f "${f}" ]; then
    printf 'ERROR: Missing file: %s\n' "${f}" >&2; exit 1
  fi
  local old_nself old_admin old_label
  old_nself="$(extract_semver "${f}" '^ARG NSELF_VERSION=')"
  old_admin="$(extract_semver "${f}" '^ENV ADMIN_VERSION=')"
  old_label="$(extract_semver "${f}" '^LABEL org\.opencontainers\.image\.version=')"

  if [ "${old_nself}" = "${NEW_VERSION}" ] && [ "${old_admin}" = "${NEW_VERSION}" ] && [ "${old_label}" = "${NEW_VERSION}" ]; then
    printf '  SKIP  %s  (all three lines already %s)\n' "${f}" "${NEW_VERSION}"
    return
  fi

  sed \
    -e "s/^ARG NSELF_VERSION=[0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*/ARG NSELF_VERSION=${NEW_VERSION}/" \
    -e "s/^ENV ADMIN_VERSION=[0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*/ENV ADMIN_VERSION=${NEW_VERSION}/" \
    -e "s/^LABEL org\.opencontainers\.image\.version=\"[0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*\"/LABEL org.opencontainers.image.version=\"${NEW_VERSION}\"/" \
    "${f}" | atomic_write "${f}"
  report "${f} (ARG NSELF_VERSION)" "${old_nself:-?}" "${NEW_VERSION}"
  report "${f} (ENV ADMIN_VERSION)" "${old_admin:-?}" "${NEW_VERSION}"
  report "${f} (LABEL image.version)" "${old_label:-?}" "${NEW_VERSION}"
}

# formula_sha <file> <arm|intel>
#   Extract the current sha256 inside the formula's on_arm or on_intel block.
formula_sha() {
  awk -v want="$2" '
    /^[[:space:]]*on_arm do/        { block = "arm" }
    /^[[:space:]]*on_intel do/      { block = "intel" }
    /^[[:space:]]*end[[:space:]]*$/ { block = "" }
    block == want && /^[[:space:]]*sha256[[:space:]]*"/ {
      sub(/^[^"]*"/, ""); sub(/".*$/, ""); print; exit
    }
  ' "$1"
}

# The formula uses dual-arch darwin binary release assets
# (nself-<ver>-darwin-{arm64,amd64}.tar.gz) whose urls interpolate #{version},
# so only the version line and the two per-arch sha256 lines change. The
# sha256 values come from the release's checksums.txt, never computed locally.
update_homebrew_formula() {
  local f="${HOMEBREW_FORMULA}"
  if [ ! -f "${f}" ]; then
    printf 'ERROR: Missing file: %s\n' "${f}" >&2; exit 1
  fi

  local checksums_url="https://github.com/nself-org/cli/releases/download/v${NEW_VERSION}/checksums.txt"
  local asset_arm="nself-${NEW_VERSION}-darwin-arm64.tar.gz"
  local asset_amd="nself-${NEW_VERSION}-darwin-amd64.tar.gz"

  if [ "${DRY_RUN}" = "true" ]; then
    printf '  [dry-run] Would fetch %s\n' "${checksums_url}"
    printf '  [dry-run] Would update version + darwin arm64/amd64 sha256 lines in %s\n' "${f}"
    return
  fi

  printf '  Fetching release checksums: %s\n' "${checksums_url}"
  local checksums
  if ! checksums="$(curl -fsSL "${checksums_url}")"; then
    printf 'ERROR: checksums.txt not found for v%s on GitHub.\n' "${NEW_VERSION}" >&2
    printf '  Push the tag and wait for the GitHub release before running --homebrew.\n' >&2
    printf '  URL checked: %s\n' "${checksums_url}" >&2
    exit 1
  fi

  # checksums.txt shape: `<64-hex-sha256>  <asset-filename>` per line.
  local sha_arm sha_amd
  sha_arm="$(printf '%s\n' "${checksums}" | awk -v a="${asset_arm}" '$2 == a { print $1; exit }')"
  sha_amd="$(printf '%s\n' "${checksums}" | awk -v a="${asset_amd}" '$2 == a { print $1; exit }')"

  if ! printf '%s' "${sha_arm}" | grep -qE '^[0-9a-f]{64}$'; then
    printf 'ERROR: No valid sha256 for %s in checksums.txt\n' "${asset_arm}" >&2
    exit 1
  fi
  if ! printf '%s' "${sha_amd}" | grep -qE '^[0-9a-f]{64}$'; then
    printf 'ERROR: No valid sha256 for %s in checksums.txt\n' "${asset_amd}" >&2
    exit 1
  fi

  local old_ver old_sha_arm old_sha_amd
  old_ver="$(extract_semver "${f}" '^[[:space:]]*version[[:space:]]+"')"
  old_sha_arm="$(formula_sha "${f}" arm)"
  old_sha_amd="$(formula_sha "${f}" intel)"

  if [ -z "${old_ver}" ] || [ -z "${old_sha_arm}" ] || [ -z "${old_sha_amd}" ]; then
    printf 'ERROR: %s does not match the dual-arch formula shape\n' "${f}" >&2
    printf '  (need a version "X.Y.Z" line plus sha256 lines inside on_arm and on_intel blocks)\n' >&2
    exit 1
  fi

  if [ "${old_ver}" = "${NEW_VERSION}" ] && [ "${old_sha_arm}" = "${sha_arm}" ] && [ "${old_sha_amd}" = "${sha_amd}" ]; then
    printf '  SKIP  %s  (already %s with matching sha256 values)\n' "${f}" "${NEW_VERSION}"
    return
  fi

  awk -v newv="${NEW_VERSION}" -v arm="${sha_arm}" -v amd="${sha_amd}" '
    /^[[:space:]]*version[[:space:]]*"[0-9]+\.[0-9]+\.[0-9]+"/ {
      sub(/"[0-9]+\.[0-9]+\.[0-9]+"/, "\"" newv "\"")
    }
    /^[[:space:]]*on_arm do/        { block = "arm" }
    /^[[:space:]]*on_intel do/      { block = "intel" }
    /^[[:space:]]*end[[:space:]]*$/ { block = "" }
    block == "arm"   && /^[[:space:]]*sha256[[:space:]]*"/ { sub(/"[0-9a-f]*"/, "\"" arm "\"") }
    block == "intel" && /^[[:space:]]*sha256[[:space:]]*"/ { sub(/"[0-9a-f]*"/, "\"" amd "\"") }
    { print }
  ' "${f}" | atomic_write "${f}"

  if [ "$(formula_sha "${f}" arm)" != "${sha_arm}" ] || [ "$(formula_sha "${f}" intel)" != "${sha_amd}" ]; then
    printf 'ERROR: sha256 replacement did not land in %s — check formula shape\n' "${f}" >&2
    exit 1
  fi

  report "${f} (version)" "${old_ver}" "${NEW_VERSION}"
  report "${f} (darwin-arm64 sha256)" "${old_sha_arm:0:12}..." "${sha_arm:0:12}..."
  report "${f} (darwin-amd64 sha256)" "${old_sha_amd:0:12}..." "${sha_amd:0:12}..."
}

# Version-hash goldens. UPDATE_GOLDEN=1 rewrites each golden whole, so the guard
# below refuses any change beyond the `.nself/build-version` lines: a bump must
# never launder an unrelated tree change into the goldens.
GOLDEN_FILES=(
  internal/build/testdata/plan-mode/golden-dev-minimal.txt
  internal/build/testdata/plan-mode/golden-fronted.txt
  internal/build/testdata/plan-mode/golden-prod-plugins.txt
  internal/reconcile/testdata/fixtures/dev-minimal/golden-tree.txt
)

# golden_tests [VAR=value...]: the two tests that own the goldens (the second
# one reads internal/reconcile/testdata/fixtures/dev-minimal/golden-tree.txt).
# The caller's UPDATE_GOLDEN and NSELF_L03_REF_BIN are cleared first: an exported
# UPDATE_GOLDEN would turn the plain re-run into a second rewrite, and a reference
# binary would regenerate the goldens for the old version.
golden_tests() {
  (cd "${REPO_ROOT}" \
    && env -u UPDATE_GOLDEN -u NSELF_L03_REF_BIN "$@" go test -count=1 ./internal/build -run 'TestWriteModeGolden$' \
    && env -u UPDATE_GOLDEN -u NSELF_L03_REF_BIN "$@" go test -count=1 ./cmd/commands -run 'TestBuildPlanDevGolden$')
}

# mask_version_hash FILE: FILE with the hash on each `.nself/build-version` line replaced by HASH.
mask_version_hash() {
  sed -E 's#^(f [0-7]{4} ([^ ]*/)?\.nself/build-version )[0-9a-f]{64}$#\1HASH#' "$1"
}

# restore_goldens: put the pre-regeneration copies back.
restore_goldens() {
  local g n=0
  for g in "${GOLDEN_FILES[@]}"; do
    n=$((n+1)); cp "${GOLD_BAK}/${n}" "${REPO_ROOT}/${g}"
  done
}

update_goldens() {
  if [ ! -f "${REPO_ROOT}/internal/build/plan_mode_test.go" ] || [ ! -f "${REPO_ROOT}/cmd/commands/build_plan_test.go" ]; then
    printf '  SKIP  version-hash goldens  (golden tests absent at this commit)\n'
    return
  fi
  if [ "${DRY_RUN}" = "true" ]; then
    printf '  [dry-run] Would regenerate %d version-hash goldens (UPDATE_GOLDEN=1 go test)\n' "${#GOLDEN_FILES[@]}"
    return
  fi
  if ! command -v go >/dev/null 2>&1; then
    printf 'ERROR: go is required to regenerate the version-hash goldens (or pass --no-goldens)\n' >&2
    exit 1
  fi
  local g n=0 stray
  for g in "${GOLDEN_FILES[@]}"; do
    if [ ! -f "${REPO_ROOT}/${g}" ]; then
      printf 'ERROR: Missing golden: %s\n' "${g}" >&2; exit 1
    fi
  done
  GOLD_BAK="$(mktemp -d)"
  for g in "${GOLDEN_FILES[@]}"; do
    n=$((n+1)); cp "${REPO_ROOT}/${g}" "${GOLD_BAK}/${n}"
  done
  printf '  Regenerating version-hash goldens (UPDATE_GOLDEN=1)\n'
  if ! golden_tests UPDATE_GOLDEN=1 >&2; then
    restore_goldens
    printf 'ERROR: golden regeneration failed; goldens restored\n' >&2
    exit 1
  fi
  n=0
  for g in "${GOLDEN_FILES[@]}"; do
    n=$((n+1))
    # Only the 64-hex hash of a `.nself/build-version` line may differ: mask it in both
    # copies and require the rest to be byte-identical, so a deleted, added, re-moded or
    # re-pathed line (or any other tree change) is refused, not just a stray non-version line.
    stray="$(diff <(mask_version_hash "${GOLD_BAK}/${n}") <(mask_version_hash "${REPO_ROOT}/${g}") | grep -E '^[<>]' || true)"
    if [ -n "${stray}" ]; then
      restore_goldens
      printf 'ERROR: %s changed beyond .nself/build-version lines; goldens restored.\n' "${g}" >&2
      printf '  The tree drifted for another reason: fix that deliberately, never inside a bump.\n' >&2
      exit 1
    fi
    if cmp -s "${GOLD_BAK}/${n}" "${REPO_ROOT}/${g}"; then
      # The hash covers the version, so an unchanged golden after a version move means the
      # test did not run (renamed, or skipped under -short): fail, never pass silently.
      if [ "${OLD_VERSION}" != "${NEW_VERSION}" ]; then
        printf 'ERROR: %s did not change although the version moved %s -> %s; the golden test did not regenerate it (renamed or skipped?).\n' "${g}" "${OLD_VERSION}" "${NEW_VERSION}" >&2
        exit 1
      fi
      printf '  SKIP  %s  (build-version hash unchanged)\n' "${g}"
    else
      printf '  %-60s  build-version hash -> %s\n' "${g}" "${NEW_VERSION}"
    fi
  done
  if ! golden_tests >&2; then
    printf 'ERROR: golden tests still fail after regeneration\n' >&2
    exit 1
  fi
}

# ── Main ───────────────────────────────────────────────────────────────────────

printf '\n'
if [ "${DRY_RUN}" = "true" ]; then
  printf 'DRY RUN — no files will be modified\n'
fi
printf 'Target version: %s\n\n' "${NEW_VERSION}"

if [ "${HOMEBREW}" = "true" ]; then
  printf 'Phase 2 — updating Homebrew formula only\n\n'
  update_homebrew_formula
  printf '\nDone. Homebrew formula updated to %s.\n' "${NEW_VERSION}"
  exit 0
fi

printf 'Phase 1 — updating 11 lockstep files + 4 version-hash goldens (Homebrew deferred to phase 2)\n\n'

update_version_file
update_version_go
update_sdk_go_doc
update_sdk_ts_pkg
update_sdk_ts_sdk_pkg
update_sdk_py_pyproject
update_admin_pkg
update_admin_cliver
update_admin_dockerfile
if [ "${GOLDENS}" = "true" ]; then
  update_goldens
fi

printf '\n'
if [ "${DRY_RUN}" = "true" ]; then
  printf 'Dry run complete — no files modified.\n'
else
  printf 'Version bump complete: all files now at %s\n' "${NEW_VERSION}"
  printf '\nNext steps:\n'
  printf '  1. Review the diff: git diff\n'
  printf '  2. Commit: git add -A && git commit -m "chore: bump version to v%s"\n' "${NEW_VERSION}"
  printf '  3. Push + tag: git tag v%s && git push origin main v%s\n' "${NEW_VERSION}" "${NEW_VERSION}"
  printf '  4. After GitHub release is published, run:\n'
  printf '     ./scripts/bump-version.sh %s --homebrew\n' "${NEW_VERSION}"
  printf '\nReminder: never hand-bump individual files — always use this script.\n'
fi
