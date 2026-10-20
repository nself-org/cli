# shellcheck shell=bash
# ── Spawn watchdog sidecar ────────────────────────────────────────────────────
# The watchdog is an INDEPENDENT process (nohup + detached).
# It cannot be killed by SIGKILL to this shell.
# It polls for DONE_MARKER and runs the verified restore if it never appears.
# The script body is a quoted heredoc: no parent state is interpolated; every
# value arrives as an argument and the restore logic is sourced from PS_LIB.

WATCHDOG_SCRIPT="${WORK_DIR}/nself-watchdog-${MERGE_TS}.sh"

if [ "${DRY_RUN}" -eq 0 ]; then
  cat > "${WATCHDOG_SCRIPT}" << 'WATCHDOG_EOF'
#!/usr/bin/env bash
# Watchdog for admin-merge.sh. Restores branch protection from the snapshot
# file if the parent dies before it writes the done marker.
# Args: lib repo snapshot done relaxed relax_ts deadline audit_log dry_run self parent_pid
set -u
LIB="$1"; REPO="$2"; SNAP="$3"; DONE_MARKER="$4"; RELAX_MARKER="$5"
RELAX_TS_MARKER="$6"; DEADLINE="$7"; AUDIT_LOG="$8"; DRY_RUN="$9"; SELF="${10}"; PARENT="${11}"
export AUDIT_LOG
# The parent verifies its own restore from the same snapshot file, so it is
# removed only once the parent is gone (a slow CI must not lose it).
drop_snapshot() { kill -0 "${PARENT}" 2>/dev/null || rm -f "${SNAP}"; }
drop_markers() { rm -f "${SELF}" "${RELAX_MARKER}" "${RELAX_TS_MARKER}"; }
# shellcheck source=/dev/null
. "${LIB}" || { printf '[WATCHDOG] cannot source %s\n' "${LIB}"; exit 2; }

deadline=$(( $(date '+%s') + DEADLINE ))

# Wait until relax marker exists (protection has been relaxed) or deadline
while [ ! -f "${RELAX_MARKER}" ]; do
  if [ "$(date '+%s')" -ge "${deadline}" ]; then
    printf '[WATCHDOG] Deadline reached waiting for relax marker. Exiting (nothing to restore).\n'
    drop_snapshot; drop_markers
    exit 0
  fi
  sleep 2
done

# FIX3: reset the deadline from the actual relax completion time.
if [ -f "${RELAX_TS_MARKER}" ]; then
  relax_epoch=$(cat "${RELAX_TS_MARKER}" 2>/dev/null || date '+%s')
  deadline=$(( relax_epoch + DEADLINE ))
  printf '[WATCHDOG] Deadline reset from relax time: epoch=%s deadline=%s\n' "${relax_epoch}" "${deadline}"
fi
printf '[WATCHDOG] Protection was relaxed. Monitoring for done marker...\n'

while [ ! -f "${DONE_MARKER}" ]; do
  if [ "$(date '+%s')" -ge "${deadline}" ]; then
    printf '[WATCHDOG] Deadline exceeded without done marker. Forcing restore.\n'
    rc=0
    restore_protection "${REPO}" "${SNAP}" "${DRY_RUN}" watchdog_timeout || rc=$?
    if [ "${rc}" -eq 0 ]; then drop_snapshot; fi
    if ! kill -0 "${PARENT}" 2>/dev/null; then drop_markers; fi
    printf '[WATCHDOG] exit %s\n' "${rc}"
    exit "${rc}"
  fi
  sleep 3
done

printf '[WATCHDOG] Done marker found. Parent completed cleanly — no restore needed.\n'
drop_markers
exit 0
WATCHDOG_EOF
  chmod +x "${WATCHDOG_SCRIPT}"
  nohup bash "${WATCHDOG_SCRIPT}" "${PS_LIB}" "${TARGET_REPO}" "${SNAPSHOT_FILE}" \
    "${DONE_MARKER}" "${RELAX_MARKER}" "${RELAX_TS_MARKER}" "${WATCHDOG_DEADLINE}" \
    "${AUDIT_LOG}" "${DRY_RUN}" "${WATCHDOG_SCRIPT}" "$$" > "${WORK_DIR}/nself-watchdog-${MERGE_TS}.log" 2>&1 &
  WATCHDOG_PID=$!
  info "Watchdog spawned: PID ${WATCHDOG_PID}, deadline ${WATCHDOG_DEADLINE}s"
  audit "WATCHDOG_SPAWN  pid=${WATCHDOG_PID}  deadline=${WATCHDOG_DEADLINE}  snapshot=${SNAPSHOT_FILE}"
else
  info "[DRY-RUN] Would spawn watchdog (skipped)"
fi

# FIX5: EXIT trap — on any exit (clean, error, SIGINT, SIGTERM) a relaxed branch
# is restored from the snapshot. The done marker is written only after a
# VERIFIED restore (or when nothing was relaxed); a failed restore leaves it
# absent so the watchdog still runs its own restore, and exits 3 for the owner.
_exit_trap_fired=0
_cleanup_on_exit() {
  local rc=$?   # bash 3.2 reports 0 here after a fatal set -u/arithmetic error; keep what we got
  if [ "${_exit_trap_fired}" -eq 1 ]; then return; fi
  _exit_trap_fired=1
  if [ -f "${RELAX_MARKER}" ] && [ ! -f "${DONE_MARKER}" ]; then
    if restore_protection "${TARGET_REPO}" "${SNAPSHOT_FILE}" "${DRY_RUN}" "exit_trap"; then
      touch "${DONE_MARKER}" 2>/dev/null || true
      rm -f "${SNAPSHOT_FILE}"
    else
      error "Branch protection NOT verified restored. Snapshot: ${SNAPSHOT_FILE}. Run: $0 --repo ${TARGET_REPO} --force-restore --snapshot ${SNAPSHOT_FILE}"
      exit 3
    fi
  elif [ ! -f "${RELAX_MARKER}" ]; then
    touch "${DONE_MARKER}" 2>/dev/null || true
  fi
  exit "${rc}"
}
trap '_cleanup_on_exit' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
