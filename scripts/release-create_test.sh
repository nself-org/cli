#!/usr/bin/env bash
# release-create_test.sh — Tests for release-create.sh (D-0727)
#
# Copies the script into a throwaway git repo and runs it with a fake `gh` that
# records its arguments. Checks:
#   1. No dist/ (tag time): DENY-001 and DENY-002 are skipped, the release goes through.
#   2. dist/ present without bundles or SBOMs: DENY-001 and DENY-002 still block.
#   3. DENY-004 asks for the license-audit run on main only.
#
# Run: bash scripts/release-create_test.sh
# Exit 0 = all pass, 1 = any failure.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TMP_BASE="$(mktemp -d)"
trap 'rm -rf "${TMP_BASE}"' EXIT

PASS=0
FAIL=0
ok()  { PASS=$((PASS+1)); printf '  PASS  %s\n' "$1"; }
bad() { FAIL=$((FAIL+1)); printf '  FAIL  %s\n    %s\n' "$1" "$2"; }

# fixture: a repo with one commit, the wrapper, the denylist file and a fake gh.
fixture() {
  local d="${TMP_BASE}/$1"
  mkdir -p "${d}/scripts" "${d}/.github" "${d}/bin"
  cp "${SCRIPT_DIR}/release-create.sh" "${d}/scripts/"
  echo '{}' > "${d}/.github/release-denylist.json"
  git -C "${d}" init -q
  git -C "${d}" -c user.name=t -c user.email=t@example.invalid commit -q --allow-empty -m init
  cat > "${d}/bin/gh" <<'EOF'
#!/usr/bin/env bash
echo "$*" >> "${GH_LOG}"
case "$1 $2" in
  "run list") echo success ;;
  "api repos/{owner}/{repo}/milestones?state=closed") echo closed ;;
  "release create") exit 0 ;;
esac
EOF
  chmod +x "${d}/bin/gh"
  echo "${d}"
}

run() { # run <fixture dir> -> stdout+stderr of the wrapper; exit status in RC
  local d="$1"
  OUT="$(cd "${d}" && PATH="${d}/bin:${PATH}" GH_LOG="${d}/gh.log" bash scripts/release-create.sh v9.9.9 2>&1)"
  RC=$?
}

# 1. tag time: no dist/
d="$(fixture notag)"
run "${d}"
if [ "${RC}" -eq 0 ] && grep -q '^release create v9.9.9' "${d}/gh.log"; then
  ok "no dist/: the release goes through"
else
  bad "no dist/: the release goes through" "rc=${RC} out=${OUT}"
fi
case "${OUT}" in
  *"DENY-001 is enforced post-build"*"DENY-002 is enforced post-build"*) ok "no dist/: the skip is announced for DENY-001 and DENY-002" ;;
  *) bad "no dist/: the skip is announced" "${OUT}" ;;
esac

# 2. dist/ exists but is empty: the post-build rules still bite
d="$(fixture emptydist)"
mkdir "${d}/dist"
run "${d}"
if [ "${RC}" -ne 0 ] && [[ "${OUT}" == *"DENY-001 signature-check"* ]] && [[ "${OUT}" == *"DENY-002 missing-sbom"* ]]; then
  ok "empty dist/: DENY-001 and DENY-002 block"
else
  bad "empty dist/: DENY-001 and DENY-002 block" "rc=${RC} out=${OUT}"
fi

# 3. DENY-004 reads the run on main
d="$(fixture branch)"
run "${d}"
if grep -q '^run list --workflow license-audit.yml --branch main ' "${d}/gh.log"; then
  ok "DENY-004 queries license-audit.yml on main"
else
  bad "DENY-004 queries license-audit.yml on main" "$(cat "${d}/gh.log")"
fi

printf '\n%d passed, %d failed\n' "${PASS}" "${FAIL}"
[ "${FAIL}" -eq 0 ]
