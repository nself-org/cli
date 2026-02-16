#!/usr/bin/env bash

# config.sh - Configuration constants for nself init command
#
# This module defines all configuration constants used by the init command.
# It centralizes all configuration to make maintenance easier.

# Guard against multiple sourcing
if [[ -n "${INIT_CONFIG_SOURCED:-}" ]]; then

set -euo pipefail

  return 0
fi
INIT_CONFIG_SOURCED=1

# ========================================================================
# ERROR CODES
# ========================================================================
# Only set if not already set (for test compatibility)
: ${INIT_E_SUCCESS:=0}
: ${INIT_E_GENERAL:=1}
: ${INIT_E_MISUSE:=2}
: ${INIT_E_CANTCREAT:=73}
: ${INIT_E_IOERR:=74}
: ${INIT_E_TEMPFAIL:=75}
: ${INIT_E_NOPERM:=77}
: ${INIT_E_CONFIG:=78}

readonly INIT_E_SUCCESS INIT_E_GENERAL INIT_E_MISUSE INIT_E_CANTCREAT
readonly INIT_E_IOERR INIT_E_TEMPFAIL INIT_E_NOPERM INIT_E_CONFIG

# ========================================================================
# FILE PERMISSIONS
# ========================================================================
: ${INIT_PERM_PUBLIC:=644}
: ${INIT_PERM_PRIVATE:=600}
: ${INIT_PERM_EXEC:=755}

readonly INIT_PERM_PUBLIC INIT_PERM_PRIVATE INIT_PERM_EXEC

# ========================================================================
# GITIGNORE ENTRIES
# ========================================================================
# Required entries that must be in .gitignore for security
readonly -a INIT_GITIGNORE_REQUIRED=(
  ".env"
  ".env.local"
  ".env.*.local"
  ".env.secrets"
  "_backup*"
  ".volumes/"
  "logs/"
  "*.log"
  "node_modules/"
  ".DS_Store"
)

# ========================================================================
# TEMPLATE FILES
# ========================================================================
# Basic template files (always copied)
readonly -a INIT_TEMPLATES_BASIC=(
  "envs/.env.example"
  "envs/.env"
)

# Full setup template files (with --full flag)
readonly -a INIT_TEMPLATES_FULL=(
  "envs/.env.dev"
  "envs/.env.staging"
  "envs/.env.prod"
  "envs/.env.secrets"
  "schema.dbml"
)

# ========================================================================
# SEARCH PATHS
# ========================================================================
# Paths to search for template directory (in order of preference)
readonly -a INIT_TEMPLATE_SEARCH_PATHS=(
  "../templates"                     # Relative to cli directory
  "../../templates"                  # Development/source location (from cli)
  "/usr/share/nself/src/templates"   # System installation
  "$HOME/.nself/src/templates"       # Local user installation
  "$HOME/.local/nself/src/templates" # Custom installation path
)

# ========================================================================
# REQUIRED COMMANDS
# ========================================================================
# Commands that must be available for init to work
readonly -a INIT_REQUIRED_COMMANDS=(
  "git"
  "cat"
  "cp"
  "mv"
  "rm"
  "grep"
  "sed"
)

# ========================================================================
# DISPLAY SETTINGS
# ========================================================================
# Terminal capability thresholds
readonly INIT_MIN_TERM_WIDTH=40
readonly INIT_PREFERRED_TERM_WIDTH=80

# ========================================================================
# STATE TRACKING
# ========================================================================
# Valid init states
readonly INIT_STATE_IDLE="idle"
readonly INIT_STATE_IN_PROGRESS="in_progress"
readonly INIT_STATE_COMPLETED="completed"
readonly INIT_STATE_FAILED="failed"
readonly INIT_STATE_ROLLED_BACK="rolled_back"

# Export all constants for use in other modules
export INIT_E_SUCCESS INIT_E_GENERAL INIT_E_MISUSE INIT_E_CANTCREAT
export INIT_E_IOERR INIT_E_TEMPFAIL INIT_E_NOPERM INIT_E_CONFIG
export INIT_PERM_PUBLIC INIT_PERM_PRIVATE INIT_PERM_EXEC
export INIT_GITIGNORE_REQUIRED INIT_TEMPLATES_BASIC INIT_TEMPLATES_FULL
export INIT_TEMPLATE_SEARCH_PATHS INIT_REQUIRED_COMMANDS
export INIT_MIN_TERM_WIDTH INIT_PREFERRED_TERM_WIDTH
export INIT_STATE_IDLE INIT_STATE_IN_PROGRESS INIT_STATE_COMPLETED
export INIT_STATE_FAILED INIT_STATE_ROLLED_BACK
