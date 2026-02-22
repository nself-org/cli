#!/usr/bin/env bash
# plugin.sh - Plugin management for nself v0.4.8
# Install, manage, and use nself plugins

set -o pipefail

set -euo pipefail


# ============================================================================
# INITIALIZATION
# ============================================================================

CLI_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source utilities
source "$CLI_SCRIPT_DIR/../lib/utils/env.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/utils/display.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/utils/header.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/utils/platform-compat.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/hooks/pre-command.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/hooks/post-command.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/plugin/core.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/plugin/registry.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/plugin/dependencies.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/plugin/runtime.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/plugin/licensing.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/utils/cli-output.sh" 2>/dev/null || true

# Fallbacks if display.sh didn't load
if ! declare -f log_success >/dev/null 2>&1; then
  log_success() { printf "\033[0;32m[SUCCESS]\033[0m %s\n" "$1"; }
fi
if ! declare -f log_warning >/dev/null 2>&1; then
  log_warning() { printf "\033[0;33m[WARNING]\033[0m %s\n" "$1"; }
fi
if ! declare -f log_error >/dev/null 2>&1; then
  log_error() { printf "\033[0;31m[ERROR]\033[0m %s\n" "$1" >&2; }
fi
if ! declare -f log_info >/dev/null 2>&1; then
  log_info() { printf "\033[0;34m[INFO]\033[0m %s\n" "$1"; }
fi

# ============================================================================
# CONSTANTS
# ============================================================================

PLUGIN_DIR="${NSELF_PLUGIN_DIR:-$HOME/.nself/plugins}"
PLUGIN_CACHE_DIR="${NSELF_PLUGIN_CACHE:-$HOME/.nself/cache/plugins}"
PLUGIN_REGISTRY_URL="${NSELF_PLUGIN_REGISTRY:-https://plugins.nself.org}"
PLUGIN_REGISTRY_FALLBACK="https://raw.githubusercontent.com/nself-org/plugins/main/registry.json"
PLUGIN_REPO_URL="https://github.com/nself-org/plugins"
NSELF_API_DOWNLOAD_URL="${NSELF_API_URL:-https://api.nself.org}/plugins"

# ============================================================================
# PLUGIN MANAGEMENT
# ============================================================================

# List available plugins
cmd_list() {
  local show_installed_only=false
  local filter_category=""
  local show_detailed=false

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --installed | -i)
        show_installed_only=true
        shift
        ;;
      --detailed | -d)
        show_detailed=true
        shift
        ;;
      --category | -c)
        filter_category="$2"
        shift 2
        ;;
      *)
        shift
        ;;
    esac
  done

  # If showing installed with detailed flag, use new list_all_plugins
  if [[ "$show_installed_only" == "true" ]] && [[ "$show_detailed" == "true" ]]; then
    list_all_plugins
    return 0
  fi

  # Fetch free registry
  local registry
  registry=$(fetch_registry)

  if [[ -z "$registry" ]]; then
    log_error "Failed to fetch plugin registry"
    return 1
  fi

  local count=0

  # ── Installed-only view ────────────────────────────────────────────────────
  if [[ "$show_installed_only" == "true" ]]; then
    printf "\n=== Installed Plugins ===\n\n"
    printf "%-20s %-10s %-12s %-6s %-30s\n" "NAME" "VERSION" "CATEGORY" "TIER" "DESCRIPTION"
    printf "%-20s %-10s %-12s %-6s %-30s\n" "----" "-------" "--------" "----" "-----------"

    for plugin_dir in "$PLUGIN_DIR"/*/; do
      [[ -d "$plugin_dir" ]] || continue
      local plugin
      plugin=$(basename "$plugin_dir")
      [[ "$plugin" == "_shared" ]] && continue
      [[ -f "$plugin_dir/plugin.json" ]] || continue

      local version description category
      if command -v jq >/dev/null 2>&1; then
        version=$(jq -r '.version // "1.0.0"' "$plugin_dir/plugin.json" 2>/dev/null)
        description=$(jq -r '.description // ""' "$plugin_dir/plugin.json" 2>/dev/null)
        category=$(jq -r '.category // "general"' "$plugin_dir/plugin.json" 2>/dev/null)
      else
        version=$(grep '"version"' "$plugin_dir/plugin.json" | head -1 | sed 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
        description=$(grep '"description"' "$plugin_dir/plugin.json" | head -1 | sed 's/.*"description"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
        category=$(grep '"category"' "$plugin_dir/plugin.json" | head -1 | sed 's/.*"category"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
      fi
      version="${version:-1.0.0}"
      category="${category:-general}"

      if [[ -n "$filter_category" ]] && [[ "$category" != "$filter_category" ]]; then
        continue
      fi

      local tier="FREE"
      if declare -f license_is_paid_plugin >/dev/null 2>&1 && license_is_paid_plugin "$plugin"; then
        tier="PRO"
      fi

      printf "%-20s %-10s %-12s %-6s %-30s\n" "$plugin" "$version" "$category" "$tier" "${description:0:30}"
      count=$((count + 1))
    done

    if [[ $count -eq 0 ]]; then
      log_info "No plugins installed"
    fi
    printf "\nDetailed status: nself plugin list --installed --detailed\n"
    return 0
  fi

  # ── Available view (free + pro) ────────────────────────────────────────────
  local free_count=0
  local pro_count=0
  if declare -f license_is_paid_plugin >/dev/null 2>&1; then
    pro_count=$(printf '%s' "$NSELF_PRO_PLUGINS" | wc -w | tr -d ' ')
  fi
  local free_plugins
  free_plugins=$(printf '%s' "$registry" | grep -o '"[a-z-]*":{' | sed 's/"//g;s/:{//')
  for _p in $free_plugins; do free_count=$((free_count + 1)); done

  printf "\n=== Available Plugins (%d free, %d pro) ===\n\n" "$free_count" "$pro_count"
  printf "%-20s %-10s %-12s %-6s %-30s\n" "NAME" "VERSION" "CATEGORY" "TIER" "DESCRIPTION"
  printf "%-20s %-10s %-12s %-6s %-30s\n" "----" "-------" "--------" "----" "-----------"

  # Free plugins from registry
  for plugin in $free_plugins; do
    local version description category
    version=$(printf '%s' "$registry" | grep -A10 "\"$plugin\"" | grep '"version"' | head -1 | sed 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
    description=$(printf '%s' "$registry" | grep -A10 "\"$plugin\"" | grep '"description"' | head -1 | sed 's/.*"description"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
    category=$(printf '%s' "$registry" | grep -A10 "\"$plugin\"" | grep '"category"' | head -1 | sed 's/.*"category"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')

    if [[ -n "$filter_category" ]] && [[ "$category" != "$filter_category" ]]; then
      continue
    fi

    local suffix=""
    if is_plugin_installed "$plugin"; then suffix=" *"; fi

    printf "%-20s %-10s %-12s %-6s %-30s%s\n" "$plugin" "${version:-1.0.0}" "${category:-general}" "FREE" "${description:0:30}" "$suffix"
    count=$((count + 1))
  done

  # Pro plugins from hardcoded list (private registry — names only)
  if declare -f license_is_paid_plugin >/dev/null 2>&1; then
    local has_license=false
    if license_get_key >/dev/null 2>&1; then has_license=true; fi

    printf "\n  --- Pro Plugins (license required — %s) ---\n\n" "${NSELF_PRICING_URL:-https://nself.org/pricing}"

    for plugin in $NSELF_PRO_PLUGINS; do
      if [[ -n "$filter_category" ]]; then
        # Without full metadata, skip category filter for pro plugins
        continue
      fi

      local suffix=""
      if is_plugin_installed "$plugin"; then suffix=" *"; fi

      if [[ "$has_license" == "true" ]]; then
        printf "%-20s %-10s %-12s %-6s %-30s%s\n" "$plugin" "1.0.0" "-" "PRO" "Pro Plugin" "$suffix"
      else
        printf "%-20s %-10s %-12s %-6s %-30s%s\n" "$plugin" "1.0.0" "-" "PRO" "License required" "$suffix"
      fi
      count=$((count + 1))
    done
  fi

  if [[ $count -eq 0 ]] && [[ -n "$filter_category" ]]; then
    log_info "No free plugins found in category: $filter_category"
  fi

  printf "\n  * = installed\n"
  printf "\nInstall free:  nself plugin install <name>\n"
  printf "Install pro:   nself plugin license set <key> && nself plugin install <name>\n"
  printf "Get a license: %s\n" "${NSELF_PRICING_URL:-https://nself.org/pricing}"
}

# Install a plugin
cmd_install() {
  local plugin_name="$1"

  if [[ -z "$plugin_name" ]]; then
    log_error "Plugin name required"
    printf "\nUsage: nself plugin install <name>\n"
    return 1
  fi

  # Check if it's a local path
  if [[ -d "$plugin_name" ]]; then
    install_local_plugin "$plugin_name"
    return $?
  fi

  # Parse version if specified (plugin@version)
  local version=""
  if [[ "$plugin_name" == *"@"* ]]; then
    version="${plugin_name#*@}"
    plugin_name="${plugin_name%@*}"
  fi

  log_info "Installing plugin: $plugin_name"

  # Check if already installed
  if is_plugin_installed "$plugin_name"; then
    log_warning "Plugin '$plugin_name' is already installed"
    printf "Use 'nself plugin update %s' to update\n" "$plugin_name"
    return 0
  fi

  # Fetch registry
  local registry
  registry=$(fetch_registry)

  # Determine if this is a pro plugin — if so, skip the free registry check
  local is_pro=false
  if declare -f license_is_paid_plugin >/dev/null 2>&1 && license_is_paid_plugin "$plugin_name"; then
    is_pro=true
  fi

  # For free plugins, verify existence in registry
  if [[ "$is_pro" == "false" ]]; then
    if ! printf '%s' "$registry" | grep -q "\"$plugin_name\":"; then
      log_error "Plugin '$plugin_name' not found in registry"
      printf "\nRun 'nself plugin list' to see all available plugins.\n"
      return 1
    fi
  fi

  # Check license entitlement before downloading
  if declare -f license_check_entitlement >/dev/null 2>&1; then
    if ! license_check_entitlement "$plugin_name"; then
      return 1
    fi
  fi

  # Download and install
  download_plugin "$plugin_name" "$version"

  # Run install script
  run_plugin_installer "$plugin_name"

  # Install plugin-to-plugin dependencies declared in plugin.json
  if declare -f plugin_install_dependencies >/dev/null 2>&1; then
    plugin_install_dependencies "$plugin_name" || true
  fi

  log_success "Plugin '$plugin_name' installed successfully!"

  # Record in project plugin list if running inside a project directory
  if [[ -f ".env" ]]; then
    mkdir -p ".nself"
    local project_list=".nself/plugins"
    if ! grep -qx "$plugin_name" "$project_list" 2>/dev/null; then
      printf '%s\n' "$plugin_name" >> "$project_list"
      log_info "Registered in project plugin list: $project_list"
    fi
  fi

  # Check for system dependencies
  if declare -f check_plugin_dependencies >/dev/null 2>&1; then
    printf "\n"
    if ! check_plugin_dependencies "$plugin_name" 2>/dev/null; then
      printf "Note: This plugin has system dependencies\n"
      printf "Run: ${CLI_CYAN}nself plugin check-deps %s${CLI_RESET}\n" "$plugin_name"
    fi
  fi

  printf "\nConfigure in .env and run: nself plugin %s sync\n" "$plugin_name"
}

# Remove a plugin
cmd_remove() {
  local plugin_name="$1"
  local delete_data="${2:-}"

  if [[ -z "$plugin_name" ]]; then
    log_error "Plugin name required"
    return 1
  fi

  if ! is_plugin_installed "$plugin_name"; then
    log_error "Plugin '$plugin_name' is not installed"
    return 1
  fi

  log_info "Removing plugin: $plugin_name"

  local plugin_dir="$PLUGIN_DIR/$plugin_name"

  # Run uninstall script
  if [[ -f "$plugin_dir/uninstall.sh" ]]; then
    local uninstall_args=""
    [[ "$delete_data" != "--delete-data" ]] && uninstall_args="--keep-data"
    bash "$plugin_dir/uninstall.sh" $uninstall_args
  fi

  # Remove plugin directory
  rm -rf "$plugin_dir"

  log_success "Plugin '$plugin_name' removed"
}

# Update a plugin
cmd_update() {
  local plugin_name=""
  local update_all=false

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --all | -a)
        update_all=true
        shift
        ;;
      -*)
        log_error "Unknown option: $1"
        return 1
        ;;
      *)
        plugin_name="$1"
        shift
        ;;
    esac
  done

  # Default to all if no plugin specified
  if [[ -z "$plugin_name" ]] || [[ "$update_all" == "true" ]]; then
    log_info "Updating all plugins..."
    local found=0
    for plugin_dir in "$PLUGIN_DIR"/*/; do
      if [[ -f "$plugin_dir/plugin.json" ]]; then
        local name
        name=$(basename "$plugin_dir")
        update_single_plugin "$name"
        found=$((found + 1))
      fi
    done
    if [[ $found -eq 0 ]]; then
      log_info "No plugins installed"
    fi
  else
    update_single_plugin "$plugin_name"
  fi
}

update_single_plugin() {
  local plugin_name="$1"

  if ! is_plugin_installed "$plugin_name"; then
    log_error "Plugin '$plugin_name' is not installed"
    return 1
  fi

  log_info "Updating: $plugin_name"

  # Get current version
  local current_version
  current_version=$(get_installed_version "$plugin_name")

  # Fetch registry for latest version
  local registry
  registry=$(fetch_registry)

  local latest_version
  latest_version=$(printf '%s' "$registry" | grep -A5 "\"$plugin_name\"" | grep '"version"' | head -1 | sed 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')

  if [[ "$current_version" == "$latest_version" ]]; then
    log_info "$plugin_name is already at latest version ($current_version)"
    return 0
  fi

  log_info "Updating $plugin_name: $current_version -> $latest_version"

  # Download new version
  download_plugin "$plugin_name" "$latest_version"

  log_success "Updated $plugin_name to $latest_version"
}

# Show plugin status
cmd_status() {
  local plugin_name="${1:-}"

  printf "\n=== Installed Plugins ===\n\n"

  if [[ -n "$plugin_name" ]]; then
    show_plugin_status "$plugin_name"
  else
    local found=0
    for plugin_dir in "$PLUGIN_DIR"/*/; do
      if [[ -f "$plugin_dir/plugin.json" ]]; then
        local name
        name=$(basename "$plugin_dir")
        show_plugin_status "$name"
        found=$((found + 1))
      fi
    done

    if [[ $found -eq 0 ]]; then
      log_info "No plugins installed"
      printf "\nInstall with: nself plugin install <name>\n"
    else
      # Check for updates
      echo ""
      if declare -f registry_check_updates_formatted >/dev/null 2>&1; then
        registry_check_updates_formatted
      fi
    fi
  fi
}

show_plugin_status() {
  local plugin_name="$1"
  local plugin_dir="$PLUGIN_DIR/$plugin_name"

  if [[ ! -d "$plugin_dir" ]]; then
    log_error "Plugin '$plugin_name' is not installed"
    return 1
  fi

  local manifest="$plugin_dir/plugin.json"
  if [[ ! -f "$manifest" ]]; then
    log_error "Invalid plugin: missing plugin.json"
    return 1
  fi

  local version description
  version=$(grep '"version"' "$manifest" | head -1 | sed 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
  description=$(grep '"description"' "$manifest" | head -1 | sed 's/.*"description"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')

  printf "Plugin: %s\n" "$plugin_name"
  printf "  Version: %s\n" "$version"
  printf "  Description: %s\n" "$description"
  printf "  Path: %s\n" "$plugin_dir"

  # Check required env vars
  local required_vars
  required_vars=$(grep -A10 '"required"' "$manifest" | grep -o '"[A-Z_]*"' | tr -d '"' || true)

  if [[ -n "$required_vars" ]]; then
    printf "  Environment:\n"
    for var in $required_vars; do
      if [[ -n "${!var:-}" ]]; then
        printf "    %s: configured\n" "$var"
      else
        printf "    %s: NOT SET\n" "$var"
      fi
    done
  fi

  # Check system dependencies (if dependencies.sh is loaded)
  if declare -f check_plugin_dependencies >/dev/null 2>&1; then
    printf "  Dependencies:\n"

    # Parse and show quick dependency status
    local required_deps=$(parse_system_dependencies "$manifest" "required" 2>/dev/null || echo "")

    if [[ -n "$required_deps" ]]; then
      local dep_count=0
      local dep_ok=0

      while IFS= read -r line; do
        if [[ "$line" =~ \"name\" ]]; then
          local name=$(echo "$line" | sed 's/.*"name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
          dep_count=$((dep_count + 1))

          # Get verify command from next few lines
          local verify_cmd=$(echo "$required_deps" | grep -A5 "\"name\"[[:space:]]*:[[:space:]]*\"$name\"" | grep '"verify"' | head -1 | sed 's/.*"verify"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')

          if verify_dependency "$name" "$verify_cmd" 2>/dev/null; then
            dep_ok=$((dep_ok + 1))
          fi
        fi
      done <<< "$required_deps"

      if [[ $dep_ok -eq $dep_count ]]; then
        printf "    ✓ All %d dependencies satisfied\n" "$dep_count"
      else
        printf "    ⚠ %d/%d dependencies installed\n" "$dep_ok" "$dep_count"
        printf "    Run: nself plugin check-deps %s\n" "$plugin_name"
      fi
    else
      printf "    (none required)\n"
    fi
  fi

  printf "\n"
}

# ============================================================================
# PLUGIN ACTION DISPATCH
# ============================================================================

# Run plugin action
cmd_run_action() {
  local plugin_name="$1"
  local action="$2"
  shift 2 || true

  if [[ -z "$plugin_name" ]]; then
    show_help
    return 1
  fi

  if ! is_plugin_installed "$plugin_name"; then
    log_error "Plugin '$plugin_name' is not installed"
    return 1
  fi

  local plugin_dir="$PLUGIN_DIR/$plugin_name"

  if [[ -z "$action" ]]; then
    show_plugin_help "$plugin_name"
    return 0
  fi

  # Export plugin context
  export NSELF_PLUGIN_PATH="$plugin_dir"
  export NSELF_PROJECT_DIR="$(pwd)"

  # 1. Check for shell script action (original behavior)
  local action_script="$plugin_dir/actions/${action}.sh"
  if [[ -f "$action_script" ]]; then
    bash "$action_script" "$@"
    return $?
  fi

  # 2. Check for built-in actions
  case "$action" in
    init)
      run_builtin_init "$plugin_name" "$@"
      return $?
      ;;
    integrate)
      run_builtin_integrate "$plugin_name" "$@"
      return $?
      ;;
  esac

  # 3. Check if action is defined in plugin manifest
  local manifest="$plugin_dir/plugin.json"
  if [[ -f "$manifest" ]]; then
    local action_exists=false
    local action_desc=""

    if command -v jq >/dev/null 2>&1; then
      if jq -e ".actions[\"$action\"]" "$manifest" >/dev/null 2>&1; then
        action_exists=true
        action_desc=$(jq -r ".actions[\"$action\"].description // \"\"" "$manifest" 2>/dev/null)
      fi
    else
      if grep -q "\"$action\"" "$manifest"; then
        action_exists=true
        action_desc=$(grep -A2 "\"$action\"" "$manifest" | grep '"description"' | head -1 | sed 's/.*"description"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/' || true)
      fi
    fi

    if [[ "$action_exists" == "true" ]]; then
      log_info "Action '$action' is defined in plugin manifest"
      [[ -n "$action_desc" ]] && printf "  Description: %s\n" "$action_desc"
      printf "\nThis action requires the plugin to be running as a service.\n"
      printf "To set up the plugin service:\n\n"
      printf "  nself plugin %s integrate    # Show CS_N configuration\n" "$plugin_name"
      printf "  # Add the configuration to your .env file\n"
      printf "  nself build && nself restart  # Start the service\n\n"
      printf "Once running, plugin actions are available via its API.\n"
      return 0
    fi
  fi

  # 4. Action not found
  log_error "Unknown action: $action"
  show_plugin_help "$plugin_name"
  return 1
}

# ============================================================================
# BUILT-IN PLUGIN ACTIONS
# ============================================================================

# Built-in init action: apply database schema
run_builtin_init() {
  local plugin_name="$1"
  shift || true
  local plugin_dir="$PLUGIN_DIR/$plugin_name"
  local manifest="$plugin_dir/plugin.json"

  log_info "Initializing plugin: $plugin_name"

  # Load project environment
  if declare -f plugin_load_env >/dev/null 2>&1; then
    plugin_load_env "$plugin_name"
  fi

  # Look for schema SQL files
  local schema_applied=false

  # Check for specific schema files (in priority order)
  for sql_file in "$plugin_dir/schema/tables.sql" "$plugin_dir/schema/schema.sql" "$plugin_dir/schema/init.sql"; do
    if [[ -f "$sql_file" ]]; then
      log_info "Applying schema: $(basename "$sql_file")"
      if declare -f plugin_db_exec >/dev/null 2>&1 && plugin_db_exec "$(cat "$sql_file")"; then
        log_success "Schema applied successfully"
        schema_applied=true
      else
        log_error "Failed to apply schema. Is the database running?"
        printf "\nEnsure services are running: nself start\n"
        return 1
      fi
      break
    fi
  done

  # Apply all SQL files in schema directory
  if [[ "$schema_applied" == "false" ]] && [[ -d "$plugin_dir/schema" ]]; then
    for sql_file in "$plugin_dir"/schema/*.sql; do
      if [[ -f "$sql_file" ]]; then
        log_info "Applying: $(basename "$sql_file")"
        if declare -f plugin_db_exec >/dev/null 2>&1 && plugin_db_exec "$(cat "$sql_file")"; then
          schema_applied=true
        else
          log_error "Failed to apply: $(basename "$sql_file")"
        fi
      fi
    done
    if [[ "$schema_applied" == "true" ]]; then
      log_success "Schema applied successfully"
    fi
  fi

  if [[ "$schema_applied" == "true" ]]; then
    log_success "Plugin '$plugin_name' initialized"
    return 0
  fi

  # No schema files found - show table info from manifest
  log_info "No SQL schema files found in plugin directory"

  local tables=""
  if command -v jq >/dev/null 2>&1; then
    tables=$(jq -r '.tables[]? // empty' "$manifest" 2>/dev/null)
  else
    tables=$(grep -o '"np_[a-z_]*"' "$manifest" | tr -d '"' || true)
  fi

  if [[ -n "$tables" ]]; then
    printf "\nPlugin expects these database tables:\n"
    printf '%s\n' "$tables" | while read -r table; do
      [[ -n "$table" ]] && printf "  - %s\n" "$table"
    done
    printf "\nThe plugin will create these tables when started as a service.\n"
  fi

  printf "\nTo run the plugin as a service:\n"
  printf "  nself plugin %s integrate    # Show configuration\n" "$plugin_name"
  printf "  # Add configuration to .env\n"
  printf "  nself build && nself restart  # Start services\n"

  return 0
}

# Built-in integrate action: generate CS_N configuration
run_builtin_integrate() {
  local plugin_name="$1"
  shift || true
  local plugin_dir="$PLUGIN_DIR/$plugin_name"
  local manifest="$plugin_dir/plugin.json"

  # Get plugin config
  local port="3000"
  local description=""

  if command -v jq >/dev/null 2>&1; then
    port=$(jq -r '.port // .config.port // 3000' "$manifest" 2>/dev/null)
    description=$(jq -r '.description // ""' "$manifest" 2>/dev/null)
  else
    port=$(grep '"port"' "$manifest" | head -1 | sed 's/[^0-9]//g')
    description=$(grep '"description"' "$manifest" | head -1 | sed 's/.*"description"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
  fi

  port="${port:-3000}"

  # Load project env to find next CS_N slot
  if declare -f plugin_load_env >/dev/null 2>&1; then
    plugin_load_env "$plugin_name"
  fi

  local next_cs=1
  for i in $(seq 1 10); do
    local cs_var="CS_${i}"
    if [[ -z "${!cs_var:-}" ]]; then
      next_cs=$i
      break
    fi
  done

  # Determine plugin type based on source
  local plugin_type="custom"
  if [[ -f "$plugin_dir/ts/package.json" ]] || [[ -f "$plugin_dir/package.json" ]]; then
    plugin_type="express-js"
  elif [[ -f "$plugin_dir/requirements.txt" ]] || [[ -f "$plugin_dir/setup.py" ]]; then
    plugin_type="fastapi"
  elif [[ -f "$plugin_dir/go.mod" ]]; then
    plugin_type="gin"
  fi

  local upper_name
  upper_name=$(printf '%s' "$plugin_name" | tr '[:lower:]' '[:upper:]' | tr '-' '_')

  printf "\n=== Plugin Integration: %s ===\n\n" "$plugin_name"
  [[ -n "$description" ]] && printf "%s\n\n" "$description"

  printf "Add the following to your .env file:\n\n"

  printf "  # %s Plugin\n" "$plugin_name"
  printf "  CS_%d=%s:%s:%s\n" "$next_cs" "$plugin_name" "$plugin_type" "$port"
  printf "  %s_PLUGIN_ENABLED=true\n" "$upper_name"
  printf "  %s_PLUGIN_PORT=%s\n" "$upper_name" "$port"

  # Show required env vars
  if command -v jq >/dev/null 2>&1; then
    local required_vars
    required_vars=$(jq -r '.envVars.required[]? // empty' "$manifest" 2>/dev/null)
    if [[ -n "$required_vars" ]]; then
      printf "\n  # Required environment variables\n"
      printf '%s\n' "$required_vars" | while read -r var; do
        [[ -n "$var" ]] && printf "  %s=\n" "$var"
      done
    fi

    local optional_vars
    optional_vars=$(jq -r '.envVars.optional[]? // empty' "$manifest" 2>/dev/null)
    if [[ -n "$optional_vars" ]]; then
      printf "\n  # Optional environment variables\n"
      printf '%s\n' "$optional_vars" | while read -r var; do
        [[ -n "$var" ]] && printf "  # %s=\n" "$var"
      done
    fi
  fi

  printf "\nThen run:\n"
  printf "  nself build      # Generate docker-compose config\n"
  printf "  nself restart     # Start/restart services\n\n"
  printf "The plugin will be available at:\n"
  printf "  https://%s.{BASE_DOMAIN}\n\n" "$plugin_name"

  # Show tables
  local tables=""
  if command -v jq >/dev/null 2>&1; then
    tables=$(jq -r '.tables[]? // empty' "$manifest" 2>/dev/null)
  fi
  if [[ -n "$tables" ]]; then
    printf "Database tables (auto-created on startup):\n"
    printf '%s\n' "$tables" | while read -r table; do
      [[ -n "$table" ]] && printf "  - %s\n" "$table"
    done
    printf "\n"
  fi
}

# ============================================================================
# PLUGIN HELP
# ============================================================================

show_plugin_help() {
  local plugin_name="$1"
  local plugin_dir="$PLUGIN_DIR/$plugin_name"
  local manifest="$plugin_dir/plugin.json"

  printf "\nUsage: nself plugin %s <action> [args]\n\n" "$plugin_name"

  # Show built-in actions
  printf "Built-in Actions:\n"
  printf "  %-20s %s\n" "init" "Initialize database schema"
  printf "  %-20s %s\n" "integrate" "Show CS_N service configuration"

  # List shell script actions
  local has_scripts=false
  if [[ -d "$plugin_dir/actions" ]]; then
    for action in "$plugin_dir"/actions/*.sh; do
      if [[ -f "$action" ]]; then
        if [[ "$has_scripts" == "false" ]]; then
          printf "\nScript Actions:\n"
          has_scripts=true
        fi
        local action_name
        action_name=$(basename "$action" .sh)
        local action_desc=""
        if [[ -f "$manifest" ]]; then
          if command -v jq >/dev/null 2>&1; then
            action_desc=$(jq -r ".actions[\"$action_name\"].description // \"\"" "$manifest" 2>/dev/null)
          else
            action_desc=$(grep -A2 "\"$action_name\"" "$manifest" | grep '"description"' | head -1 | sed 's/.*"description"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/' || true)
          fi
        fi
        printf "  %-20s %s\n" "$action_name" "$action_desc"
      fi
    done
  fi

  # List manifest-defined actions (service actions)
  if [[ -f "$manifest" ]]; then
    local manifest_actions=""
    if command -v jq >/dev/null 2>&1; then
      manifest_actions=$(jq -r '.actions // {} | keys[]' "$manifest" 2>/dev/null)
    fi

    if [[ -n "$manifest_actions" ]]; then
      local has_service_actions=false
      while IFS= read -r action_name; do
        [[ -z "$action_name" ]] && continue
        # Skip if shell script exists or is a built-in
        [[ -f "$plugin_dir/actions/${action_name}.sh" ]] && continue
        [[ "$action_name" == "init" || "$action_name" == "integrate" ]] && continue

        if [[ "$has_service_actions" == "false" ]]; then
          printf "\nService Actions (requires running service):\n"
          has_service_actions=true
        fi

        local desc=""
        desc=$(jq -r ".actions[\"$action_name\"].description // \"\"" "$manifest" 2>/dev/null)
        printf "  %-20s %s\n" "$action_name" "$desc"
      done <<< "$manifest_actions"
    fi
  fi

  printf "\n"
}

# ============================================================================
# HELPER FUNCTIONS
# ============================================================================

fetch_registry() {
  # Check cache first
  local cache_file="$PLUGIN_CACHE_DIR/registry.json"
  local cache_age=3600 # 1 hour

  mkdir -p "$PLUGIN_CACHE_DIR"

  if [[ -f "$cache_file" ]]; then
    local file_time current_time
    current_time=$(date +%s)

    if [[ "$OSTYPE" == "darwin"* ]]; then
      file_time=$(stat -f %m "$cache_file" 2>/dev/null || echo 0)
    else
      file_time=$(stat -c %Y "$cache_file" 2>/dev/null || echo 0)
    fi

    if ((current_time - file_time < cache_age)); then
      cat "$cache_file"
      return 0
    fi
  fi

  # Fetch fresh registry
  local registry
  registry=$(curl -s "$PLUGIN_REGISTRY_URL" 2>/dev/null)

  if [[ -n "$registry" ]]; then
    printf '%s' "$registry" >"$cache_file"
    printf '%s' "$registry"
  elif [[ -f "$cache_file" ]]; then
    # Return stale cache if fetch failed
    cat "$cache_file"
  fi
}

is_plugin_installed() {
  local plugin_name="$1"
  [[ -f "$PLUGIN_DIR/$plugin_name/plugin.json" ]]
}

get_installed_version() {
  local plugin_name="$1"
  local manifest="$PLUGIN_DIR/$plugin_name/plugin.json"

  if [[ -f "$manifest" ]]; then
    grep '"version"' "$manifest" | head -1 | sed 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/'
  fi
}

download_plugin() {
  local plugin_name="$1"
  local version="${2:-main}"

  mkdir -p "$PLUGIN_DIR"

  local temp_dir
  temp_dir=$(mktemp -d)

  # Paid plugins are served from the API via a signed download URL.
  # Free plugins come straight from the public GitHub repo.
  local license_key="${NSELF_PLUGIN_LICENSE_KEY:-}"
  local use_signed_url=false
  if declare -f license_is_paid_plugin >/dev/null 2>&1; then
    if license_is_paid_plugin "$plugin_name" && [[ -n "$license_key" ]]; then
      use_signed_url=true
    fi
  fi

  if [[ "$use_signed_url" == "true" ]]; then
    log_info "Downloading $plugin_name (pro)..."
    if ! _download_plugin_signed "$plugin_name" "$license_key" "$temp_dir"; then
      rm -rf "$temp_dir"
      return 1
    fi
  else
    log_info "Downloading $plugin_name..."
    local tarball_url="${PLUGIN_REPO_URL}/archive/refs/heads/main.tar.gz"
    if ! curl -sL "$tarball_url" | tar -xz -C "$temp_dir" 2>/dev/null; then
      log_error "Failed to download plugin"
      rm -rf "$temp_dir"
      return 1
    fi

    local plugin_src="$temp_dir/nself-plugins-main/plugins/$plugin_name"
    if [[ ! -d "$plugin_src" ]]; then
      log_error "Plugin '$plugin_name' not found in repository"
      rm -rf "$temp_dir"
      return 1
    fi

    rm -rf "$PLUGIN_DIR/$plugin_name"
    cp -r "$plugin_src" "$PLUGIN_DIR/$plugin_name"

    if [[ -d "$temp_dir/nself-plugins-main/shared" ]]; then
      mkdir -p "$PLUGIN_DIR/_shared"
      cp -r "$temp_dir/nself-plugins-main/shared/"* "$PLUGIN_DIR/_shared/"
    fi
  fi

  rm -rf "$temp_dir"
  log_success "Downloaded $plugin_name"
}

# Fetch a paid plugin via a server-issued signed URL.
# The API validates the license, generates a time-limited token, and the CLI
# redeems the token in a second request — keeping the license key out of
# download logs and caches.
_download_plugin_signed() {
  local plugin_name="$1"
  local license_key="$2"
  local temp_dir="$3"

  local url_endpoint="${NSELF_API_DOWNLOAD_URL}/${plugin_name}/download-url"

  # Step 1: request a signed download URL from the API
  local response
  response=$(curl -sf \
    -H "X-License-Key: ${license_key}" \
    -H "X-Domain: ${NSELF_DOMAIN:-}" \
    "$url_endpoint" 2>/dev/null)

  if [[ -z "$response" ]]; then
    log_error "Failed to reach plugin distribution service"
    printf "Check your internet connection or run: nself plugin license validate\n"
    return 1
  fi

  # Parse HTTP-level errors returned as JSON { "error": "..." }
  local api_error
  api_error=$(printf '%s' "$response" | grep -o '"error"[[:space:]]*:[[:space:]]*"[^"]*"' | sed 's/.*"error"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
  if [[ -n "$api_error" ]]; then
    log_error "Download authorization failed: $api_error"
    if [[ "$api_error" == *"expired"* ]] || [[ "$api_error" == *"Invalid license"* ]]; then
      printf "Renew your license at: https://nself.org/commercial\n"
    fi
    return 1
  fi

  # Extract the signed URL from the JSON response
  local signed_url
  signed_url=$(printf '%s' "$response" | grep -o '"url"[[:space:]]*:[[:space:]]*"[^"]*"' | sed 's/.*"url"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
  if [[ -z "$signed_url" ]]; then
    log_error "Unexpected response from plugin distribution service"
    return 1
  fi

  # Step 2: download the tarball using the signed URL (no license key needed here)
  local tarball="$temp_dir/${plugin_name}.tar.gz"
  if ! curl -sL -o "$tarball" "$signed_url" 2>/dev/null; then
    log_error "Failed to download plugin tarball"
    return 1
  fi

  if ! tar -xz -C "$temp_dir" -f "$tarball" 2>/dev/null; then
    log_error "Failed to extract plugin tarball"
    return 1
  fi

  # Pro plugins unpack as plugins-pro-main/paid/<plugin_name>/
  local plugin_src="$temp_dir/plugins-pro-main/paid/$plugin_name"
  if [[ ! -d "$plugin_src" ]]; then
    log_error "Plugin '$plugin_name' not found in downloaded archive"
    return 1
  fi

  rm -rf "$PLUGIN_DIR/$plugin_name"
  cp -r "$plugin_src" "$PLUGIN_DIR/$plugin_name"

  # Copy shared utilities bundled with the pro repo (if any)
  if [[ -d "$temp_dir/plugins-pro-main/shared" ]]; then
    mkdir -p "$PLUGIN_DIR/_shared"
    cp -r "$temp_dir/plugins-pro-main/shared/"* "$PLUGIN_DIR/_shared/"
  fi

  return 0
}

install_local_plugin() {
  local plugin_path="$1"

  if [[ ! -f "$plugin_path/plugin.json" ]]; then
    log_error "Invalid plugin: missing plugin.json"
    return 1
  fi

  local plugin_name
  plugin_name=$(grep '"name"' "$plugin_path/plugin.json" | head -1 | sed 's/.*"name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')

  log_info "Installing local plugin: $plugin_name"

  mkdir -p "$PLUGIN_DIR"
  rm -rf "$PLUGIN_DIR/$plugin_name"
  cp -r "$plugin_path" "$PLUGIN_DIR/$plugin_name"

  run_plugin_installer "$plugin_name"

  log_success "Local plugin '$plugin_name' installed"
}

run_plugin_installer() {
  local plugin_name="$1"
  local plugin_dir="$PLUGIN_DIR/$plugin_name"

  if [[ -f "$plugin_dir/install.sh" ]]; then
    log_info "Running plugin installer..."

    # Make shared utilities available
    export PLUGIN_DIR="$plugin_dir"
    export NSELF_PROJECT_DIR="$(pwd)"

    # Update shared path in install script
    local shared_path="$PLUGIN_DIR/_shared"
    if [[ -d "$shared_path" ]]; then
      export SHARED_DIR="$shared_path"
    fi

    bash "$plugin_dir/install.sh"
  fi
}

# ============================================================================
# UPDATE CHECKING
# ============================================================================

# Check for plugin updates
cmd_updates() {
  local quiet=false

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --quiet | -q)
        quiet=true
        shift
        ;;
      *)
        shift
        ;;
    esac
  done

  if [[ "$quiet" == "true" ]]; then
    # Quiet mode: just output update info
    if declare -f registry_check_updates >/dev/null 2>&1; then
      registry_check_updates 2>/dev/null
    fi
  else
    printf "\n=== Plugin Updates ===\n\n"

    # Check if any plugins are installed
    local installed_count=0
    for plugin_dir in "$PLUGIN_DIR"/*/; do
      [[ -f "$plugin_dir/plugin.json" ]] && installed_count=$((installed_count + 1))
    done

    if [[ $installed_count -eq 0 ]]; then
      log_info "No plugins installed"
      printf "\nInstall plugins with: nself plugin install <name>\n"
      return 0
    fi

    log_info "Checking for updates ($installed_count plugins installed)..."
    echo ""

    if declare -f registry_check_updates_formatted >/dev/null 2>&1; then
      registry_check_updates_formatted
    else
      # Fallback if registry.sh not loaded
      log_warning "Registry module not loaded. Skipping update check."
    fi
  fi
}

# Refresh plugin registry cache
cmd_refresh() {
  log_info "Refreshing plugin registry..."

  if declare -f registry_fetch >/dev/null 2>&1; then
    if registry_fetch "true" >/dev/null 2>&1; then
      log_success "Registry cache refreshed"

      # Show registry info
      if declare -f registry_get_metadata >/dev/null 2>&1; then
        echo ""
        registry_get_metadata
      fi
    else
      log_error "Failed to refresh registry"
      return 1
    fi
  else
    # Fallback
    local registry
    if registry=$(curl -sf "$PLUGIN_REGISTRY_URL/registry.json" 2>/dev/null || curl -sf "$PLUGIN_REGISTRY_FALLBACK" 2>/dev/null); then
      mkdir -p "$PLUGIN_CACHE_DIR"
      printf '%s' "$registry" >"$PLUGIN_CACHE_DIR/registry.json"
      log_success "Registry cache refreshed"
    else
      log_error "Failed to fetch registry"
      return 1
    fi
  fi
}

# ============================================================================
# LICENSE MANAGEMENT
# ============================================================================

# Manage the Pro Plugins license key
cmd_plugin_license() {
  local subcmd="${1:-show}"
  shift || true

  case "$subcmd" in
    show | status)
      license_show_status
      ;;

    set)
      local key="${1:-}"
      if [[ -z "$key" ]]; then
        log_error "Usage: nself plugin license set <key>"
        printf "Keys start with 'nself_pro_' — get one at: %s\n" "${NSELF_PRICING_URL:-https://nself.org/pricing}"
        return 1
      fi
      if ! license_validate_format "$key"; then
        log_error "Invalid license key format."
        printf "Key must start with 'nself_pro_' and be at least 32 characters.\n"
        return 1
      fi
      license_save_key "$key"
      log_success "License key saved to ~/.nself/license/key"
      printf "Run 'nself plugin license validate' to verify with the server.\n"
      ;;

    clear | remove)
      license_clear_key
      log_success "License key removed."
      ;;

    validate)
      local license_key
      license_key=$(license_get_key) || true
      if [[ -z "$license_key" ]]; then
        log_error "No license key configured."
        printf "Set one with: nself plugin license set nself_pro_...\n"
        printf "Get a license at: %s\n" "${NSELF_PRICING_URL:-https://nself.org/pricing}"
        return 1
      fi
      if ! license_validate_format "$license_key"; then
        log_error "Invalid license key format."
        printf "Key must start with 'nself_pro_' and be at least 32 characters.\n"
        return 1
      fi
      log_info "Validating license against server..."
      if license_validate_remote "$license_key"; then
        log_success "License is valid."
      else
        log_error "License validation failed. Check or renew at: ${NSELF_PRICING_URL:-https://nself.org/pricing}"
        return 1
      fi
      ;;

    plugins | list)
      printf "\nPro Plugins (require license):\n\n"
      for plugin_name in $NSELF_PRO_PLUGINS; do
        printf "  %s\n" "$plugin_name"
      done
      printf "\nTotal: $(printf '%s' "$NSELF_PRO_PLUGINS" | wc -w | tr -d ' ') pro plugins\n"
      printf "Details: %s\n\n" "${NSELF_PRICING_URL:-https://nself.org/pricing}"
      ;;

    help | --help | -h)
      printf "Usage: nself plugin license <subcommand>\n\n"
      printf "Subcommands:\n"
      printf "  set <key>  Save your Pro Plugins license key\n"
      printf "  clear      Remove saved license key\n"
      printf "  show       Show current license key and status (default)\n"
      printf "  validate   Force-validate license key against the API\n"
      printf "  plugins    List all Pro Plugins covered by a license\n"
      printf "\nQuick start:\n"
      printf "  nself plugin license set nself_pro_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\n"
      printf "  nself plugin install analytics\n"
      printf "\nOr add to your .env:\n"
      printf "  NSELF_PLUGIN_LICENSE_KEY=nself_pro_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\n"
      printf "\nGet a license at: %s\n\n" "${NSELF_PRICING_URL:-https://nself.org/pricing}"
      ;;

    *)
      log_error "Unknown subcommand: $subcmd"
      printf "Run 'nself plugin license help' for usage.\n"
      return 1
      ;;
  esac
}

# ============================================================================
# HELP
# ============================================================================

show_help() {
  printf "
nself plugin - Plugin Management

Usage: nself plugin <command> [options]

Commands:
  list [options]          List available plugins
    --installed, -i         Show only installed plugins
    --detailed, -d          Show detailed status (with --installed)
    --category, -c <cat>    Filter by category (billing, ecommerce, devops)

  install <name>          Install a plugin from registry
  install <path>          Install a local plugin

  remove <name>           Remove a plugin
    --keep-data             Keep database tables

  update [name]           Update a specific plugin
    --all, -a               Update all installed plugins

  status [name]           Show plugin status and health

  updates                 Check for available plugin updates
    --quiet, -q             Output only update info (for scripts)

  refresh                 Force refresh the plugin registry cache

  license [subcommand]    Manage Pro Plugins license
    set <key>               Save your license key persistently
    clear                   Remove saved license key
    show                    Show current license key and status (default)
    validate                Force-validate license key against API
    plugins                 List all 49 Pro Plugins covered by license

  check-deps <name>       Check system dependencies for a plugin
  install-deps <name>     Install missing system dependencies
    --check-only            Dry run (show what would be installed)
  check-conflicts         Scan all installed plugins for dependency conflicts
    --fix                   Auto-install required versions where possible

Runtime Management:
  start <name>            Start a plugin as external process
  start --all             Start all installed plugins (respects dependencies)
  stop <name>             Stop a running plugin
    --force                 Force-stop immediately (SIGKILL)
  stop --all              Stop all running plugins
    --force                 Force-stop all immediately
  restart <name>          Restart a plugin
  logs <name>             View plugin logs
    -f, --follow            Follow log output
  ps | running            List running plugins
  health                  Health check all running plugins

Plugin Actions:
  <plugin> <action>       Run plugin action (e.g., stripe sync)
  <plugin> --help         Show plugin's available actions

Built-in Plugin Actions:
  <plugin> init           Initialize database schema for the plugin
  <plugin> integrate      Show CS_N service configuration for .env

Examples:
  # Installation & management
  nself plugin list
  nself plugin list --installed
  nself plugin list --installed --detailed   # Show states, PIDs, ports
  nself plugin install stripe
  nself plugin update --all
  nself plugin status

  # Runtime (external processes)
  nself plugin start vpn              # Start single plugin
  nself plugin start --all            # Start all (respects dependencies)
  nself plugin stop vpn               # Stop plugin gracefully
  nself plugin stop vpn --force       # Force-stop immediately
  nself plugin stop --all             # Stop all gracefully
  nself plugin stop --all --force     # Force-stop all immediately
  nself plugin restart vpn            # Restart plugin
  nself plugin logs vpn               # View logs
  nself plugin logs vpn -f            # Follow logs
  nself plugin ps                     # List running
  nself plugin health                 # Health check all

  # Plugin actions
  nself plugin stripe sync
  nself plugin stripe customers list
  nself plugin devices init           # Initialize schema
  nself plugin devices integrate      # Show CS_N config

  # Dependencies
  nself plugin check-deps stripe
  nself plugin install-deps stripe
  nself plugin check-conflicts         # Scan for version conflicts
  nself plugin check-conflicts --fix   # Auto-resolve update-needed conflicts

  # License
  nself plugin license               # Show license status
  nself plugin license validate      # Force-validate against API
  nself plugin license plugins       # List all Pro Plugins

Plugin Features:
  • Lifecycle states (starting/running/stopping/stopped/failed)
  • Dependency management (automatic startup ordering)
  • Required environment variable validation
  • Progress indicators for builds
  • Plugin URLs in 'nself urls' command
  • PID/log consolidation in ~/.nself/runtime/

Available Plugins:
  stripe    - Payment processing & subscriptions (billing)
  github    - Repository & CI integration (devops)
  shopify   - E-commerce store sync (ecommerce)
  vpn       - VPN management (NordVPN, PIA, Mullvad)
  See full list: nself plugin list

Registry:
  Primary:  https://plugins.nself.org
  Fallback: https://github.com/nself-org/plugins

Environment:
  NSELF_PLUGIN_DIR          Plugin installation directory (~/.nself/plugins)
  NSELF_PLUGIN_REGISTRY     Custom registry URL (default: https://plugins.nself.org)
  NSELF_REGISTRY_CACHE_TTL  Registry cache TTL in seconds (default: 300)
  NSELF_PLUGIN_RUNTIME      Plugin runtime directory (~/.nself/runtime)
  NSELF_PLUGIN_LICENSE_KEY  Pro Plugins license key (nself_pro_...)

"
}

# ============================================================================
# MAIN
# ============================================================================

main() {
  local command="${1:-}"
  shift || true

  case "$command" in
    list | ls)
      cmd_list "$@"
      ;;
    install | add)
      cmd_install "$@"
      ;;
    remove | rm | uninstall)
      cmd_remove "$@"
      ;;
    update | upgrade)
      cmd_update "$@"
      ;;
    status)
      cmd_status "$@"
      ;;
    updates | check-updates)
      cmd_updates "$@"
      ;;
    refresh | sync-registry)
      cmd_refresh "$@"
      ;;
    check-deps)
      local plugin_name="$1"
      if [[ -z "$plugin_name" ]]; then
        log_error "Plugin name required"
        printf "\nUsage: nself plugin check-deps <name>\n"
        return 1
      fi
      if ! is_plugin_installed "$plugin_name"; then
        log_error "Plugin '$plugin_name' is not installed"
        return 1
      fi
      check_plugin_dependencies "$plugin_name"
      ;;
    check-conflicts)
      local fix_flag="${1:-}"
      if declare -f plugin_resolve_conflicts >/dev/null 2>&1; then
        plugin_resolve_conflicts "$fix_flag"
      else
        log_error "plugin_resolve_conflicts not available"
        return 1
      fi
      ;;
    install-deps)
      local plugin_name="$1"
      shift || true
      local check_only=false
      while [[ $# -gt 0 ]]; do
        case "$1" in
          --check-only)
            check_only=true
            shift
            ;;
          *)
            shift
            ;;
        esac
      done
      if [[ -z "$plugin_name" ]]; then
        log_error "Plugin name required"
        printf "\nUsage: nself plugin install-deps <name> [--check-only]\n"
        return 1
      fi
      if ! is_plugin_installed "$plugin_name"; then
        log_error "Plugin '$plugin_name' is not installed"
        return 1
      fi
      install_plugin_dependencies "$plugin_name" "$check_only"
      ;;
    start)
      # Export project directory for runtime functions to read backend .env files
      export NSELF_PROJECT_DIR="$(pwd)"

      # Check for --all first
      if [[ "$1" == "--all" ]] || [[ "$1" == "-a" ]]; then
        start_all_plugins
      else
        local plugin_name="$1"
        if [[ -z "$plugin_name" ]]; then
          log_error "Plugin name required"
          printf "\nUsage: nself plugin start <name>  OR  nself plugin start --all\n"
          return 1
        fi
        if ! is_plugin_installed "$plugin_name"; then
          log_error "Plugin '$plugin_name' is not installed"
          return 1
        fi
        start_plugin "$plugin_name"
      fi
      ;;
    stop)
      # Export project directory for runtime functions
      export NSELF_PROJECT_DIR="$(pwd)"

      # Check for --all first
      if [[ "$1" == "--all" ]] || [[ "$1" == "-a" ]]; then
        stop_all_plugins
      else
        local plugin_name="$1"
        if [[ -z "$plugin_name" ]]; then
          log_error "Plugin name required"
          printf "\nUsage: nself plugin stop <name>  OR  nself plugin stop --all\n"
          return 1
        fi
        stop_plugin "$plugin_name"
      fi
      ;;
    restart)
      # Export project directory for runtime functions
      export NSELF_PROJECT_DIR="$(pwd)"

      local plugin_name="$1"
      if [[ -z "$plugin_name" ]]; then
        log_error "Plugin name required"
        printf "\nUsage: nself plugin restart <name>\n"
        return 1
      fi
      if ! is_plugin_installed "$plugin_name"; then
        log_error "Plugin '$plugin_name' is not installed"
        return 1
      fi
      restart_plugin "$plugin_name"
      ;;
    logs)
      local plugin_name="$1"
      shift || true
      local follow=false
      while [[ $# -gt 0 ]]; do
        case "$1" in
          -f | --follow)
            follow=true
            shift
            ;;
          *)
            shift
            ;;
        esac
      done
      if [[ -z "$plugin_name" ]]; then
        log_error "Plugin name required"
        printf "\nUsage: nself plugin logs <name> [-f|--follow]\n"
        return 1
      fi
      show_plugin_logs "$plugin_name" "$follow"
      ;;
    ps | running)
      list_running_plugins
      ;;
    health)
      health_check_all
      ;;
    license)
      cmd_plugin_license "$@"
      ;;
    -h | --help | help | "")
      show_help
      ;;
    *)
      # Check if it's a plugin name (for running actions)
      if is_plugin_installed "$command"; then
        cmd_run_action "$command" "$@"
      else
        log_error "Unknown command: $command"
        show_help
        return 1
      fi
      ;;
  esac
}

main "$@"
