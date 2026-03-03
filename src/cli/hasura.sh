#!/usr/bin/env bash
# hasura.sh - Hasura GraphQL management commands
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../lib/utils/display.sh"
source "$SCRIPT_DIR/../lib/utils/cli-output.sh"
source "$SCRIPT_DIR/../lib/utils/env.sh"

cmd_hasura() {
  local subcommand="${1:-help}"
  shift || true

  case "$subcommand" in
    metadata) hasura_metadata "$@" ;;
    console) hasura_console "$@" ;;
    help|--help|-h) hasura_usage ;;
    *) cli_error "Unknown subcommand: $subcommand" && hasura_usage && exit 1 ;;
  esac
}

hasura_metadata() {
  local action="${1:-help}"
  shift || true

  case "$action" in
    apply) metadata_apply "$@" ;;
    export) metadata_export "$@" ;;
    reload) metadata_reload "$@" ;;
    *) cli_error "Unknown action: $action" && exit 1 ;;
  esac
}

# Resolve the Hasura project directory.
# Checks HASURA_PROJECT_DIR env var first, then looks for hasura/ in CWD.
# Returns the path to use with --project, or empty string if not found.
get_hasura_project_dir() {
  if [[ -n "${HASURA_PROJECT_DIR:-}" ]]; then
    echo "$HASURA_PROJECT_DIR"
    return
  fi
  if [[ -d "hasura" ]]; then
    echo "hasura"
    return
  fi
  echo ""
}

# Ensure hasura/config.yaml exists so the Hasura CLI can find its project.
# The Hasura CLI v2 requires config.yaml even when --endpoint and --admin-secret
# are passed as flags — it uses config.yaml to locate metadata_directory.
# We generate the file without the admin_secret (passed as a CLI flag instead)
# so no secret is written to disk.
ensure_hasura_config() {
  local project_dir="${1:-hasura}"
  local hasura_url="${2:-http://localhost:8080}"

  # Only generate if the directory exists but config.yaml is missing
  if [[ ! -d "$project_dir" ]]; then
    return 0
  fi

  if [[ -f "$project_dir/config.yaml" ]]; then
    return 0
  fi

  cli_info "Generating ${project_dir}/config.yaml from environment..."
  # admin_secret is intentionally omitted — pass it as --admin-secret CLI flag
  # so the secret never lives in a file on disk.
  printf 'version: 3\nendpoint: %s\nmetadata_directory: metadata\nactions:\n  kind: synchronous\n  handler_webhook_baseurl: http://localhost:3000\n' \
    "$hasura_url" > "$project_dir/config.yaml"
  cli_success "Generated ${project_dir}/config.yaml (admin_secret passed as CLI flag)"
}

metadata_apply() {
  cli_header "nself db hasura metadata apply"
  load_env_with_priority true

  local hasura_url="${HASURA_GRAPHQL_ENDPOINT:-http://localhost:${HASURA_PORT:-8080}}"
  local admin_secret="${HASURA_GRAPHQL_ADMIN_SECRET:-}"

  [[ -z "$admin_secret" ]] && cli_error "HASURA_GRAPHQL_ADMIN_SECRET not set. Source .env or set the variable." && exit 1

  if ! command -v hasura >/dev/null 2>&1; then
    cli_warning "Hasura CLI not found"
    cli_info "Install: npm install -g hasura-cli"
    exit 1
  fi

  local project_dir
  project_dir=$(get_hasura_project_dir)

  if [[ -n "$project_dir" ]]; then
    ensure_hasura_config "$project_dir" "$hasura_url"

    # Guard: if metadata directory is empty, warn and exit 0 rather than letting
    # the Hasura CLI emit a cryptic parse-failed error.
    local metadata_dir="$project_dir/metadata"
    local metadata_has_content=false
    if [[ -d "$metadata_dir" ]]; then
      for _f in "$metadata_dir"/*.yaml "$metadata_dir"/*.json; do
        if [[ -f "$_f" ]]; then
          metadata_has_content=true
          break
        fi
      done
    fi

    if [[ "$metadata_has_content" == "false" ]]; then
      cli_warning "${metadata_dir}/ is empty — no metadata to apply."
      printf "  To populate it, either:\n"
      printf "  1. nself db hasura metadata export  (exports current Hasura state)\n"
      printf "  2. Track tables via Hasura Console:  nself db hasura console\n"
      exit 0
    fi

    hasura metadata apply \
      --project "$project_dir" \
      --endpoint "$hasura_url" \
      --admin-secret "$admin_secret"
  else
    # No hasura/ directory — run without --project (user must be inside a hasura project)
    hasura metadata apply --endpoint "$hasura_url" --admin-secret "$admin_secret"
  fi
}

metadata_export() {
  cli_header "nself db hasura metadata export"
  load_env_with_priority true

  local hasura_url="${HASURA_GRAPHQL_ENDPOINT:-http://localhost:${HASURA_PORT:-8080}}"
  local admin_secret="${HASURA_GRAPHQL_ADMIN_SECRET:-}"

  [[ -z "$admin_secret" ]] && cli_error "HASURA_GRAPHQL_ADMIN_SECRET not set. Source .env or set the variable." && exit 1

  if ! command -v hasura >/dev/null 2>&1; then
    cli_warning "Hasura CLI not found"
    cli_info "Install: npm install -g hasura-cli"
    exit 1
  fi

  local project_dir
  project_dir=$(get_hasura_project_dir)

  if [[ -n "$project_dir" ]]; then
    ensure_hasura_config "$project_dir" "$hasura_url"
    hasura metadata export \
      --project "$project_dir" \
      --endpoint "$hasura_url" \
      --admin-secret "$admin_secret"
  else
    hasura metadata export --endpoint "$hasura_url" --admin-secret "$admin_secret"
  fi
}

metadata_reload() {
  cli_header "nself db hasura metadata reload"
  load_env_with_priority true

  local hasura_url="${HASURA_GRAPHQL_ENDPOINT:-http://localhost:${HASURA_PORT:-8080}}"
  local admin_secret="${HASURA_GRAPHQL_ADMIN_SECRET:-}"

  [[ -z "$admin_secret" ]] && cli_error "HASURA_GRAPHQL_ADMIN_SECRET not set. Source .env or set the variable." && exit 1

  curl -s -X POST \
    -H "Content-Type: application/json" \
    -H "X-Hasura-Admin-Secret: $admin_secret" \
    -d '{"type": "reload_metadata", "args": {}}' \
    "$hasura_url/v1/metadata" >/dev/null && cli_success "Metadata reloaded"
}

hasura_console() {
  cli_header "nself db hasura console"
  load_env_with_priority true

  local hasura_url="${HASURA_GRAPHQL_ENDPOINT:-http://localhost:${HASURA_PORT:-8080}}"
  local admin_secret="${HASURA_GRAPHQL_ADMIN_SECRET:-}"

  if ! command -v hasura >/dev/null 2>&1; then
    cli_info "Open: $hasura_url/console"
    return 0
  fi

  local project_dir
  project_dir=$(get_hasura_project_dir)

  if [[ -n "$project_dir" ]]; then
    ensure_hasura_config "$project_dir" "$hasura_url"
    hasura console \
      --project "$project_dir" \
      --endpoint "$hasura_url" \
      --admin-secret "$admin_secret"
  else
    hasura console --endpoint "$hasura_url" --admin-secret "$admin_secret"
  fi
}

hasura_usage() {
  printf "\nUsage: nself db hasura <subcommand>\n\n"
  printf "SUBCOMMANDS:\n"
  printf "  console            Open Hasura Console\n"
  printf "  metadata apply     Apply metadata\n"
  printf "  metadata export    Export metadata\n"
  printf "  metadata reload    Reload metadata cache\n\n"
  printf "NOTE: 'nself hasura' is deprecated. Use 'nself db hasura' instead.\n\n"
}

export -f cmd_hasura

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  printf "\033[0;33m⚠\033[0m  WARNING: 'nself hasura' is deprecated. Use 'nself db hasura' instead.\n" >&2
  printf "   This compatibility wrapper will be removed in v1.0.0\n\n" >&2
  cmd_hasura "$@"
fi
