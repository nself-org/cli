#!/usr/bin/env bash
# dev.sh - Developer Tools
# Consolidated command including: frontend, ci, docs, whitelabel subcommands

set -euo pipefail

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Source utilities (save SCRIPT_DIR before it gets redefined by sourced files)
CLI_DIR="$SCRIPT_DIR"
source "$CLI_DIR/../lib/utils/cli-output.sh"
source "$CLI_DIR/../lib/utils/env.sh"
source "$CLI_DIR/../lib/utils/header.sh"
source "$CLI_DIR/../lib/hooks/pre-command.sh"
source "$CLI_DIR/../lib/hooks/post-command.sh"

# Source dev libraries
source "$CLI_DIR/../lib/dev/sdk-generator.sh" 2>/dev/null || true
source "$CLI_DIR/../lib/dev/docs-generator.sh" 2>/dev/null || true
source "$CLI_DIR/../lib/dev/test-helpers.sh" 2>/dev/null || true

# =============================================================================
# HELP TEXT
# =============================================================================

show_help() {
  cat <<'EOF'
nself dev - Developer Tools

USAGE:
  nself dev <subcommand> [options]

SUBCOMMANDS:
  mode [on|off]                  Enable/disable dev mode
  frontend <action>              Manage frontend applications
  ci <action>                    CI/CD configuration
  docs <action>                  Documentation generation
  whitelabel <action>            White-label customization
  sdk generate <language>        Generate SDK from GraphQL
  test <action>                  Testing tools

FRONTEND COMMANDS:
  add <name> [options]           Add frontend application
  remove <name>                  Remove frontend application
  list                           List frontend apps
  status                         Show frontend status
  deploy <name>                  Deploy frontend
  logs <name>                    View frontend logs
  env <name>                     Show environment variables

FRONTEND OPTIONS:
  --port N                       Frontend port (default: 3000)
  --route PATH                   Route prefix (default: app name)

CI COMMANDS:
  init [provider]                Initialize CI/CD (github, gitlab)
  status                         Show CI/CD status
  validate                       Validate CI/CD configuration

DOCS COMMANDS:
  generate [output]              Generate API documentation
  openapi [output]               Generate OpenAPI spec
  [section]                      Open specific docs section

WHITELABEL COMMANDS:
  branding <action>              Manage brand customization
  domain <action>                Configure custom domains
  email <action>                 Customize email templates
  theme <action>                 Create and manage themes
  logo <action>                  Upload and manage logos

SDK COMMANDS:
  generate <language> [output]   Generate SDK (typescript, python)

TEST COMMANDS:
  init [dir]                     Initialize test environment
  fixtures <entity> [count]      Generate test fixtures
  factory <entity> [output]      Generate mock data factory
  snapshot create <name>         Create database snapshot
  snapshot restore <name>        Restore database snapshot
  run [dir]                      Run integration tests

EXAMPLES:
  # Development mode
  nself dev mode on

  # Frontend management
  nself dev frontend add webapp --port 3000
  nself dev frontend status
  nself dev frontend deploy webapp --env prod

  # CI/CD
  nself dev ci init github
  nself dev ci validate

  # Documentation
  nself dev docs generate
  nself dev docs quick-start

  # SDK generation
  nself dev sdk generate typescript
  nself dev sdk generate python ./my-sdk

  # Testing
  nself dev test init
  nself dev test fixtures users 50
  nself dev test snapshot create baseline

For more information: https://docs.nself.org/developer-tools
EOF
}

# =============================================================================
# MODE SUBCOMMAND
# =============================================================================

cmd_mode() {
  local action="${1:-status}"

  case "$action" in
    on | enable)
      cli_info "Enabling development mode..."
      printf "DEV_MODE=true\n" >>.env.local
      cli_success "Development mode enabled"
      ;;
    off | disable)
      cli_info "Disabling development mode..."
      if [[ -f .env.local ]]; then
        grep -v "DEV_MODE=" .env.local >.env.local.tmp || true
        mv .env.local.tmp .env.local
      fi
      cli_success "Development mode disabled"
      ;;
    status)
      load_env_with_priority
      if [[ "${DEV_MODE:-false}" == "true" ]]; then
        cli_success "Development mode is enabled"
      else
        cli_info "Development mode is disabled"
      fi
      ;;
    *)
      cli_error "Unknown mode action: $action"
      printf "Usage: nself dev mode [on|off|status]\n"
      return 1
      ;;
  esac
}

# =============================================================================
# FRONTEND SUBCOMMAND (from frontend.sh)
# =============================================================================

# Include frontend.sh functionality
source "$CLI_DIR/frontend.sh" 2>/dev/null || true

cmd_frontend() {
  # Delegate to frontend command
  if declare -f cmd_frontend >/dev/null 2>&1; then
    command cmd_frontend "$@"
  else
    cli_error "Frontend functionality not available"
    return 1
  fi
}

# =============================================================================
# CI SUBCOMMAND (from ci.sh)
# =============================================================================

cmd_ci() {
  local action="${1:-help}"

  case "$action" in
    init)
      shift
      local provider="${1:-github}"
      cli_section "Initializing CI/CD for $provider"
      printf "\n"

      mkdir -p .github/workflows

      case "$provider" in
        github | github-actions)
          # Generate GitHub Actions workflow
          cat >.github/workflows/nself-ci.yml <<'EOF'
name: nself CI/CD

on:
  push:
    branches: [main, develop]
  pull_request:
    branches: [main]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version: '20'
      - run: npm ci
      - run: npm test -- --passWithNoTests
EOF
          cli_success "Created .github/workflows/nself-ci.yml"
          ;;
        gitlab)
          cat >.gitlab-ci.yml <<'EOF'
stages:
  - build
  - test

build:
  stage: build
  image: node:20
  script:
    - npm ci
    - npm test -- --passWithNoTests
EOF
          cli_success "Created .gitlab-ci.yml"
          ;;
        *)
          cli_error "Unknown CI provider: $provider"
          cli_info "Supported providers: github, gitlab"
          return 1
          ;;
      esac

      printf "\n"
      cli_info "Next steps:"
      cli_list_numbered 1 "Configure secrets in your CI provider"
      cli_list_numbered 2 "Commit and push the workflow files"
      ;;

    status)
      cli_section "CI/CD Configuration Status"
      printf "\n"

      if [[ -d ".github/workflows" ]]; then
        cli_success "GitHub Actions: Configured"
        for workflow in .github/workflows/*.yml; do
          [[ -f "$workflow" ]] && cli_list_item "$(basename "$workflow")"
        done
      else
        cli_info "GitHub Actions: Not configured"
      fi

      if [[ -f ".gitlab-ci.yml" ]]; then
        cli_success "GitLab CI: Configured"
      else
        cli_info "GitLab CI: Not configured"
      fi
      ;;

    validate)
      cli_section "Validating CI/CD configuration"
      printf "\n"

      local errors=0

      if [[ -d ".github/workflows" ]]; then
        for workflow in .github/workflows/*.yml; do
          if [[ -f "$workflow" ]]; then
            if grep -q "^name:" "$workflow" && grep -q "^on:" "$workflow" && grep -q "^jobs:" "$workflow"; then
              cli_success "$(basename "$workflow"): Valid structure"
            else
              cli_error "$(basename "$workflow"): Invalid structure"
              errors=$((errors + 1))
            fi
          fi
        done
      fi

      printf "\n"
      if [[ $errors -eq 0 ]]; then
        cli_success "Validation passed"
      else
        cli_error "Validation failed with $errors error(s)"
        return 1
      fi
      ;;

    help | -h | --help)
      cli_section "CI/CD Commands"
      printf "\n"
      printf "Usage: nself dev ci <action>\n\n"
      printf "Actions:\n"
      cli_list_item "init [provider]  - Initialize CI/CD (github, gitlab)"
      cli_list_item "status          - Show CI/CD status"
      cli_list_item "validate        - Validate configuration"
      ;;

    *)
      cli_error "Unknown CI action: $action"
      printf "Run 'nself dev ci help' for usage\n"
      return 1
      ;;
  esac
}

# =============================================================================
# DOCS SUBCOMMAND (from docs.sh)
# =============================================================================

cmd_docs() {
  local section="${1:-}"

  # Documentation URLs
  local WIKI_URL="https://github.com/nself-org/cli/wiki"
  local REPO_URL="https://github.com/nself-org/cli"

  local url="$WIKI_URL"

  case "$section" in
    generate)
      cli_section "Generating API documentation"
      printf "\n"
      cli_info "Documentation generation not yet implemented"
      cli_info "Visit: $WIKI_URL"
      return 0
      ;;
    openapi)
      cli_section "Generating OpenAPI specification"
      printf "\n"
      cli_info "OpenAPI generation not yet implemented"
      return 0
      ;;
    quick-start)
      url="${WIKI_URL}/Quick-Start"
      ;;
    configuration)
      url="${WIKI_URL}/Configuration"
      ;;
    deployment)
      url="${WIKI_URL}/Deployment"
      ;;
    troubleshooting)
      url="${WIKI_URL}/Troubleshooting"
      ;;
    "")
      url="$WIKI_URL"
      ;;
  esac

  # Open URL in browser
  if command -v open >/dev/null 2>&1; then
    open "$url"
  elif command -v xdg-open >/dev/null 2>&1; then
    xdg-open "$url"
  else
    cli_info "Open this URL in your browser:"
    printf "  %s\n" "$url"
  fi

  cli_success "Opening documentation in browser..."
}

# =============================================================================
# WHITELABEL SUBCOMMAND (from whitelabel.sh)
# =============================================================================

cmd_whitelabel() {
  cli_info "White-label features are now under 'nself tenant'"
  printf "\n"
  printf "Available commands:\n"
  cli_list_item "nself tenant branding  - Brand customization"
  cli_list_item "nself tenant domains   - Custom domains"
  cli_list_item "nself tenant email     - Email templates"
  cli_list_item "nself tenant themes    - UI themes"
  printf "\n"
  cli_info "Run 'nself tenant --help' for more information"
}

# =============================================================================
# SDK SUBCOMMAND
# =============================================================================

cmd_sdk() {
  local action="${1:-help}"

  case "$action" in
    generate)
      shift
      local language="${1:-}"
      local output="${2:-./sdk/$language}"

      if [[ -z "$language" ]]; then
        cli_error "Language parameter required"
        printf "Usage: nself dev sdk generate <language> [output]\n"
        printf "Languages: typescript, python\n"
        return 1
      fi

      cli_section "Generating $language SDK"
      printf "\n"

      if declare -f generate_sdk >/dev/null 2>&1; then
        generate_sdk "$language" "$output"
      else
        cli_info "SDK generation not yet implemented"
        cli_info "Placeholder SDK will be created at: $output"
        mkdir -p "$output"
        printf "# %s SDK\n\nGenerated by nself dev sdk\n" "$language" >"$output/README.md"
        cli_success "SDK stub created at $output"
      fi
      ;;

    help | -h | --help)
      cli_section "SDK Commands"
      printf "\n"
      printf "Usage: nself dev sdk generate <language> [output]\n\n"
      printf "Languages:\n"
      cli_list_item "typescript"
      cli_list_item "python"
      ;;

    *)
      cli_error "Unknown SDK action: $action"
      printf "Run 'nself dev sdk help' for usage\n"
      return 1
      ;;
  esac
}

# =============================================================================
# TEST SUBCOMMAND
# =============================================================================

cmd_test() {
  local action="${1:-help}"

  case "$action" in
    init)
      shift
      local test_dir="${1:-.nself/test}"
      cli_section "Initializing test environment"
      printf "\n"

      mkdir -p "$test_dir"
      mkdir -p "$test_dir/fixtures"
      mkdir -p "$test_dir/factories"
      mkdir -p "$test_dir/integration"

      cli_success "Test environment initialized at $test_dir"
      ;;

    fixtures)
      shift
      local entity="${1:-}"
      local count="${2:-10}"

      if [[ -z "$entity" ]]; then
        cli_error "Entity parameter required"
        printf "Usage: nself dev test fixtures <entity> [count]\n"
        return 1
      fi

      cli_section "Generating $count $entity fixtures"
      printf "\n"

      if declare -f generate_fixtures >/dev/null 2>&1; then
        generate_fixtures "$entity" "$count" ".nself/test/fixtures/${entity}.json"
      else
        cli_info "Fixture generation not yet implemented"
      fi
      ;;

    snapshot)
      shift
      local snapshot_action="${1:-}"
      local name="${2:-test-snapshot}"

      case "$snapshot_action" in
        create)
          cli_section "Creating snapshot: $name"
          printf "\n"
          # Create database backup
          if command -v nself >/dev/null 2>&1; then
            nself backup create database "snapshot-$name"
            cli_success "Snapshot created: $name"
          else
            cli_error "nself command not found"
            return 1
          fi
          ;;
        restore)
          cli_section "Restoring snapshot: $name"
          printf "\n"
          if command -v nself >/dev/null 2>&1; then
            nself backup restore "snapshot-$name"
            cli_success "Snapshot restored: $name"
          else
            cli_error "nself command not found"
            return 1
          fi
          ;;
        *)
          cli_error "Unknown snapshot action: $snapshot_action"
          printf "Usage: nself dev test snapshot <create|restore> <name>\n"
          return 1
          ;;
      esac
      ;;

    run)
      shift
      local test_dir="${1:-.nself/test/integration}"
      cli_section "Running integration tests"
      printf "\n"

      if [[ -d "$test_dir" ]]; then
        cli_info "Test directory: $test_dir"
        cli_info "Integration test runner not yet implemented"
      else
        cli_error "Test directory not found: $test_dir"
        cli_info "Run 'nself dev test init' first"
        return 1
      fi
      ;;

    help | -h | --help)
      cli_section "Test Commands"
      printf "\n"
      printf "Usage: nself dev test <action>\n\n"
      printf "Actions:\n"
      cli_list_item "init [dir]                  - Initialize test environment"
      cli_list_item "fixtures <entity> [count]   - Generate test fixtures"
      cli_list_item "snapshot create <name>      - Create database snapshot"
      cli_list_item "snapshot restore <name>     - Restore database snapshot"
      cli_list_item "run [dir]                   - Run integration tests"
      ;;

    *)
      cli_error "Unknown test action: $action"
      printf "Run 'nself dev test help' for usage\n"
      return 1
      ;;
  esac
}

# =============================================================================
# MAIN COMMAND ROUTER
# =============================================================================

main() {
  local subcommand="${1:-help}"

  # Check for help
  if [[ "$subcommand" == "-h" ]] || [[ "$subcommand" == "--help" ]] || [[ "$subcommand" == "help" ]]; then
    show_help
    return 0
  fi

  shift || true

  # Route to subcommand
  case "$subcommand" in
    mode)
      cmd_mode "$@"
      ;;
    frontend)
      cmd_frontend "$@"
      ;;
    ci)
      cmd_ci "$@"
      ;;
    docs)
      cmd_docs "$@"
      ;;
    whitelabel)
      cmd_whitelabel "$@"
      ;;
    sdk)
      cmd_sdk "$@"
      ;;
    test)
      cmd_test "$@"
      ;;
    help | -h | --help)
      show_help
      ;;
    *)
      cli_error "Unknown subcommand: $subcommand"
      printf "\n"
      show_help
      return 1
      ;;
  esac
}

# Execute if run directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  # Help is read-only - bypass init/env guards
  for _arg in "$@"; do
    if [[ "$_arg" == "--help" ]] || [[ "$_arg" == "-h" ]]; then
      show_help
      exit 0
    fi
  done
  pre_command "dev" || exit $?
  main "$@"
  exit_code=$?
  post_command "dev" $exit_code
  exit $exit_code
fi
