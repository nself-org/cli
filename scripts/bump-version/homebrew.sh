# shellcheck shell=bash
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
