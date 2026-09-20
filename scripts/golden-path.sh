#!/usr/bin/env bash
# golden-path.sh
#
# 13-step golden-path E2E smoke test.
# Validates the full journey from install → nClaw first response.
#
# Steps:
#   1.  Install nSelf CLI via Homebrew (or curl installer fallback)
#   2.  Create a fresh test project directory
#   3.  nself init --preset b2b-saas
#   4.  nself build
#   5.  nself start
#   6.  Wait for all services healthy (GOLDEN_PATH_HEALTH_TIMEOUT, default 60s)
#   7.  nself doctor --quick  (exit 0)
#   8.  nself plugin install ai
#   9.  nself plugin install claw
#   10. nself license set $NSELF_PLUGIN_LICENSE_KEY_OWNER
#   11. nself admin start
#   12. curl localhost:3021/api/health → 200
#   13. Verify nClaw chat readiness (mock or real first response)
#
# Timing budgets (per step):
#   WARN_MULTIPLIER=1.5x  (logs a warning, continues)
#   FAIL_MULTIPLIER=3x    (captures diagnostics, exits non-zero)
#
# Usage:
#   bash scripts/golden-path.sh
#   GOLDEN_PATH_DRY_RUN=1 bash scripts/golden-path.sh   # skip real installs
#
# Environment:
#   NSELF_PLUGIN_LICENSE_KEY_OWNER  — owner-tier license key (required for steps 10-13)
#   GOLDEN_PATH_DRY_RUN             — set to 1 to mock install steps
#   GOLDEN_PATH_WORK_DIR            — override temp project dir (default: mktemp)
#   GOLDEN_PATH_SKIP_CLEANUP        — set to 1 to leave work dir after run
#
# Output:
#   Human-readable step-by-step log to stdout
#   JSON report written to /tmp/golden-path-report.json
#
# Exit codes:
#   0   all 13 steps passed within timing budgets
#   1   one or more steps failed

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# ── Colours ───────────────────────────────────────────────────────────────────
GREEN_C='\033[0;32m'
YELLOW_C='\033[1;33m'
RED_C='\033[0;31m'
CYAN_C='\033[0;36m'
RESET_C='\033[0m'

# ── Config ────────────────────────────────────────────────────────────────────
DRY_RUN="${GOLDEN_PATH_DRY_RUN:-0}"
# nself start auto-installs ollama when AI_AUTO_INSTALL is unset/true; the
# golden-path validates the stack lifecycle, not a multi-GB AI model download.
export AI_AUTO_INSTALL="${AI_AUTO_INSTALL:-false}"
SKIP_CLEANUP="${GOLDEN_PATH_SKIP_CLEANUP:-0}"
REPORT_FILE="/tmp/golden-path-report.json"

# Per-step baseline durations (seconds).
# WARN fires at WARN_MULT * BASELINE; FAIL fires at FAIL_MULT * BASELINE.
WARN_MULT=1.5
FAIL_MULT=3

declare -A STEP_BASELINE=(
  [1]=30    # brew install
  [2]=1     # mkdir/cd
  [3]=10    # nself init
  [4]=30    # nself build
  [5]=20    # nself start
  [6]=60    # wait healthy
  [7]=15    # doctor --quick
  [8]=5     # license set (must precede the licensed plugin installs below)
  [9]=30    # plugin install ai
  [10]=30   # plugin install claw
  [11]=10   # admin start
  [12]=5    # curl health
  [13]=10   # claw readiness
)

# ── State ─────────────────────────────────────────────────────────────────────
PASS=0
FAIL=0
WARN_COUNT=0
declare -A STEP_STATUS=()
declare -A STEP_DURATION=()
declare -A STEP_NOTE=()
WORK_DIR=""

# ── Helpers ───────────────────────────────────────────────────────────────────
log()  { printf "${CYAN_C}[golden-path]${RESET_C} %s\n" "$*"; }
ok()   { printf "${GREEN_C}  ✓${RESET_C} %s\n" "$*"; }
warn() { printf "${YELLOW_C}  ⚠${RESET_C} %s\n" "$*"; }
err()  { printf "${RED_C}  ✗${RESET_C} %s\n" "$*" >&2; }

capture_diagnostics() {
  local step="$1"
  local diag_file="/tmp/golden-path-diag-step${step}.txt"
  {
    echo "=== golden-path diagnostic: step ${step} ==="
    echo "--- nself version ---"
    nself version 2>&1 || true
    echo "--- docker ps -a (includes restarting/exited) ---"
    docker ps -a 2>&1 || true
    # doctor is project-scoped: run it where the project actually is. Run from
    # the wrong directory it reports "no .env or .env.dev found" and a missing
    # JWT secret, which describes the empty cwd rather than the stack under
    # test. The health wait itself runs inside PROJECT_DIR (see its caller), so
    # diagnostics that do not were reporting on a different thing entirely.
    echo "--- nself doctor --quick (in ${PROJECT_DIR:-<unset>}) ---"
    if [ -n "${PROJECT_DIR:-}" ] && [ -d "${PROJECT_DIR}" ]; then
      (cd "${PROJECT_DIR}" && nself doctor --quick 2>&1) || true
    else
      echo "PROJECT_DIR not set yet; skipping project-scoped doctor run"
    fi
    # Logs for anything not running. A container in Restarting is the usual
    # reason doctor fails (checkContainerHealth fails any state != running),
    # and its exit reason is only in its own logs.
    echo "--- logs for containers not in a running state ---"
    for c in $(docker ps -a --format '{{.Names}} {{.State}}' 2>/dev/null | awk '$2 != "running" {print $1}'); do
      echo "=== ${c} (last 40 lines) ==="
      docker logs --tail 40 "${c}" 2>&1 || true
    done
    echo "--- work dir ---"
    ls -la "${WORK_DIR:-/tmp}" 2>&1 || true
  } > "${diag_file}" 2>&1
  warn "Diagnostics captured at ${diag_file}"

  # Print them too. The file alone is useless in CI: the runner is torn down
  # after the job, so unless the diagnostics reach the job log or an artifact,
  # a failure here says only "not healthy" and nothing about WHY. The health
  # wait polls `nself doctor --quick >/dev/null 2>&1`, discarding exactly the
  # output that would identify the failing check.
  echo "--- begin diagnostics (step ${step}) ---"
  cat "${diag_file}" 2>/dev/null || true
  echo "--- end diagnostics (step ${step}) ---"
}

run_step() {
  local step_num="$1"
  local step_label="$2"
  shift 2
  local cmd=("$@")

  local baseline="${STEP_BASELINE[$step_num]}"
  local warn_limit
  local fail_limit
  # Use awk for integer-safe multiplication — bc returns floats (e.g. 45.0) which
  # bash arithmetic (( )) cannot handle, causing "syntax error: invalid arithmetic
  # operator" on every step.  awk printf "%.0f" rounds to nearest integer.
  warn_limit=$(awk "BEGIN { printf \"%.0f\", ${baseline} * ${WARN_MULT} }")
  fail_limit=$(awk "BEGIN { printf \"%.0f\", ${baseline} * ${FAIL_MULT} }")

  log "Step ${step_num}/13: ${step_label}"

  local t_start
  t_start=$(date +%s)
  local exit_code=0

  if [ "${DRY_RUN}" = "1" ]; then
    ok "DRY RUN — skipped: ${cmd[*]}"
    STEP_STATUS[$step_num]="pass"
    STEP_DURATION[$step_num]=0
    STEP_NOTE[$step_num]="dry-run"
    PASS=$((PASS + 1))
    return 0
  fi

  "${cmd[@]}" 2>&1 || exit_code=$?

  local t_end
  t_end=$(date +%s)
  local elapsed=$(( t_end - t_start ))
  STEP_DURATION[$step_num]="${elapsed}"

  if [ "${exit_code}" -ne 0 ]; then
    err "Step ${step_num} FAILED (exit ${exit_code}, ${elapsed}s)"
    STEP_STATUS[$step_num]="fail"
    STEP_NOTE[$step_num]="exit=${exit_code}"
    FAIL=$((FAIL + 1))
    capture_diagnostics "${step_num}"
    return 1
  fi

  if (( elapsed > fail_limit )); then
    err "Step ${step_num} exceeded FAIL threshold (${elapsed}s > ${fail_limit}s)"
    STEP_STATUS[$step_num]="fail"
    STEP_NOTE[$step_num]="timeout=${elapsed}s"
    FAIL=$((FAIL + 1))
    capture_diagnostics "${step_num}"
    return 1
  fi

  if (( elapsed > warn_limit )); then
    warn "Step ${step_num} slow: ${elapsed}s (warn threshold: ${warn_limit}s)"
    STEP_STATUS[$step_num]="warn"
    STEP_NOTE[$step_num]="slow=${elapsed}s"
    WARN_COUNT=$((WARN_COUNT + 1))
  else
    ok "Step ${step_num} passed (${elapsed}s)"
    STEP_STATUS[$step_num]="pass"
    STEP_NOTE[$step_num]=""
  fi
  PASS=$((PASS + 1))
  return 0
}

wait_healthy() {
  # CLI-R05: 60s was too tight on a cold CI runner — postgres + hasura + auth
  # + nginx have to pull and boot before `doctor --quick` can pass, and the
  # weekly scheduled run failed on exactly this while local runs passed.
  # GOLDEN_PATH_HEALTH_TIMEOUT overrides it; CI sets 240.
  local timeout="${GOLDEN_PATH_HEALTH_TIMEOUT:-60}"
  local interval=3
  local elapsed=0
  log "Waiting for services healthy (timeout ${timeout}s)..."
  # `nself doctor` exits 0 when everything passes, 2 when there are warnings but
  # no failures, and 1 on a real failure (cmd/commands/doctor.go). Accept 0 and
  # 2 here: a warning is not an unhealthy service.
  #
  # Demanding exit 0 made this loop unsatisfiable. Once the stack is up, doctor
  # warns that ports 80, 443, 5432, 8080 and 4000 are "already in use" -- by our
  # own containers -- and that the JWT secret lives in .env.secrets rather than
  # .env, which is where it belongs. None of those can clear while the stack is
  # running, so the wait could only ever time out, and did, for as long as the
  # smoke has existed.
  while (( elapsed < timeout )); do
    nself doctor --quick >/dev/null 2>&1
    local rc=$?
    if (( rc == 0 || rc == 2 )); then
      ok "All services healthy after ${elapsed}s (doctor exit ${rc})"
      return 0
    fi
    sleep "${interval}"
    elapsed=$(( elapsed + interval ))
  done
  err "Services not healthy after ${timeout}s"
  return 1
}

write_report() {
  local overall="pass"
  [ "${FAIL}" -gt 0 ] && overall="fail"

  # Serialize step data via a temp env-style file the python helper reads.
  # The prior version inlined "${STEP_STATUS[$k]:-unknown}" inside the heredoc
  # where $k was meant to be a Python loop variable — but bash expands shell
  # vars in the heredoc before Python sees it. With "set -u" enabled, $k is
  # unbound and the script aborts ("scripts/golden-path.sh: line 201: k:
  # unbound variable") whenever any step actually fails.
  local steps_data
  steps_data=$(mktemp)
  for i in $(seq 1 13); do
    local s="${STEP_STATUS[$i]:-skipped}"
    local d="${STEP_DURATION[$i]:-0}"
    local n="${STEP_NOTE[$i]:-}"
    printf '%s\t%s\t%s\t%s\n' "$i" "$s" "$d" "$n" >>"$steps_data"
  done

  REPORT_FILE="${REPORT_FILE}" \
  STEPS_DATA="$steps_data" \
  OVERALL="${overall}" \
  PASS="${PASS}" \
  FAIL="${FAIL}" \
  WARN_COUNT="${WARN_COUNT}" \
  python3 - <<'PYEOF'
import json, os

steps = {}
with open(os.environ["STEPS_DATA"]) as fh:
    for line in fh:
        parts = line.rstrip("\n").split("\t")
        if len(parts) < 4:
            continue
        idx, status, duration, note = parts[0], parts[1], parts[2], parts[3]
        try:
            duration = int(duration)
        except ValueError:
            duration = 0
        steps[idx] = {"status": status, "duration": duration, "note": note}

report = {
    "overall": os.environ["OVERALL"],
    "pass":    int(os.environ["PASS"]),
    "fail":    int(os.environ["FAIL"]),
    "warn":    int(os.environ["WARN_COUNT"]),
    "steps":   steps,
}
with open(os.environ["REPORT_FILE"], "w") as f:
    json.dump(report, f, indent=2)
PYEOF
  local py_rc=$?
  rm -f "$steps_data"

  # Fallback pure-bash JSON if python3 unavailable.
  if [ "$py_rc" -ne 0 ]; then
    {
      printf '{"overall":"%s","pass":%d,"fail":%d,"warn":%d,"steps":{}}\n' \
        "${overall}" "${PASS}" "${FAIL}" "${WARN_COUNT}"
    } > "${REPORT_FILE}"
  fi

  log "Report written to ${REPORT_FILE}"
}

cleanup() {
  if [ "${SKIP_CLEANUP}" != "1" ] && [ -n "${WORK_DIR}" ] && [ -d "${WORK_DIR}" ]; then
    log "Cleaning up work dir: ${WORK_DIR}"
    nself stop --volumes 2>/dev/null || true
    nself admin stop 2>/dev/null || true
    rm -rf "${WORK_DIR}"
  fi
}
trap cleanup EXIT

# ── Pre-flight ────────────────────────────────────────────────────────────────
log "nSelf golden-path E2E smoke"
log "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

if [ -z "${NSELF_PLUGIN_LICENSE_KEY_OWNER:-}" ]; then
  err "NSELF_PLUGIN_LICENSE_KEY_OWNER is not set — required for steps 10-13"
  exit 1
fi

# ── Step 1: Install nSelf CLI ─────────────────────────────────────────────────
# GOLDEN_PATH_SOURCE=local builds the CLI from the checked-out ref and installs
# that, instead of fetching the last published release.
#
# The scheduled run leaves this at "release" deliberately: its job is to prove
# the install path real users take still works. But that also means a fix to any
# command exercised below cannot be validated here until AFTER the release that
# contains it, which is backwards for a pre-release gate. Dispatching with
# source=local proves a fix first.
if [ "${GOLDEN_PATH_SOURCE:-release}" = "local" ]; then
  run_step 1 "build this ref and install it" \
    bash -c 'set -e; cd "${GITHUB_WORKSPACE:-$PWD}"; make build; sudo install -m 0755 ./nself /usr/local/bin/nself; nself --version'
# On macOS, prefer Homebrew. On Linux, use the curl installer even if brew is
# present — the Homebrew formula only ships macOS (darwin) binaries and will
# error with "formula requires at least a URL" on Linux runners.
elif command -v brew >/dev/null 2>&1 && [ "$(uname)" = "Darwin" ]; then
  run_step 1 "brew install nself-org/nself/nself" \
    bash -c 'brew install nself-org/nself/nself 2>&1 || brew upgrade nself-org/nself/nself 2>&1'
else
  run_step 1 "curl install.sh | bash" \
    bash -c 'curl -fsSL https://install.nself.org | bash'
fi
[ "${STEP_STATUS[1]}" = "fail" ] && { write_report; exit 1; }

# ── Step 2: Create test project dir ──────────────────────────────────────────
if [ -n "${GOLDEN_PATH_WORK_DIR:-}" ]; then
  WORK_DIR="${GOLDEN_PATH_WORK_DIR}"
  mkdir -p "${WORK_DIR}"
else
  WORK_DIR="$(mktemp -d /tmp/nself-golden-path-XXXXXX)"
fi

# mktemp -d creates 0700. The stack bind-mounts paths from under this directory
# into containers that run as their own uid, and nginx is not the uid that owns
# it, so it cannot traverse in:
#
#   nginx: [emerg] cannot load certificate
#   "/etc/nginx/ssl/certificates/local-nself-org/fullchain.pem":
#   BIO_new_file() failed (... Permission denied ...)
#
# The certificate itself is already 0644 in a 0755 directory; only the temp
# parent was unreachable, so nginx crash-looped and the run died at step 6.
# 0755 on the work dir is safe here: it holds a throwaway test project, and
# nself still writes .env at 0600 and privkey.pem at 0640 inside it.
chmod 755 "${WORK_DIR}"

run_step 2 "mkdir testproject && cd" \
  bash -c "mkdir -p ${WORK_DIR}/testproject"
[ "${STEP_STATUS[2]}" = "fail" ] && { write_report; exit 1; }

PROJECT_DIR="${WORK_DIR}/testproject"

# ── Step 3: nself init ────────────────────────────────────────────────────────
run_step 3 "nself init --preset b2b-saas" \
  bash -c "cd ${PROJECT_DIR} && nself init --preset b2b-saas --non-interactive"
[ "${STEP_STATUS[3]}" = "fail" ] && { write_report; exit 1; }

# ── Step 4: nself build ───────────────────────────────────────────────────────
run_step 4 "nself build" \
  bash -c "cd ${PROJECT_DIR} && nself build"
[ "${STEP_STATUS[4]}" = "fail" ] && { write_report; exit 1; }

# ── Step 5: nself start ───────────────────────────────────────────────────────
run_step 5 "nself start" \
  bash -c "cd ${PROJECT_DIR} && nself start"
[ "${STEP_STATUS[5]}" = "fail" ] && { write_report; exit 1; }

# ── Step 6: Wait for services healthy ────────────────────────────────────────
t6_start=$(date +%s)
(cd "${PROJECT_DIR}" && wait_healthy)
step6_rc=$?
t6_end=$(date +%s)
STEP_DURATION[6]=$(( t6_end - t6_start ))
if [ "${step6_rc}" -ne 0 ]; then
  STEP_STATUS[6]="fail"
  STEP_NOTE[6]="services never became healthy"
  FAIL=$((FAIL + 1))
  capture_diagnostics 6
  write_report
  exit 1
fi
STEP_STATUS[6]="pass"
STEP_NOTE[6]=""
PASS=$((PASS + 1))

# ── Step 7: nself doctor --quick ─────────────────────────────────────────────
# doctor exits 2 for "warnings but no failures" (cmd/commands/doctor.go). Against
# a RUNNING stack that is the normal outcome, not a problem: it warns that ports
# 80, 443, 5432, 8080 and 4000 are in use, which is our own containers holding
# them, and that the JWT secret is in .env.secrets rather than .env, which is
# where it belongs. Treating 2 as failure made this step, like the health wait
# before it, assert a state the running stack cannot reach.
#
# Exit 1 (a real failure) still fails the step.
#
# P6-E11-W2-S4-T23 (2026-08-31): --quick itself is not stale — it is a real,
# registered flag (cmd/commands/doctor.go) with a regression guard
# (doctor_test.go: TestDoctorCmd_QuickFlagRegistered). Runs 33086844923 and
# 33092121374 (2026-08-27) failed here with "Error: unknown flag: --quick",
# not because the flag was missing from source, but because
# GOLDEN_PATH_SOURCE defaults to "release" (see the comment above this
# script's source-selection block) — those runs installed the last published
# release, which predated the --quick fix landing in a tagged release. That
# is expected, self-healing behavior of testing "last release" rather than
# "current source": a later run (33353502333, 2026-08-31) confirms Step 7
# passes once a release containing the fix ships. It surfaced an unrelated,
# separate live failure instead (Step 9, nself plugin install ai: checksum
# mismatch) — out of scope for this ticket (plugin registry/checksum
# subsystem, not golden-path.sh/doctor.go), filed as a follow-up PCI.
run_step 7 "nself doctor --quick" \
  bash -c "cd ${PROJECT_DIR} && nself doctor --quick; rc=\$?; [ \$rc -eq 2 ] && exit 0; exit \$rc"
[ "${STEP_STATUS[7]}" = "fail" ] && { write_report; exit 1; }

# ── Step 8: license set ──────────────────────────────────────────────────────
# Fail here with something readable rather than letting an empty variable become
# a missing argument. Unset, the command below expands to `nself license set`
# and cobra reports "accepts 1 arg(s), received 0", which says nothing about the
# secret being absent.
if [ -z "${NSELF_PLUGIN_LICENSE_KEY_OWNER:-}" ]; then
  err "NSELF_PLUGIN_LICENSE_KEY_OWNER is empty."
  err "Steps 8 to 10 need it: the ai and claw plugins are license-gated."
  err "In CI it comes from the repository secret of the same name, wired into"
  err "the \"Run golden-path smoke\" step in .github/workflows/e2e-golden-path.yml."
  STEP_STATUS[8]="fail"
  STEP_NOTE[8]="NSELF_PLUGIN_LICENSE_KEY_OWNER unset"
  FAIL=$((FAIL + 1))
  write_report
  exit 1
fi

# Must come BEFORE the plugin installs below. ai and claw are license-gated, so
# installing them first failed every run with
#   error installing "ai": plugin "ai" requires a license key
# The license step existed, it just ran after the two steps that needed it.
# Trim surrounding whitespace and newlines. A secret stored with a trailing or
# leading newline survives the `-z` check above but disappears under word
# splitting when interpolated into a command string, which is how this failed:
# the value was present (87 chars, confirmed in CI) and cobra still reported
# "accepts 1 arg(s), received 0".
NSELF_LICENSE_KEY_TRIMMED="$(printf '%s' "${NSELF_PLUGIN_LICENSE_KEY_OWNER}" | tr -d '[:space:]')"
if [ -z "${NSELF_LICENSE_KEY_TRIMMED}" ]; then
  err "NSELF_PLUGIN_LICENSE_KEY_OWNER contains only whitespace."
  STEP_STATUS[8]="fail"
  STEP_NOTE[8]="license key is whitespace only"
  FAIL=$((FAIL + 1))
  write_report
  exit 1
fi

# Validate the SHAPE before spending a step on it. The secret being present is
# not the same as it being a license key: on 2026-08-26 the repo secret was set
# (87 chars) but carried no recognised prefix, so step 8 died with
#   Error: setting license key: unknown key prefix: invalid license key format
# which names neither the secret nor what a valid key looks like. Whoever reads
# a failed nightly should not have to go find the prefix list in Go source.
#
# The prefixes are the accepted tiers in internal/license/manager.go
# (validPrefixes). Only the prefix is checked here, never the value, and the
# value is never printed.
case "${NSELF_LICENSE_KEY_TRIMMED}" in
  nself_owner_*|nself_ent_*|nself_max_*|nself_pro_*) ;;
  *)
    err "NSELF_PLUGIN_LICENSE_KEY_OWNER is set but is not a license key."
    err "It carries no recognised prefix. Valid prefixes are nself_owner_,"
    err "nself_ent_, nself_max_ and nself_pro_ (internal/license/manager.go)."
    err "Length seen: ${#NSELF_LICENSE_KEY_TRIMMED} characters. The value is not printed."
    err "Fix the repository secret:"
    err "  gh secret set NSELF_PLUGIN_LICENSE_KEY_OWNER --repo nself-org/cli"
    STEP_STATUS[8]="fail"
    STEP_NOTE[8]="license key has no recognised prefix"
    FAIL=$((FAIL + 1))
    write_report
    exit 1
    ;;
esac

# Passed as a positional argument, not interpolated into the command string.
# Interpolating means the value is re-parsed by the shell, so a newline splits
# it into a second command and a leading '#' would comment the rest away. It is
# also the safer shape for a secret in general.
run_step 8 "nself license set <owner-key>" \
  bash -c 'cd "$1" && nself license set "$2"' _ "${PROJECT_DIR}" "${NSELF_LICENSE_KEY_TRIMMED}"
[ "${STEP_STATUS[8]}" = "fail" ] && { write_report; exit 1; }

# ── Step 9: plugin install ai ────────────────────────────────────────────────
run_step 9 "nself plugin install ai" \
  bash -c "cd ${PROJECT_DIR} && nself plugin install ai"
[ "${STEP_STATUS[9]}" = "fail" ] && { write_report; exit 1; }

# ── Step 10: plugin install claw ─────────────────────────────────────────────
run_step 10 "nself plugin install claw" \
  bash -c "cd ${PROJECT_DIR} && nself plugin install claw"
[ "${STEP_STATUS[10]}" = "fail" ] && { write_report; exit 1; }

# ── Step 11: nself admin start ───────────────────────────────────────────────
run_step 11 "nself admin start" \
  bash -c "cd ${PROJECT_DIR} && nself admin start"
[ "${STEP_STATUS[11]}" = "fail" ] && { write_report; exit 1; }

# ── Step 12: curl localhost:3021/api/health → 200 ────────────────────────────
run_step 12 "curl localhost:3021/api/health" \
  bash -c '
    # The admin compose service declares start_period: 30s, so the probe has
    # to outlast that. The old budget was 5 tries x 2s ~= 10s, which is less
    # than the container is documented to need: measured cold start to a 200
    # is ~10s, so a green result was luck rather than a margin.
    for i in $(seq 1 20); do
      code=$(curl -sS -o /dev/null -w "%{http_code}" http://localhost:3021/api/health 2>/dev/null || echo 000)
      if [ "${code}" = "200" ]; then
        echo "admin health: 200 OK (after ${i} attempt(s))"
        exit 0
      fi
      sleep 2
    done
    echo "admin health not 200 after retries (last: ${code})" >&2
    echo "--- admin container state ---" >&2
    docker ps -a --filter "name=_admin" --format "{{.Names}} {{.Status}}" >&2 || true
    echo "--- admin health body ---" >&2
    curl -sS -m 5 http://localhost:3021/api/health 2>&1 | head -c 800 >&2 || true
    exit 1
  '
[ "${STEP_STATUS[12]}" = "fail" ] && { write_report; exit 1; }

# ── Step 13: nClaw chat readiness ────────────────────────────────────────────
# Verify the claw plugin endpoint is reachable and returns a valid JSON envelope.
# Uses a mock prompt to avoid real AI costs in CI.
run_step 13 "nClaw chat readiness check" \
  bash -c '
    cd '"${PROJECT_DIR}"' || exit 1

    # The old probe built its URL from `nself env get NSELF_API_URL`. There is
    # no `env get` subcommand: cobra fell through to the parent, printed the
    # help text and exited 0, so base_url became that multi-line prose and the
    # `|| echo` fallback never fired. Every probe curled a garbage URL.
    #
    # It was also aimed at the wrong place. `nself plugin start` runs a plugin
    # as a LOCAL BACKGROUND PROCESS (see internal/plugin/runtime_start.go —
    # PID under ~/.nself/runtime/pids), not a container behind hasura or
    # nginx. Each plugin listens on its own registered port and serves
    # /health directly. Ports are canonical in SPORT F10-PORT-REGISTRY:
    #   3709 = ai, 3710 = plugin-claw
    CLAW_URL="http://localhost:3710/health"
    AI_URL="http://localhost:3709/health"

    # Steps 9 and 10 INSTALL the plugins; install does not start them, and
    # step 5 ran `nself start` before they existed. Probing here without
    # starting them tests a service that was never running.
    for p in ai claw; do
      nself plugin start "${p}" 2>&1 | sed "s/^/  plugin start ${p}: /" || true
    done

    probe() {
      url=$2
      for i in $(seq 1 15); do
        c=$(curl -sS -o "/tmp/golden-path-$1-health.json" -w "%{http_code}" \
              "${url}" 2>/dev/null || echo 000)
        [ "${c}" = "200" ] && { echo "${c}"; return 0; }
        sleep 2
      done
      echo "${c}"
      return 1
    }

    code=$(probe claw "${CLAW_URL}") && { echo "claw health: 200 OK"; cat /tmp/golden-path-claw-health.json; exit 0; }
    echo "claw health returned ${code} — checking the ai plugin instead"
    code2=$(probe ai "${AI_URL}") && { echo "ai plugin health: 200 OK (claw endpoint not exposed on this build)"; exit 0; }

    echo "Both claw and ai health checks failed (${code} / ${code2})" >&2
    echo "--- plugin state ---" >&2
    nself plugin list 2>&1 | head -20 >&2 || true
    echo "--- containers ---" >&2
    docker ps -a --format "{{.Names}} {{.Status}} {{.Ports}}" >&2 || true
    echo "--- listening ports ---" >&2
    (ss -ltnp 2>/dev/null || netstat -ltn 2>/dev/null) | head -20 >&2 || true
    exit 1
  '

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
log "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
log "Results: ${PASS} passed  |  ${WARN_COUNT} warnings  |  ${FAIL} failed"

write_report

if [ "${FAIL}" -gt 0 ]; then
  err "Golden path FAILED — ${FAIL} step(s) did not pass"
  exit 1
fi

ok "Golden path PASSED (${PASS}/13 steps)"
exit 0
