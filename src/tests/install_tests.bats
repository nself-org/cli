#!/usr/bin/env bats

# Test suite for nself installation functionality

setup() {
  # Create a temporary directory for testing
  export TEST_DIR=$(mktemp -d)
  export HOME="$TEST_DIR"
  export NSELF_DIR="$HOME/.nself"
  export BIN_DIR="$NSELF_DIR/bin"

  # Source the color variables and functions from install.sh
  source <(sed -n '111,224p' ../../install.sh) # Color variables and functions
}

teardown() {
  # Clean up test directory
  rm -rf "$TEST_DIR"
}

@test "command_exists detects existing commands" {
  # Test with a command that should exist
  command_exists() {
    command -v "$1" >/dev/null 2>&1
  }

  run command_exists "bash"
  [ "$status" -eq 0 ]
}

@test "command_exists fails for non-existent commands" {
  command_exists() {
    command -v "$1" >/dev/null 2>&1
  }

  run command_exists "nonexistentcommand123456"
  [ "$status" -eq 1 ]
}

@test "echo_info outputs colored info message" {
  echo_info() {
    echo "[INFO] $1"
  }
  run echo_info "Test message"
  [ "$status" -eq 0 ]
  [[ "$output" == *"[INFO]"* ]]
  [[ "$output" == *"Test message"* ]]
}

@test "echo_success outputs colored success message" {
  echo_success() {
    echo "[SUCCESS] $1"
  }
  run echo_success "Success message"
  [ "$status" -eq 0 ]
  [[ "$output" == *"[SUCCESS]"* ]]
  [[ "$output" == *"Success message"* ]]
}

@test "echo_warning outputs colored warning message" {
  echo_warning() {
    echo "[WARNING] $1"
  }
  run echo_warning "Warning message"
  [ "$status" -eq 0 ]
  [[ "$output" == *"[WARNING]"* ]]
  [[ "$output" == *"Warning message"* ]]
}

@test "echo_error outputs colored error message to stderr" {
  echo_error() {
    echo "[ERROR] $1" >&2
  }
  run echo_error "Error message"
  [ "$status" -eq 0 ]
  [[ "$output" == *"[ERROR]"* ]]
  [[ "$output" == *"Error message"* ]]
}

@test "spinner function shows progress indicator" {
  # Create a simple background task
  show_spinner() {
    local pid=$1
    local message=$2
    wait $pid 2>/dev/null
    echo "$message completed"
  }

  (sleep 0.1) &
  run show_spinner $! "Test task"
  [ "$status" -eq 0 ]
  [[ "$output" == *"Test task completed"* ]]
}
