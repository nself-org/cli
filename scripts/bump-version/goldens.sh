# shellcheck shell=bash
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
