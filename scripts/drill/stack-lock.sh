#!/usr/bin/env bash
# Host-wide lock for drill legs that start an nself stack (P7-CI-20). Sourced, never run.
#
# Purpose: the generated docker-compose.yml publishes fixed host ports (5432, 8080, 4000, 80,
#   443) and the nself-ci executor starts every drill job at once, so two legs that run
#   `nself start` would collide. Each such leg takes this lock before it starts a stack and
#   drops it on exit; the others wait. Legs that do not publish those ports do not need it.
# Inputs: DRILL_LOCK_DIR (default /tmp/nself-drill-stack.lock: one path for every job, whatever checkout copy or TMPDIR the executor gives it);
#   DRILL_LOCK_TIMEOUT (seconds to wait, default 3600; a leg's own job timeout also runs while it waits).
# Outputs: stack_lock_acquire (0 holding the lock, 1 after the timeout), stack_lock_release.
# Constraints: mkdir is the atomic step. The holder's pid is stored; a lock whose holder is gone is
#   taken over. Bash 3.2 compatible.

STACK_LOCK_DIR=${DRILL_LOCK_DIR:-/tmp/nself-drill-stack.lock}
STACK_LOCK_HELD=0

stack_lock_acquire() {
  local end holder
  end=$(($(date +%s) + ${DRILL_LOCK_TIMEOUT:-3600}))
  mkdir -p "$(dirname "$STACK_LOCK_DIR")" || return 1
  while :; do
    if mkdir "$STACK_LOCK_DIR" 2>/dev/null; then
      printf '%s\n' "$$" >"$STACK_LOCK_DIR/pid"
      STACK_LOCK_HELD=1
      return 0
    fi
    holder=$(cat "$STACK_LOCK_DIR/pid" 2>/dev/null || true)
    if [ -n "$holder" ] && ! kill -0 "$holder" 2>/dev/null; then
      # Takeover is guarded by a second mkdir mutex and re-reads the holder inside it, so two
      # waiters cannot both remove a lock the first one has already replaced.
      if mkdir "$STACK_LOCK_DIR.takeover" 2>/dev/null; then
        if [ "$(cat "$STACK_LOCK_DIR/pid" 2>/dev/null || true)" = "$holder" ]; then
          rm -rf "$STACK_LOCK_DIR"
        fi
        rmdir "$STACK_LOCK_DIR.takeover" 2>/dev/null || true
      fi
      continue
    fi
    [ "$(date +%s)" -ge "$end" ] && return 1
    printf 'drill: waiting for the stack lock held by pid %s\n' "${holder:-?}" >&2
    sleep 5
  done
}

stack_lock_release() {
  [ "$STACK_LOCK_HELD" = 1 ] || return 0
  STACK_LOCK_HELD=0
  rm -rf "$STACK_LOCK_DIR"
}
