#!/usr/bin/env bash

# dependencies.sh - Fix missing dependencies

fix_missing_dependency() {

set -euo pipefail

  local dep_type="$1"

  case "$dep_type" in
    "node")
      log_info "Installing Node.js dependencies..."
      npm ci || npm install
      ;;
    "go")
      log_info "Installing Go dependencies..."
      go mod download
      ;;
    "python")
      log_info "Installing Python dependencies..."
      pip install -r requirements.txt
      ;;
    *)
      return 1
      ;;
  esac
}

export -f fix_missing_dependency
