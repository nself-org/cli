#!/usr/bin/env bash

# help.sh - Help and documentation functions for nself init command
#
# This module provides all help-related functionality for the init command.
# It is designed to be sourced by the main init.sh script.

# Show help for init command
# Inputs: None
# Outputs: Help text to stdout
# Returns: 0
show_init_help() {

set -euo pipefail

  # Ensure display variables are set for help text
  local BULLET="${BULLET:--}"
  local ARROW="${ARROW:-->}"
  local LIGHTNING="${LIGHTNING:-*}"

  echo "nself init - Initialize a new full-stack application"
  echo ""
  echo "Usage: nself init [OPTIONS]"
  echo ""
  echo "Description:"
  echo "  Creates a new nself project with .env configuration file"
  echo "  and .env.example reference documentation. Sets up the foundation"
  echo "  for a full-stack application with smart defaults."
  echo ""
  echo "Options:"
  echo "  --full              Create all environment files and starter schema"
  echo "  --wizard            Launch interactive setup wizard"
  echo "  --demo              Create a complete demo app with all services"
  echo "  --admin             Setup minimal admin UI only"
  echo "  --force             Reinitialize even if .env exists"
  echo "  --quiet, -q         Minimal output (for automation)"
  echo "  -h, --help          Show this help message"
  echo ""
  echo "Examples:"
  echo "  mkdir myproject && cd myproject"
  echo "  nself init                     # Basic setup (.env + .env.example)"
  echo "  nself init --full              # Complete setup with all env files"
  echo "  nself init --wizard            # Interactive setup wizard"
  echo "  nself init --demo              # Full demo with all services & apps"
  echo ""
  echo "Files Created (Basic):"
  echo "  ${BULLET} .env                         # Your personal dev configuration"
  echo "  ${BULLET} .env.example                 # Complete reference docs"
  echo "  ${BULLET} .gitignore                   # Security rules"
  echo ""
  echo "Files Created (--full):"
  echo "  ${BULLET} All basic files plus:"
  echo "  ${BULLET} .env.dev                     # Team-shared dev defaults"
  echo "  ${BULLET} .env.staging                 # Staging environment config"
  echo "  ${BULLET} .env.prod                    # Production config (public)"
  echo "  ${BULLET} .env.secrets                 # Sensitive data (git-ignored)"
  echo "  ${BULLET} schema.dbml                  # Example database schema"
  echo ""
  echo "Next Steps:"
  echo "  1. Edit .env (optional - defaults work!)"
  echo "  2. nself build                 # Generate infrastructure"
  echo "  3. nself start                 # Start services"
  echo ""
  echo "Notes:"
  echo "  ${BULLET} Safe to run multiple times"
  echo "  ${BULLET} Won't overwrite existing configuration"
  echo "  ${BULLET} Works with smart defaults out of the box"
}

# Export functions for use in other scripts
export -f show_init_help
