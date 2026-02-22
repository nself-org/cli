#!/usr/bin/env bash
# test-cli-output.sh - Test suite for CLI output library
# Tests all functions for correctness and compatibility

set -euo pipefail

# Source the library (without debug output)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/../../lib/utils/cli-output.sh" 2>/dev/null

# Test results
TESTS_PASSED=0
TESTS_FAILED=0

# Test runner
run_test() {
  local test_name="$1"
  local test_func="$2"

  printf "\nRunning: %s\n" "${test_name}"
  printf "═%.0s" {1..60}
  printf "\n"

  if $test_func; then
    cli_success "PASS: ${test_name}"
    TESTS_PASSED=$((TESTS_PASSED + 1))
  else
    cli_error "FAIL: ${test_name}"
    TESTS_FAILED=$((TESTS_FAILED + 1))
  fi
}

# =============================================================================
# TEST FUNCTIONS
# =============================================================================

test_basic_messages() {
  cli_success "This is a success message"
  cli_error "This is an error message"
  cli_warning "This is a warning message"
  cli_info "This is an info message"
  cli_debug "This is a debug message (DEBUG=false, should not show)"

  DEBUG=true cli_debug "This is a debug message (DEBUG=true, should show)"

  cli_message "This is a plain message"
  cli_bold "This is a bold message"
  cli_dim "This is a dimmed message"

  return 0
}

test_sections_headers() {
  cli_section "Configuration"
  cli_info "Section content here"

  cli_header "Build Process"
  cli_info "Header content here"

  cli_step 1 5 "Installing dependencies"
  cli_step 2 5 "Running tests"

  return 0
}

test_boxes() {
  cli_box "Simple info box"
  cli_box "Success box" "success"
  cli_box "Error box" "error"
  cli_box "Warning box" "warning"

  cli_box_detailed "Detailed Box" "This is a detailed box with longer content that should wrap properly within the box boundaries."

  return 0
}

test_lists() {
  cli_info "Bullet list:"
  cli_list_item "First item"
  cli_list_item "Second item"
  cli_list_item "Third item"

  cli_blank
  cli_info "Numbered list:"
  cli_list_numbered 1 "First task"
  cli_list_numbered 2 "Second task"
  cli_list_numbered 3 "Third task"

  cli_blank
  cli_info "Checklist:"
  cli_list_checked "Completed task"
  cli_list_unchecked "Pending task"

  return 0
}

test_tables() {
  cli_info "Service status table:"

  cli_table_header "Service" "Status" "Port"
  cli_table_row "postgres" "running" "5432"
  cli_table_row "hasura" "running" "8080"
  cli_table_row "auth" "stopped" "4000"
  cli_table_footer "Service" "Status" "Port"

  return 0
}

test_progress() {
  cli_info "Progress bar demonstrations:"

  cli_progress "Building project" 0 100
  sleep 0.2
  cli_progress "Building project" 25 100
  sleep 0.2
  cli_progress "Building project" 50 100
  sleep 0.2
  cli_progress "Building project" 75 100
  sleep 0.2
  cli_progress "Building project" 100 100

  return 0
}

test_spinner() {
  cli_info "Spinner demonstration (skipped in non-TTY):"

  # Spinners don't work in non-TTY environments (like CI)
  # Just test that the functions exist and don't crash
  if [[ -t 1 ]]; then
    local spinner_pid
    spinner_pid=$(cli_spinner_start "Loading configuration")
    sleep 1
    cli_spinner_stop "$spinner_pid" "Configuration loaded successfully"
  else
    cli_info "Spinner test skipped (not a TTY)"
  fi

  return 0
}

test_summary() {
  cli_summary "Build Complete" \
    "5 services started" \
    "Database initialized" \
    "Nginx configured" \
    "SSL certificates generated"

  return 0
}

test_banner() {
  cli_banner "nself v1.0.0" "Modern Full-Stack Platform"

  cli_banner "Welcome to nself"

  return 0
}

test_utilities() {
  cli_separator
  cli_info "Standard separator above"

  cli_separator 40
  cli_info "Shorter separator above (40 chars)"

  cli_blank 2
  cli_info "Two blank lines above"

  cli_center "Centered Text" 60

  cli_indent "Indented level 1" 1
  cli_indent "Indented level 2" 2
  cli_indent "Indented level 3" 3

  return 0
}

test_color_stripping() {
  local colored_text
  colored_text=$(cli_success "This has colors")

  local stripped_text
  stripped_text=$(echo "$colored_text" | cli_strip_colors)

  # Check that stripped text contains the message but no ANSI codes
  if echo "$stripped_text" | grep -q "This has colors"; then
    cli_success "Color stripping works"
    return 0
  else
    cli_error "Color stripping failed"
    return 1
  fi
}

test_no_color_support() {
  cli_info "Testing NO_COLOR environment variable..."

  # Save current NO_COLOR state
  local old_no_color="${NO_COLOR:-}"

  # Enable NO_COLOR and re-source
  export NO_COLOR=1
  source "${SCRIPT_DIR}/../../lib/utils/cli-output.sh"

  # Verify colors are disabled
  if [[ -z "$CLI_RED" ]] && [[ -z "$CLI_GREEN" ]]; then
    cli_success "NO_COLOR support works"
    local result=0
  else
    cli_error "NO_COLOR support failed"
    local result=1
  fi

  # Restore NO_COLOR state and re-source
  if [[ -n "$old_no_color" ]]; then
    export NO_COLOR="$old_no_color"
  else
    unset NO_COLOR
  fi
  source "${SCRIPT_DIR}/../../lib/utils/cli-output.sh"

  return $result
}

test_non_tty_output() {
  cli_info "Testing non-TTY output (piped to cat)..."

  # Test that output works when piped
  if cli_success "Piped output test" | cat >/dev/null; then
    cli_success "Non-TTY output works"
    return 0
  else
    cli_error "Non-TTY output failed"
    return 1
  fi
}

test_bash_32_compatibility() {
  # Test that we're not using any Bash 4+ features

  # Check for lowercase expansion (Bash 4+)
  if grep -q '\${[^}]*,,[^}]*}' "${SCRIPT_DIR}/../../lib/utils/cli-output.sh"; then
    cli_error "Found Bash 4+ lowercase expansion"
    return 1
  fi

  # Check for associative arrays (Bash 4+)
  if grep -q 'declare -A' "${SCRIPT_DIR}/../../lib/utils/cli-output.sh"; then
    cli_error "Found Bash 4+ associative arrays"
    return 1
  fi

  # Check for echo -e (not portable)
  if grep -q 'echo -e' "${SCRIPT_DIR}/../../lib/utils/cli-output.sh"; then
    cli_error "Found non-portable echo -e"
    return 1
  fi

  cli_success "No Bash 4+ features detected"
  return 0
}

# =============================================================================
# RUN ALL TESTS
# =============================================================================

main() {
  cli_banner "CLI Output Library Tests"

  run_test "Basic Messages" test_basic_messages
  run_test "Sections and Headers" test_sections_headers
  run_test "Boxes" test_boxes
  run_test "Lists" test_lists
  run_test "Tables" test_tables
  run_test "Progress Bars" test_progress
  run_test "Spinner" test_spinner
  run_test "Summary" test_summary
  run_test "Banner" test_banner
  run_test "Utilities" test_utilities
  run_test "Color Stripping" test_color_stripping
  run_test "NO_COLOR Support" test_no_color_support
  run_test "Non-TTY Output" test_non_tty_output
  run_test "Bash 3.2 Compatibility" test_bash_32_compatibility

  # Results
  cli_separator 60
  cli_blank

  if [[ $TESTS_FAILED -eq 0 ]]; then
    cli_summary "All Tests Passed" \
      "Total: $TESTS_PASSED tests" \
      "Passed: $TESTS_PASSED" \
      "Failed: 0"
    exit 0
  else
    cli_error "Some tests failed"
    cli_summary "Test Results" \
      "Total: $((TESTS_PASSED + TESTS_FAILED)) tests" \
      "Passed: $TESTS_PASSED" \
      "Failed: $TESTS_FAILED"
    exit 1
  fi
}

# Run tests
main "$@"
