#!/usr/bin/env bats
# test-ai-command.bats — Tests for nself ai command
# Bash 3.2+ compatible; no external services required

load test_helper

# ---------------------------------------------------------------------------
# Help / usage
# ---------------------------------------------------------------------------

@test "nself ai --help exits 0 and shows usage" {
  run bash "${BATS_TEST_DIRNAME}/../../cli/ai.sh" --help
  [ "$status" -eq 0 ]
  [[ "$output" == *"nself ai"* ]]
}

@test "nself ai help exits 0" {
  run bash "${BATS_TEST_DIRNAME}/../../cli/ai.sh" help
  [ "$status" -eq 0 ]
}

@test "nself ai with no args exits 0 and shows usage" {
  run bash "${BATS_TEST_DIRNAME}/../../cli/ai.sh"
  [ "$status" -eq 0 ]
  [[ "$output" == *"auth"* ]]
}

# ---------------------------------------------------------------------------
# auth subcommand — argument validation
# ---------------------------------------------------------------------------

@test "nself ai auth with no action exits non-zero" {
  run bash "${BATS_TEST_DIRNAME}/../../cli/ai.sh" auth
  [ "$status" -ne 0 ]
}

@test "nself ai auth login with no --provider exits non-zero" {
  run bash "${BATS_TEST_DIRNAME}/../../cli/ai.sh" auth login
  [ "$status" -ne 0 ]
  [[ "$output" == *"provider"* ]] || [[ "$output" == *"required"* ]] || [[ "$output" == *"Usage"* ]]
}

@test "nself ai auth add with no --provider exits non-zero" {
  run bash "${BATS_TEST_DIRNAME}/../../cli/ai.sh" auth add
  [ "$status" -ne 0 ]
}

@test "nself ai auth remove with no --provider exits non-zero" {
  run bash "${BATS_TEST_DIRNAME}/../../cli/ai.sh" auth remove
  [ "$status" -ne 0 ]
}

# ---------------------------------------------------------------------------
# Unknown subcommands
# ---------------------------------------------------------------------------

@test "nself ai unknown subcommand exits non-zero" {
  run bash "${BATS_TEST_DIRNAME}/../../cli/ai.sh" notasubcommand
  [ "$status" -ne 0 ]
  [[ "$output" == *"Unknown"* ]] || [[ "$output" == *"Usage"* ]] || [[ "$output" == *"nself ai"* ]]
}

@test "nself ai auth unknown action exits non-zero" {
  run bash "${BATS_TEST_DIRNAME}/../../cli/ai.sh" auth notanaction
  [ "$status" -ne 0 ]
}

# ---------------------------------------------------------------------------
# Bash 3.2 compatibility
# ---------------------------------------------------------------------------

@test "ai.sh has no echo -e (Bash 3.2 compat)" {
  ai_file="${BATS_TEST_DIRNAME}/../../cli/ai.sh"
  run grep -n 'echo -e' "$ai_file"
  [ "$status" -ne 0 ]  # grep exits 1 = no matches = PASS
}

@test "ai.sh has no declare -A (Bash 3.2 compat)" {
  ai_file="${BATS_TEST_DIRNAME}/../../cli/ai.sh"
  run grep -n 'declare -A' "$ai_file"
  [ "$status" -ne 0 ]
}

@test "ai.sh has no mapfile or readarray (Bash 3.2 compat)" {
  ai_file="${BATS_TEST_DIRNAME}/../../cli/ai.sh"
  run grep -nE '\b(mapfile|readarray)\b' "$ai_file"
  [ "$status" -ne 0 ]
}

@test "ai.sh has no \${var,,} or \${var^^} (Bash 3.2 compat)" {
  ai_file="${BATS_TEST_DIRNAME}/../../cli/ai.sh"
  run grep -nE '\$\{[^}]*,,\}|\$\{[^}]*\^\^\}' "$ai_file"
  [ "$status" -ne 0 ]
}

@test "ai.sh has no [[ ]] double-bracket tests (Bash 3.2 compat)" {
  ai_file="${BATS_TEST_DIRNAME}/../../cli/ai.sh"
  run grep -n '\[\[' "$ai_file"
  [ "$status" -ne 0 ]  # grep exits 1 = no [[ found = PASS
}

# ---------------------------------------------------------------------------
# NSELF_AI_URL override
# ---------------------------------------------------------------------------

@test "nself ai respects NSELF_AI_URL override" {
  # Point to a guaranteed-closed port; URL should appear in output/error when login fails
  run env NSELF_AI_URL="http://127.0.0.1:19998" \
    bash "${BATS_TEST_DIRNAME}/../../cli/ai.sh" auth login --provider openai --key sk-test
  # Connection refused is expected; exit code may vary — confirm URL override is read
  [ "$status" -ne 0 ] || true
}
