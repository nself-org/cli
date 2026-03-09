#!/usr/bin/env bats
# test-command-tree.bats
# Auto-validates that every command listed in COMMAND-TREE-V1.md exists and
# responds to --help with exit 0 and non-trivial output.
#
# T-0358 — CLI: command tree completeness test
#
# No Docker required. No network required.
# Uses BATS_BASH variable when set (e.g. bash32 on Linux CI for Bash 3.2 testing).

load test_helper

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

# Run a command optionally under a specific bash binary.
# When BATS_BASH is set to "bash32", we exec the nself binary via that shell.
run_cmd() {
  if [ -n "${BATS_BASH:-}" ] && [ "${BATS_BASH}" != "bash" ]; then
    run "$BATS_BASH" "$(command -v nself)" "$@"
  else
    run nself "$@"
  fi
}

# ---------------------------------------------------------------------------
# Guard: minimum command count
# If the number of registered top-level commands ever drops below 30 this
# test will fail, preventing accidental removal of commands.
# ---------------------------------------------------------------------------

@test "nself --help lists at least 30 top-level commands" {
  run_cmd --help
  assert_success
  # Count lines that look like command names (leading whitespace + word chars)
  local count
  count=$(printf '%s' "$output" | grep -cE '^\s+[a-z][a-z0-9-]+\s' 2>/dev/null || true)
  if [ "${count:-0}" -lt 30 ]; then
    printf "Expected >= 30 commands in --help output, found: %s\nOutput:\n%s\n" \
      "${count:-0}" "$output" >&2
    return 1
  fi
}

# ---------------------------------------------------------------------------
# Core (5 commands)
# ---------------------------------------------------------------------------

@test "nself init --help exits 0 and has output" {
  run_cmd init --help
  assert_success
  assert_output_length_gt 10
}

@test "nself build --help exits 0 and has output" {
  run_cmd build --help
  assert_success
  assert_output_length_gt 10
}

@test "nself start --help exits 0 and has output" {
  run_cmd start --help
  assert_success
  assert_output_length_gt 10
}

@test "nself stop --help exits 0 and has output" {
  run_cmd stop --help
  assert_success
  assert_output_length_gt 10
}

@test "nself restart --help exits 0 and has output" {
  run_cmd restart --help
  assert_success
  assert_output_length_gt 10
}

# ---------------------------------------------------------------------------
# Utilities (16 commands)
# ---------------------------------------------------------------------------

@test "nself status --help exits 0 and has output" {
  run_cmd status --help
  assert_success
  assert_output_length_gt 10
}

@test "nself logs --help exits 0 and has output" {
  run_cmd logs --help
  assert_success
  assert_output_length_gt 10
}

@test "nself help exits 0 and has output" {
  run_cmd help
  assert_success
  assert_output_length_gt 10
}

@test "nself admin --help exits 0 and has output" {
  run_cmd admin --help
  assert_success
  assert_output_length_gt 10
}

@test "nself urls --help exits 0 and has output" {
  run_cmd urls --help
  assert_success
  assert_output_length_gt 10
}

@test "nself exec --help exits 0 and has output" {
  run_cmd exec --help
  assert_success
  assert_output_length_gt 10
}

@test "nself doctor --help exits 0 and has output" {
  run_cmd doctor --help
  assert_success
  assert_output_length_gt 10
}

@test "nself monitor --help exits 0 and has output" {
  run_cmd monitor --help
  assert_success
  assert_output_length_gt 10
}

@test "nself health --help exits 0 and has output" {
  run_cmd health --help
  assert_success
  assert_output_length_gt 10
}

@test "nself version --help exits 0 and has output" {
  run_cmd version --help
  assert_success
  assert_output_length_gt 10
}

@test "nself update --help exits 0 and has output" {
  run_cmd update --help
  assert_success
  assert_output_length_gt 10
}

@test "nself completion --help exits 0 and has output" {
  run_cmd completion --help
  assert_success
  assert_output_length_gt 10
}

@test "nself metrics --help exits 0 and has output" {
  run_cmd metrics --help
  assert_success
  assert_output_length_gt 10
}

@test "nself history --help exits 0 and has output" {
  run_cmd history --help
  assert_success
  assert_output_length_gt 10
}

@test "nself audit --help exits 0 and has output" {
  run_cmd audit --help
  assert_success
  assert_output_length_gt 10
}

@test "nself harden --help exits 0 and has output" {
  run_cmd harden --help
  assert_success
  assert_output_length_gt 10
}

# ---------------------------------------------------------------------------
# Complex commands (10 commands)
# ---------------------------------------------------------------------------

@test "nself db --help exits 0 and has output" {
  run_cmd db --help
  assert_success
  assert_output_length_gt 10
}

@test "nself tenant --help exits 0 and has output" {
  run_cmd tenant --help
  assert_success
  assert_output_length_gt 10
}

@test "nself deploy --help exits 0 and has output" {
  run_cmd deploy --help
  assert_success
  assert_output_length_gt 10
}

@test "nself infra --help exits 0 and has output" {
  run_cmd infra --help
  assert_success
  assert_output_length_gt 10
}

@test "nself service --help exits 0 and has output" {
  run_cmd service --help
  assert_success
  assert_output_length_gt 10
}

@test "nself config --help exits 0 and has output" {
  run_cmd config --help
  assert_success
  assert_output_length_gt 10
}

@test "nself auth --help exits 0 and has output" {
  run_cmd auth --help
  assert_success
  assert_output_length_gt 10
}

@test "nself dev --help exits 0 and has output" {
  run_cmd dev --help
  assert_success
  assert_output_length_gt 10
}

@test "nself plugin --help exits 0 and has output" {
  run_cmd plugin --help
  assert_success
  assert_output_length_gt 10
}

@test "nself nginx --help exits 0 and has output" {
  run_cmd nginx --help
  assert_success
  assert_output_length_gt 10
}

# ---------------------------------------------------------------------------
# Selected high-value subcommands
# These verify that subcommand routing works, not just top-level dispatch.
# ---------------------------------------------------------------------------

@test "nself db migrate --help exits 0" {
  run_cmd db migrate --help
  assert_success
  assert_output_length_gt 10
}

@test "nself db shell --help exits 0" {
  run_cmd db shell --help
  assert_success
  assert_output_length_gt 10
}

@test "nself db backup --help exits 0" {
  run_cmd db backup --help
  assert_success
  assert_output_length_gt 10
}

@test "nself db hasura --help exits 0" {
  run_cmd db hasura --help
  assert_success
  assert_output_length_gt 10
}

@test "nself tenant create --help exits 0" {
  run_cmd tenant create --help
  assert_success
  assert_output_length_gt 10
}

@test "nself deploy staging --help exits 0" {
  run_cmd deploy staging --help
  assert_success
  assert_output_length_gt 10
}

@test "nself deploy server --help exits 0" {
  run_cmd deploy server --help
  assert_success
  assert_output_length_gt 10
}

@test "nself infra provider --help exits 0" {
  run_cmd infra provider --help
  assert_success
  assert_output_length_gt 10
}

@test "nself infra k8s --help exits 0" {
  run_cmd infra k8s --help
  assert_success
  assert_output_length_gt 10
}

@test "nself service storage --help exits 0" {
  run_cmd service storage --help
  assert_success
  assert_output_length_gt 10
}

@test "nself config env --help exits 0" {
  run_cmd config env --help
  assert_success
  assert_output_length_gt 10
}

@test "nself config secrets --help exits 0" {
  run_cmd config secrets --help
  assert_success
  assert_output_length_gt 10
}

@test "nself auth ssl --help exits 0" {
  run_cmd auth ssl --help
  assert_success
  assert_output_length_gt 10
}

@test "nself auth oauth --help exits 0" {
  run_cmd auth oauth --help
  assert_success
  assert_output_length_gt 10
}

@test "nself dev frontend --help exits 0" {
  run_cmd dev frontend --help
  assert_success
  assert_output_length_gt 10
}

@test "nself plugin install --help exits 0" {
  run_cmd plugin install --help
  assert_success
  assert_output_length_gt 10
}

@test "nself plugin list --help exits 0" {
  run_cmd plugin list --help
  assert_success
  assert_output_length_gt 10
}

@test "nself nginx shared --help exits 0" {
  run_cmd nginx shared --help
  assert_success
  assert_output_length_gt 10
}
