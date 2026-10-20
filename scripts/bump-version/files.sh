# shellcheck shell=bash
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

# Flutter SDK removed 2026-06-30 (#159, ASI Policy 2) — pubspec updater retired.

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

