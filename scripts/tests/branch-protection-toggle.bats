#!/usr/bin/env bats
# branch-protection-toggle.bats  (P7-HYG-48)
#
# Offline tests for `branch-protection-toggle.sh --on --dry-run`. `gh` is a
# stub on PATH: it serves a canned protection snapshot (a live GitHub response
# for nself-org/cli, taken 2026-10-04) or a canned error, and refuses and logs
# any write (-X ...). No test touches the network or GitHub.

SCRIPT="${BATS_TEST_DIRNAME}/../branch-protection-toggle.sh"

setup() {
  export HOME="${BATS_TEST_TMPDIR}/home"
  mkdir -p "${HOME}" "${BATS_TEST_TMPDIR}/bin"
  export GH_LOG="${BATS_TEST_TMPDIR}/gh.log"
  : > "${GH_LOG}"
  cat > "${BATS_TEST_TMPDIR}/bin/gh" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "${GH_LOG}"
case "$*" in
  *"-X "*) printf 'WRITE: %s\n' "$*" >> "${GH_LOG}.writes"; exit 99 ;;
  "api user"*) echo tester; exit 0 ;;
  "api repos/"*"/protection")
    [ -n "${GH_STUB_BODY:-}" ] && cat "${GH_STUB_BODY}"
    [ -n "${GH_STUB_ERR:-}" ] && printf '%s\n' "${GH_STUB_ERR}" >&2
    exit "${GH_STUB_RC:-0}" ;;
esac
exit 98
STUB
  chmod +x "${BATS_TEST_TMPDIR}/bin/gh"
  export PATH="${BATS_TEST_TMPDIR}/bin:${PATH}"

  LIVE="${BATS_TEST_TMPDIR}/live.json"
  cat > "${LIVE}" <<'JSON'
{
  "url": "https://api.github.com/repos/nself-org/cli/branches/main/protection",
  "required_status_checks": {
    "url": "https://api.github.com/repos/nself-org/cli/branches/main/protection/required_status_checks",
    "strict": false,
    "contexts": ["golangci-lint", "Vet, Build & Test (ubuntu-latest)"],
    "contexts_url": "https://api.github.com/repos/nself-org/cli/branches/main/protection/required_status_checks/contexts",
    "checks": [
      {"context": "golangci-lint", "app_id": 15368},
      {"context": "Vet, Build & Test (ubuntu-latest)", "app_id": 15368}
    ]
  },
  "required_pull_request_reviews": {
    "url": "https://api.github.com/repos/nself-org/cli/branches/main/protection/required_pull_request_reviews",
    "dismiss_stale_reviews": false,
    "require_code_owner_reviews": false,
    "require_last_push_approval": false,
    "required_approving_review_count": 1
  },
  "required_signatures": {
    "url": "https://api.github.com/repos/nself-org/cli/branches/main/protection/required_signatures",
    "enabled": false
  },
  "enforce_admins": {
    "url": "https://api.github.com/repos/nself-org/cli/branches/main/protection/enforce_admins",
    "enabled": true
  },
  "required_linear_history": {"enabled": false},
  "allow_force_pushes": {"enabled": false},
  "allow_deletions": {"enabled": false},
  "block_creations": {"enabled": false},
  "required_conversation_resolution": {"enabled": false},
  "lock_branch": {"enabled": false},
  "allow_fork_syncing": {"enabled": false}
}
JSON

  # Policy that states the real settings, with no url keys.
  POLICY="${BATS_TEST_TMPDIR}/policy.yaml"
  cat > "${POLICY}" <<'YAML'
required_status_checks:
  strict: false
  contexts:
    - "golangci-lint"
    - "Vet, Build & Test (ubuntu-latest)"
enforce_admins: true
required_pull_request_reviews:
  required_approving_review_count: 1
  dismiss_stale_reviews: false
  require_code_owner_reviews: false
  require_last_push_approval: false
restrictions: null
allow_force_pushes: false
allow_deletions: false
YAML
}

# run_dry [policy]: dry-run --on against the stubbed live snapshot.
run_dry() {
  export GH_STUB_BODY="${LIVE}"
  run "${SCRIPT}" --on --dry-run --repo nself-org/cli --policy "${1:-${POLICY}}"
}

# mutate <jq program>: change the live snapshot in place.
mutate() {
  jq "$1" "${LIVE}" > "${LIVE}.new" && mv "${LIVE}.new" "${LIVE}"
}

assert_match() {
  [ "${status}" -eq 0 ]
  [[ "${output}" == *"already matches baseline — no-op"* ]]
  [[ "${output}" != *"would PUT"* ]]
}

assert_drift() {
  [ "${status}" -eq 0 ]
  [[ "${output}" == *"DRY-RUN: would PUT"* ]]
  [[ "${output}" != *"already matches baseline"* ]]
}

no_writes() {
  [ ! -e "${GH_LOG}.writes" ]
}

@test "match: policy without url keys equals the live snapshot with url keys" {
  run_dry
  assert_match
  no_writes
}

@test "match: policy that still embeds url keys also matches" {
  sed -i.bak 's|^  required_approving_review_count: 1|  url: "https://api.github.com/x/required_pull_request_reviews"\n  required_approving_review_count: 1|' "${POLICY}"
  run_dry
  assert_match
}

@test "match: url values differing between live and policy are ignored" {
  mutate '.url = "https://elsewhere" | .required_pull_request_reviews.url = "https://elsewhere/r" | .required_status_checks.contexts_url = "https://elsewhere/c"'
  run_dry
  assert_match
}

@test "match: nested url / *_url keys under restrictions are dropped at every depth" {
  mutate '.restrictions = {url: "u", users_url: "uu", teams_url: "tu", apps_url: "au",
    users: [{login: "ali", id: 7, url: "x", avatar_url: "y", html_url: "z"}], teams: [], apps: []}'
  sed -i.bak '/^restrictions: null$/d' "${POLICY}"
  cat >> "${POLICY}" <<'YAML'
restrictions:
  users:
    - {login: ali, id: 7}
  teams: []
  apps: []
YAML
  run_dry
  assert_match
}

@test "drift: required_status_checks.strict" {
  mutate '.required_status_checks.strict = true'
  run_dry
  assert_drift
}

@test "drift: required_status_checks.contexts" {
  mutate '.required_status_checks.contexts += ["extra-check"]'
  run_dry
  assert_drift
}

@test "drift: enforce_admins" {
  mutate '.enforce_admins.enabled = false'
  run_dry
  assert_drift
}

@test "drift: required review count" {
  mutate '.required_pull_request_reviews.required_approving_review_count = 2'
  run_dry
  assert_drift
}

@test "drift: dismiss_stale_reviews" {
  mutate '.required_pull_request_reviews.dismiss_stale_reviews = true'
  run_dry
  assert_drift
}

@test "drift: require_code_owner_reviews" {
  mutate '.required_pull_request_reviews.require_code_owner_reviews = true'
  run_dry
  assert_drift
}

@test "drift: restrictions" {
  mutate '.restrictions = {url: "u", users: [{login: "ali", url: "x"}], teams: [], apps: []}'
  run_dry
  assert_drift
}

@test "drift: allow_force_pushes" {
  mutate '.allow_force_pushes.enabled = true'
  run_dry
  assert_drift
}

@test "drift: allow_deletions" {
  mutate '.allow_deletions.enabled = true'
  run_dry
  assert_drift
}

@test "drift: a changed review value is still caught when url keys also differ" {
  mutate '.required_pull_request_reviews.url = "https://elsewhere" | .required_pull_request_reviews.required_approving_review_count = 3'
  run_dry
  assert_drift
}

@test "404 Branch not found: n/a, exit 0, audit entry, no writes" {
  export GH_STUB_RC=1
  export GH_STUB_BODY="${BATS_TEST_TMPDIR}/err.json"
  printf '%s' '{"message":"Branch not found","documentation_url":"https://docs.github.com/rest/branches/branch-protection#get-branch-protection","status":"404"}' > "${GH_STUB_BODY}"
  export GH_STUB_ERR="gh: Branch not found (HTTP 404)"
  run "${SCRIPT}" --on --dry-run --repo nself-org/plugin-template --policy "${POLICY}"
  [ "${status}" -eq 0 ]
  [[ "${output}" == *"n/a: no default branch"* ]]
  [[ "${output}" != *"would PUT"* ]]
  grep -q '| n/a |' "${HOME}/.nself/branch-protection-audit.log"
  no_writes
}

@test "404 Branch not protected: still reads as unprotected (would PUT), exit 0" {
  export GH_STUB_RC=1
  export GH_STUB_BODY="${BATS_TEST_TMPDIR}/err.json"
  printf '%s' '{"message":"Branch not protected","status":"404"}' > "${GH_STUB_BODY}"
  export GH_STUB_ERR="gh: Branch not protected (HTTP 404)"
  run "${SCRIPT}" --on --dry-run --repo nself-org/cli --policy "${POLICY}"
  [ "${status}" -eq 0 ]
  [[ "${output}" == *"DRY-RUN: would PUT"* ]]
  no_writes
}

@test "403 plan-limit error is fatal (exit 2)" {
  export GH_STUB_RC=1
  export GH_STUB_BODY="${BATS_TEST_TMPDIR}/err.json"
  printf '%s' '{"message":"Upgrade to GitHub Pro or make this repository public to enable this feature.","status":"403"}' > "${GH_STUB_BODY}"
  export GH_STUB_ERR="gh: Upgrade to GitHub Pro or make this repository public to enable this feature. (HTTP 403)"
  run "${SCRIPT}" --on --dry-run --repo nself-org/web --policy "${POLICY}"
  [ "${status}" -eq 2 ]
  [[ "${output}" == *"gh API error reading protection"* ]]
  [[ "${output}" != *"n/a: no default branch"* ]]
  no_writes
}

@test "500 server error is fatal (exit 2)" {
  export GH_STUB_RC=1
  export GH_STUB_BODY=""
  export GH_STUB_ERR="gh: Server Error (HTTP 500)"
  run "${SCRIPT}" --on --dry-run --repo nself-org/cli --policy "${POLICY}"
  [ "${status}" -eq 2 ]
  no_writes
}

@test "404 Not Found for any other reason (e.g. unknown repo) is fatal" {
  export GH_STUB_RC=1
  export GH_STUB_BODY=""
  export GH_STUB_ERR="gh: Not Found (HTTP 404)"
  run "${SCRIPT}" --on --dry-run --repo nself-org/nope --policy "${POLICY}"
  [ "${status}" -eq 2 ]
  [[ "${output}" != *"n/a: no default branch"* ]]
  no_writes
}

@test "dry-run never calls a write verb on gh" {
  run_dry
  ! grep -qE -- '-X (PUT|PATCH|DELETE|POST)' "${GH_LOG}"
}
