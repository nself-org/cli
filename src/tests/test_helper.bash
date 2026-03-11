#!/usr/bin/env bash
# test_helper.bash
# Inline assert helpers for the bats/ subdirectory.
# Provides assert_success, assert_failure, assert_output without bats-assert.
# Compatible with Bash 3.2+.

# Assert that the most recent `run` exited 0.
assert_success() {
  if [ "$status" -ne 0 ]; then
    printf "Expected exit 0 but got %s\nOutput:\n%s\n" "$status" "$output" >&2
    return 1
  fi
}

# Assert that the most recent `run` exited non-0.
assert_failure() {
  if [ "$status" -eq 0 ]; then
    printf "Expected non-zero exit but got 0\nOutput:\n%s\n" "$output" >&2
    return 1
  fi
}

# Assert output contains or matches a value.
# Usage:
#   assert_output "exact string"
#   assert_output --partial "substring"
#   assert_output --regexp "pattern"
assert_output() {
  local mode="exact"
  local expected=""

  if [ "$1" = "--partial" ]; then
    mode="partial"
    expected="$2"
  elif [ "$1" = "--regexp" ]; then
    mode="regexp"
    expected="$2"
  else
    mode="exact"
    expected="$1"
  fi

  case "$mode" in
    exact)
      if [ "$output" != "$expected" ]; then
        printf "Expected output:\n%s\nActual output:\n%s\n" "$expected" "$output" >&2
        return 1
      fi
      ;;
    partial)
      if ! printf '%s' "$output" | grep -qF "$expected" 2>/dev/null; then
        # Fallback: plain string match
        case "$output" in
          *"$expected"*) ;;
          *)
            printf "Expected output to contain: %s\nActual output:\n%s\n" "$expected" "$output" >&2
            return 1
            ;;
        esac
      fi
      ;;
    regexp)
      if ! printf '%s' "$output" | grep -qE "$expected" 2>/dev/null; then
        printf "Expected output to match regexp: %s\nActual output:\n%s\n" "$expected" "$output" >&2
        return 1
      fi
      ;;
  esac
}

# Assert output length is greater than N characters.
assert_output_length_gt() {
  local min_len="$1"
  local actual_len
  actual_len=$(printf '%s' "$output" | wc -c | tr -d ' ')
  if [ "$actual_len" -le "$min_len" ]; then
    printf "Expected output length > %s but got %s\nOutput:\n%s\n" \
      "$min_len" "$actual_len" "$output" >&2
    return 1
  fi
}

# Skip a test with a message (bats built-in `skip` works without this,
# but this provides a consistent pattern when calling from helper functions).
bats_skip() {
  skip "$@"
}
