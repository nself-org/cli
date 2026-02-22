#!/usr/bin/env bash

# core.sh - Core plugin utilities for nself
# Provides common functions for plugin management

# ============================================================================
# Plugin Registry
# ============================================================================

PLUGIN_REGISTRY_URL="${NSELF_PLUGIN_REGISTRY:-https://raw.githubusercontent.com/nself-org/plugins/main/registry.json}"

set -euo pipefail

PLUGIN_DIR="${NSELF_PLUGIN_DIR:-$HOME/.nself/plugins}"
PLUGIN_CACHE_DIR="${NSELF_PLUGIN_CACHE:-$HOME/.nself/cache/plugins}"

# ============================================================================
# Plugin Information
# ============================================================================

# Get plugin info from manifest
plugin_get_info() {
  local plugin_name="$1"
  local field="$2"
  local manifest="$PLUGIN_DIR/$plugin_name/plugin.json"

  if [[ ! -f "$manifest" ]]; then
    return 1
  fi

  grep "\"$field\"" "$manifest" | head -1 | sed 's/.*"'"$field"'"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/'
}

# Get list of installed plugins
plugin_list_installed() {
  local plugins=()

  for plugin_dir in "$PLUGIN_DIR"/*/; do
    if [[ -f "$plugin_dir/plugin.json" ]]; then
      plugins+=("$(basename "$plugin_dir")")
    fi
  done

  if [[ ${#plugins[@]} -gt 0 ]]; then
    printf '%s\n' "${plugins[@]}"
  fi
}

# Check if plugin is compatible with current nself version
plugin_check_compatibility() {
  local plugin_name="$1"

  local min_version
  min_version=$(plugin_get_info "$plugin_name" "minNselfVersion")

  if [[ -z "$min_version" ]]; then
    return 0
  fi

  local current_version
  current_version=$(cat "$(dirname "${BASH_SOURCE[0]}")/../../VERSION" 2>/dev/null || echo "0.0.0")

  # Compare versions
  plugin_version_compare "$current_version" "$min_version"
}

# Compare semantic versions
# Returns 0 if v1 >= v2, 1 if v1 < v2
plugin_version_compare() {
  local v1="$1"
  local v2="$2"

  # Remove 'v' prefix if present
  v1="${v1#v}"
  v2="${v2#v}"

  # Split into parts using IFS
  local v1_major v1_minor v1_patch
  local v2_major v2_minor v2_patch

  IFS='.' read -r v1_major v1_minor v1_patch <<<"$v1"
  IFS='.' read -r v2_major v2_minor v2_patch <<<"$v2"

  # Default to 0 if empty
  v1_major="${v1_major:-0}"
  v1_minor="${v1_minor:-0}"
  v1_patch="${v1_patch:-0}"
  v2_major="${v2_major:-0}"
  v2_minor="${v2_minor:-0}"
  v2_patch="${v2_patch:-0}"

  if ((v1_major > v2_major)); then
    return 0
  elif ((v1_major < v2_major)); then
    return 1
  fi

  if ((v1_minor > v2_minor)); then
    return 0
  elif ((v1_minor < v2_minor)); then
    return 1
  fi

  if ((v1_patch >= v2_patch)); then
    return 0
  else
    return 1
  fi
}

# ============================================================================
# Plugin Database Operations
# ============================================================================

# Get database connection for plugin operations
plugin_db_connection() {
  local db_host="${POSTGRES_HOST:-localhost}"
  local db_port="${POSTGRES_PORT:-5432}"
  local db_name="${POSTGRES_DB:-nself}"
  local db_user="${POSTGRES_USER:-postgres}"
  local db_pass="${POSTGRES_PASSWORD:-}"

  printf "postgresql://%s:%s@%s:%s/%s" "$db_user" "$db_pass" "$db_host" "$db_port" "$db_name"
}

# Execute SQL for plugin
plugin_db_exec() {
  local sql="$1"
  local container="${PROJECT_NAME:-nself}_postgres"

  if docker ps --format "{{.Names}}" 2>/dev/null | grep -q "^${container}$"; then
    docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself}" -c "$sql" 2>/dev/null
  elif command -v psql >/dev/null 2>&1; then
    psql "$(plugin_db_connection)" -c "$sql" 2>/dev/null
  else
    return 1
  fi
}

# ============================================================================
# Plugin Versioning
# ============================================================================

# Get plugin version from manifest
plugin_get_version() {
  local plugin_name="$1"
  local manifest="$PLUGIN_DIR/$plugin_name/plugin.json"

  if [[ ! -f "$manifest" ]]; then
    return 1
  fi

  grep '"version"' "$manifest" | head -1 | sed 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/'
}

# Check if plugin version satisfies requirement
plugin_version_satisfies() {
  local current="$1"
  local requirement="$2"

  # Handle version operators: >=, >, <, <=, =, ^, ~
  local operator=""
  local required_version=""

  if [[ "$requirement" =~ ^([><=^~]+)(.+)$ ]]; then
    operator="${BASH_REMATCH[1]}"
    required_version="${BASH_REMATCH[2]}"
  else
    operator="="
    required_version="$requirement"
  fi

  case "$operator" in
    ">=")
      plugin_version_compare "$current" "$required_version"
      ;;
    ">")
      plugin_version_compare "$current" "$required_version" && [[ "$current" != "$required_version" ]]
      ;;
    "<=")
      ! plugin_version_compare "$current" "$required_version" || [[ "$current" == "$required_version" ]]
      ;;
    "<")
      ! plugin_version_compare "$current" "$required_version"
      ;;
    "=")
      [[ "$current" == "$required_version" ]]
      ;;
    "^")
      # Caret: compatible with version (same major)
      plugin_check_caret_compat "$current" "$required_version"
      ;;
    "~")
      # Tilde: approximately equivalent (same major.minor)
      plugin_check_tilde_compat "$current" "$required_version"
      ;;
    *)
      return 1
      ;;
  esac
}

# Check caret compatibility (^1.2.3 allows >=1.2.3 <2.0.0)
plugin_check_caret_compat() {
  local current="$1"
  local required="$2"

  local curr_major curr_minor curr_patch
  local req_major req_minor req_patch

  IFS='.' read -r curr_major curr_minor curr_patch <<<"${current#v}"
  IFS='.' read -r req_major req_minor req_patch <<<"${required#v}"

  curr_major="${curr_major:-0}"
  req_major="${req_major:-0}"

  # Must have same major version and be >= required version
  [[ "$curr_major" == "$req_major" ]] && plugin_version_compare "$current" "$required"
}

# Check tilde compatibility (~1.2.3 allows >=1.2.3 <1.3.0)
plugin_check_tilde_compat() {
  local current="$1"
  local required="$2"

  local curr_major curr_minor curr_patch
  local req_major req_minor req_patch

  IFS='.' read -r curr_major curr_minor curr_patch <<<"${current#v}"
  IFS='.' read -r req_major req_minor req_patch <<<"${required#v}"

  curr_major="${curr_major:-0}"
  curr_minor="${curr_minor:-0}"
  req_major="${req_major:-0}"
  req_minor="${req_minor:-0}"

  # Must have same major.minor and be >= required version
  [[ "$curr_major" == "$req_major" ]] && [[ "$curr_minor" == "$req_minor" ]] && plugin_version_compare "$current" "$required"
}

# ============================================================================
# Plugin Dependency Management
# ============================================================================

# Get plugin dependencies from manifest
plugin_get_dependencies() {
  local plugin_name="$1"
  local manifest="$PLUGIN_DIR/$plugin_name/plugin.json"

  if [[ ! -f "$manifest" ]]; then
    return 1
  fi

  # Extract dependencies section (basic parsing)
  grep -A20 '"dependencies"' "$manifest" | grep -o '"[a-z-]*"[[:space:]]*:[[:space:]]*"[^"]*"' | sed 's/"//g;s/[[:space:]]*:[[:space:]]*/|/' || true
}

# Check if dependencies are satisfied
plugin_check_dependencies() {
  local plugin_name="$1"
  local manifest="$PLUGIN_DIR/$plugin_name/plugin.json"

  if [[ ! -f "$manifest" ]]; then
    return 1
  fi

  local missing=0
  local dependencies
  dependencies=$(plugin_get_dependencies "$plugin_name")

  if [[ -z "$dependencies" ]]; then
    return 0
  fi

  while IFS='|' read -r dep_name dep_version; do
    case "$dep_name" in
      nself)
        # Check nself version
        local nself_version
        nself_version=$(cat "$(dirname "${BASH_SOURCE[0]}")/../../VERSION" 2>/dev/null || echo "0.0.0")
        if ! plugin_version_satisfies "$nself_version" "$dep_version"; then
          printf "Dependency not satisfied: nself %s (current: %s)\n" "$dep_version" "$nself_version" >&2
          missing=$((missing + 1))
        fi
        ;;
      postgres)
        # Check PostgreSQL version
        if command -v psql >/dev/null 2>&1; then
          local pg_version
          pg_version=$(psql --version 2>/dev/null | grep -o '[0-9]\+\.[0-9]\+' | head -1 || echo "0.0")
          if ! plugin_version_satisfies "$pg_version" "$dep_version"; then
            printf "Dependency not satisfied: postgres %s (current: %s)\n" "$dep_version" "$pg_version" >&2
            missing=$((missing + 1))
          fi
        else
          printf "Dependency not satisfied: postgres %s (not found)\n" "$dep_version" >&2
          missing=$((missing + 1))
        fi
        ;;
      node)
        # Check Node.js version
        if command -v node >/dev/null 2>&1; then
          local node_version
          node_version=$(node --version 2>/dev/null | sed 's/v//' || echo "0.0.0")
          if ! plugin_version_satisfies "$node_version" "$dep_version"; then
            printf "Dependency not satisfied: node %s (current: %s)\n" "$dep_version" "$node_version" >&2
            missing=$((missing + 1))
          fi
        else
          printf "Dependency not satisfied: node %s (not found)\n" "$dep_version" >&2
          missing=$((missing + 1))
        fi
        ;;
      *)
        # Check if it's another plugin
        if [[ -d "$PLUGIN_DIR/$dep_name" ]]; then
          local installed_version
          installed_version=$(plugin_get_version "$dep_name")
          if ! plugin_version_satisfies "$installed_version" "$dep_version"; then
            printf "Dependency not satisfied: %s %s (current: %s)\n" "$dep_name" "$dep_version" "$installed_version" >&2
            missing=$((missing + 1))
          fi
        else
          printf "Dependency not satisfied: %s %s (not installed)\n" "$dep_name" "$dep_version" >&2
          missing=$((missing + 1))
        fi
        ;;
    esac
  done <<<"$dependencies"

  return $missing
}

# Guard variable: colon-separated list of plugins currently being installed
# Prevents infinite recursion when plugins have circular dependencies.
: "${_NSELF_INSTALLING_DEPS:=}"

# Install plugin dependencies
plugin_install_dependencies() {
  local plugin_name="$1"
  local dependencies
  dependencies=$(plugin_get_dependencies "$plugin_name")

  if [[ -z "$dependencies" ]]; then
    return 0
  fi

  printf "Installing dependencies for %s...\n" "$plugin_name"
  local failed=0

  while IFS='|' read -r dep_name dep_version; do
    # Skip system-level dependencies — handled by check_plugin_dependencies
    case "$dep_name" in
      nself|postgres|node)
        continue
        ;;
    esac

    # Already installed at a satisfying version — nothing to do
    if [[ -d "$PLUGIN_DIR/$dep_name" ]]; then
      local installed_ver
      installed_ver=$(plugin_get_version "$dep_name" 2>/dev/null || printf "0.0.0")
      if plugin_version_satisfies "$installed_ver" "$dep_version"; then
        printf "  %-20s already installed (v%s)\n" "$dep_name" "$installed_ver"
        continue
      fi
    fi

    # Guard against circular dependency chains
    case ":${_NSELF_INSTALLING_DEPS}:" in
      *":${dep_name}:"*)
        printf "  Skipping circular dependency: %s\n" "$dep_name" >&2
        continue
        ;;
    esac

    printf "  Installing dependency: %s@%s\n" "$dep_name" "$dep_version"

    if declare -f cmd_install >/dev/null 2>&1; then
      # Track this dep to prevent re-entrant loops
      local prev_deps="$_NSELF_INSTALLING_DEPS"
      _NSELF_INSTALLING_DEPS="${_NSELF_INSTALLING_DEPS}:${dep_name}"

      if ! cmd_install "$dep_name"; then
        printf "  Could not auto-install '%s'. Run: nself plugin install %s\n" \
          "$dep_name" "$dep_name" >&2
        failed=$((failed + 1))
      fi

      _NSELF_INSTALLING_DEPS="$prev_deps"
    else
      # cmd_install not in scope — print manual instructions
      printf "  Run: nself plugin install %s\n" "$dep_name"
    fi
  done <<<"$dependencies"

  return $failed
}

# Scan all installed plugins for dependency version conflicts.
#
# Collects every plugin-to-plugin dependency requirement, then for each shared
# dependency checks:
#   1. Update needed  — installed version doesn't satisfy one or more requirements
#                       but a single higher version would satisfy all of them.
#   2. True conflict  — requirements from different plugins are mutually exclusive
#                       (no single version can satisfy all constraints).
#
# Usage: plugin_resolve_conflicts [--fix]
#   --fix   attempt to auto-install the required version for "update needed" cases
#
# Returns 0 if no conflicts, 1 if any conflicts are found.
plugin_resolve_conflicts() {
  local fix_mode=false
  if [[ "${1:-}" == "--fix" ]]; then
    fix_mode=true
  fi

  # Build requirement list: newline-separated "dep|requiring_plugin|version_req"
  local req_lines=""
  local plugin_dir plugin
  for plugin_dir in "$PLUGIN_DIR"/*/; do
    [[ ! -d "$plugin_dir" ]] && continue
    plugin=$(basename "$plugin_dir")
    [[ "$plugin" == "_shared" ]] && continue

    local deps
    deps=$(plugin_get_dependencies "$plugin" 2>/dev/null || true)
    [[ -z "$deps" ]] && continue

    while IFS='|' read -r dep_name dep_version; do
      case "$dep_name" in
        nself|postgres|node|"") continue ;;
      esac
      req_lines="${req_lines}${dep_name}|${plugin}|${dep_version}"$'\n'
    done <<<"$deps"
  done

  if [[ -z "$req_lines" ]]; then
    printf "No inter-plugin dependencies found.\n"
    return 0
  fi

  local conflicts=0
  local processed_deps="|"

  # Iterate over unique dep names found in req_lines
  while IFS='|' read -r dep_name _requirer _version; do
    [[ -z "$dep_name" ]] && continue

    # Skip already-processed deps
    case "$processed_deps" in
      *"|${dep_name}|"*) continue ;;
    esac
    processed_deps="${processed_deps}${dep_name}|"

    # Gather all requirements for this specific dep
    local requirements=""
    while IFS='|' read -r rdep rrequirer rversion; do
      [[ "$rdep" != "$dep_name" || -z "$rrequirer" ]] && continue
      requirements="${requirements}${rrequirer}|${rversion}"$'\n'
    done <<<"$req_lines"

    [[ -z "$requirements" ]] && continue

    # Find the highest minimum-version across all requirements
    local max_required="0.0.0"
    while IFS='|' read -r _rrequirer rversion; do
      [[ -z "$rversion" ]] && continue
      local numeric_ver
      numeric_ver=$(printf '%s' "$rversion" | sed 's/^[><=^~]*//')
      if plugin_version_compare "$numeric_ver" "$max_required" 2>/dev/null; then
        max_required="$numeric_ver"
      fi
    done <<<"$requirements"

    # Check whether max_required satisfies every individual requirement
    # (If it doesn't, requirements are mutually exclusive — a true conflict)
    local true_conflict=false
    local conflict_detail=""
    while IFS='|' read -r rrequirer rversion; do
      [[ -z "$rrequirer" ]] && continue
      if ! plugin_version_satisfies "$max_required" "$rversion" 2>/dev/null; then
        true_conflict=true
        conflict_detail="${conflict_detail}    ${rrequirer} requires ${rversion}"$'\n'
      fi
    done <<<"$requirements"

    # Check whether the currently-installed version satisfies all requirements
    local installed_ver="not installed"
    local update_needed=false
    if [[ -d "$PLUGIN_DIR/$dep_name" ]]; then
      installed_ver=$(plugin_get_version "$dep_name" 2>/dev/null || printf "0.0.0")
      while IFS='|' read -r _rrequirer rversion; do
        [[ -z "$rversion" ]] && continue
        if ! plugin_version_satisfies "$installed_ver" "$rversion" 2>/dev/null; then
          update_needed=true
        fi
      done <<<"$requirements"
    else
      update_needed=true
    fi

    if [[ "$true_conflict" == "true" ]]; then
      conflicts=$((conflicts + 1))
      printf "\nConflict: %s\n" "$dep_name"
      printf "  No single version satisfies all requirements:\n"
      printf '%s' "$conflict_detail"
      printf "  Action: update the plugins listed above to agree on a compatible version.\n"

    elif [[ "$update_needed" == "true" ]]; then
      conflicts=$((conflicts + 1))
      local req_summary=""
      while IFS='|' read -r rrequirer rversion; do
        [[ -z "$rrequirer" ]] && continue
        req_summary="${req_summary}${rrequirer}(${rversion}) "
      done <<<"$requirements"

      printf "\nUpdate needed: %s\n" "$dep_name"
      printf "  Installed:   %s\n" "$installed_ver"
      printf "  Required by: %s\n" "$req_summary"
      printf "  Resolution:  nself plugin install %s@%s\n" "$dep_name" "$max_required"

      if [[ "$fix_mode" == "true" ]] && declare -f cmd_install >/dev/null 2>&1; then
        printf "  Auto-fixing: installing %s@%s...\n" "$dep_name" "$max_required"
        local prev_deps="$_NSELF_INSTALLING_DEPS"
        _NSELF_INSTALLING_DEPS="${_NSELF_INSTALLING_DEPS}:${dep_name}"
        cmd_install "${dep_name}@${max_required}" || printf "  Auto-fix failed for %s\n" "$dep_name" >&2
        _NSELF_INSTALLING_DEPS="$prev_deps"
      fi
    fi
  done <<<"$req_lines"

  if [[ $conflicts -eq 0 ]]; then
    printf "All plugin dependencies are compatible.\n"
    return 0
  fi

  if [[ "$fix_mode" == "false" ]]; then
    printf "\n%d conflict(s) found. Run: nself plugin check-conflicts --fix\n" "$conflicts"
  fi
  return 1
}

# ============================================================================
# Plugin Lifecycle Hooks
# ============================================================================

# Run pre-install hooks
plugin_pre_install() {
  local plugin_name="$1"
  local plugin_dir="${2:-$PLUGIN_DIR/$plugin_name}"

  printf "Running pre-install hooks for %s...\n" "$plugin_name"

  # Check compatibility
  if [[ -f "$plugin_dir/plugin.json" ]]; then
    if ! plugin_check_compatibility "$plugin_name"; then
      printf "Plugin '%s' requires a newer version of nself\n" "$plugin_name" >&2
      return 1
    fi

    # Check dependencies
    if ! plugin_check_dependencies "$plugin_name"; then
      printf "Plugin '%s' has unsatisfied dependencies\n" "$plugin_name" >&2
      return 1
    fi
  fi

  # Run custom pre-install script if exists
  if [[ -f "$plugin_dir/hooks/pre-install.sh" ]]; then
    printf "  Running custom pre-install script...\n"
    bash "$plugin_dir/hooks/pre-install.sh" || return 1
  fi

  return 0
}

# Run post-install hooks
plugin_post_install() {
  local plugin_name="$1"
  local plugin_dir="${2:-$PLUGIN_DIR/$plugin_name}"

  printf "Running post-install hooks for %s...\n" "$plugin_name"

  # Verify installation
  if [[ ! -f "$plugin_dir/plugin.json" ]]; then
    printf "Plugin installation failed: missing plugin.json\n" >&2
    return 1
  fi

  # Install dependencies (if any)
  plugin_install_dependencies "$plugin_name" || true

  # Run custom post-install script if exists
  if [[ -f "$plugin_dir/hooks/post-install.sh" ]]; then
    printf "  Running custom post-install script...\n"
    bash "$plugin_dir/hooks/post-install.sh" || return 1
  fi

  # Apply database schema if exists
  if [[ -f "$plugin_dir/schema/schema.sql" ]]; then
    printf "  Database schema found (run 'nself plugin %s schema apply' to apply)\n" "$plugin_name"
  fi

  return 0
}

# Run pre-uninstall hooks
plugin_pre_uninstall() {
  local plugin_name="$1"
  local plugin_dir="${2:-$PLUGIN_DIR/$plugin_name}"
  local keep_data="${3:-false}"

  printf "Running pre-uninstall hooks for %s...\n" "$plugin_name"

  # Run custom pre-uninstall script if exists
  if [[ -f "$plugin_dir/hooks/pre-uninstall.sh" ]]; then
    printf "  Running custom pre-uninstall script...\n"
    if [[ "$keep_data" == "true" ]]; then
      bash "$plugin_dir/hooks/pre-uninstall.sh" --keep-data || return 1
    else
      bash "$plugin_dir/hooks/pre-uninstall.sh" || return 1
    fi
  fi

  return 0
}

# Run post-uninstall hooks
plugin_post_uninstall() {
  local plugin_name="$1"
  local keep_data="${2:-false}"

  printf "Running post-uninstall hooks for %s...\n" "$plugin_name"

  # Cleanup cache
  if [[ -d "$PLUGIN_CACHE_DIR/$plugin_name" ]]; then
    rm -rf "$PLUGIN_CACHE_DIR/$plugin_name"
  fi

  if [[ "$keep_data" == "false" ]]; then
    printf "  Plugin data removed\n"
  else
    printf "  Plugin data preserved\n"
  fi

  return 0
}

# Run pre-update hooks
plugin_pre_update() {
  local plugin_name="$1"
  local current_version="$2"
  local new_version="$3"
  local plugin_dir="${4:-$PLUGIN_DIR/$plugin_name}"

  printf "Running pre-update hooks for %s (%s -> %s)...\n" "$plugin_name" "$current_version" "$new_version"

  # Run custom pre-update script if exists
  if [[ -f "$plugin_dir/hooks/pre-update.sh" ]]; then
    printf "  Running custom pre-update script...\n"
    export PLUGIN_CURRENT_VERSION="$current_version"
    export PLUGIN_NEW_VERSION="$new_version"
    bash "$plugin_dir/hooks/pre-update.sh" || return 1
  fi

  return 0
}

# Run post-update hooks
plugin_post_update() {
  local plugin_name="$1"
  local old_version="$2"
  local new_version="$3"
  local plugin_dir="${4:-$PLUGIN_DIR/$plugin_name}"

  printf "Running post-update hooks for %s (%s -> %s)...\n" "$plugin_name" "$old_version" "$new_version"

  # Run custom post-update script if exists
  if [[ -f "$plugin_dir/hooks/post-update.sh" ]]; then
    printf "  Running custom post-update script...\n"
    export PLUGIN_OLD_VERSION="$old_version"
    export PLUGIN_NEW_VERSION="$new_version"
    bash "$plugin_dir/hooks/post-update.sh" || return 1
  fi

  # Check for schema migrations
  if [[ -d "$plugin_dir/schema/migrations" ]]; then
    printf "  Schema migrations found (run 'nself plugin %s migrate' to apply)\n" "$plugin_name"
  fi

  return 0
}

# ============================================================================
# Plugin Environment
# ============================================================================

# Load plugin environment
plugin_load_env() {
  local plugin_name="$1"

  # Source project .env if exists
  if [[ -f ".env" ]]; then
    set -a
    source ".env" 2>/dev/null || true
    set +a
  fi
}

# Check required environment variables
plugin_check_env() {
  local plugin_name="$1"
  local manifest="$PLUGIN_DIR/$plugin_name/plugin.json"

  if [[ ! -f "$manifest" ]]; then
    return 1
  fi

  # Extract required env vars
  local required_vars
  required_vars=$(grep -A10 '"required"' "$manifest" | grep -o '"[A-Z_]*"' | tr -d '"' || true)

  local missing=0
  for var in $required_vars; do
    if [[ -z "${!var:-}" ]]; then
      printf "Missing required variable: %s\n" "$var" >&2
      missing=$((missing + 1))
    fi
  done

  return $missing
}

# ============================================================================
# Plugin Security Validation
# ============================================================================

# Validate plugin manifest structure
plugin_validate_manifest() {
  local plugin_name="$1"
  local manifest="$PLUGIN_DIR/$plugin_name/plugin.json"

  if [[ ! -f "$manifest" ]]; then
    printf "Error: Missing plugin.json\n" >&2
    return 1
  fi

  local errors=0

  # Check required fields
  local required_fields=("name" "version" "description" "author" "license")
  for field in "${required_fields[@]}"; do
    if ! grep -q "\"$field\"" "$manifest"; then
      printf "Error: Missing required field: %s\n" "$field" >&2
      errors=$((errors + 1))
    fi
  done

  # Validate version format (semver)
  local version
  version=$(grep '"version"' "$manifest" | head -1 | sed 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
  if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?(\+[a-zA-Z0-9.]+)?$ ]]; then
    printf "Error: Invalid version format (must be semver): %s\n" "$version" >&2
    errors=$((errors + 1))
  fi

  # Check for valid JSON (if jq available)
  if command -v jq >/dev/null 2>&1; then
    if ! jq empty "$manifest" 2>/dev/null; then
      printf "Error: Invalid JSON in plugin.json\n" >&2
      errors=$((errors + 1))
    fi
  fi

  return $errors
}

# Validate plugin permissions
plugin_validate_permissions() {
  local plugin_name="$1"
  local manifest="$PLUGIN_DIR/$plugin_name/plugin.json"

  if [[ ! -f "$manifest" ]]; then
    return 1
  fi

  # Check if permissions are declared
  if ! grep -q '"permissions"' "$manifest"; then
    printf "Warning: No permissions declared in plugin.json\n" >&2
    return 0
  fi

  # Validate permission scope (basic check)
  local dangerous_perms=0

  # Check for dangerous database permissions
  if grep -q '"drop"' "$manifest"; then
    printf "Warning: Plugin requests DROP permissions\n" >&2
    dangerous_perms=$((dangerous_perms + 1))
  fi

  # Check for broad filesystem access
  if grep -q '"\/"' "$manifest"; then
    printf "Warning: Plugin requests root filesystem access\n" >&2
    dangerous_perms=$((dangerous_perms + 1))
  fi

  # Check for Docker access
  if grep -A5 '"docker"' "$manifest" | grep -q '"execute"[[:space:]]*:[[:space:]]*true'; then
    printf "Warning: Plugin requests Docker execution permissions\n" >&2
    dangerous_perms=$((dangerous_perms + 1))
  fi

  if [[ $dangerous_perms -gt 0 ]]; then
    printf "\nThis plugin requests %d potentially dangerous permissions.\n" $dangerous_perms
    printf "Review plugin.json before installation.\n"
  fi

  return 0
}

# Check plugin file integrity
plugin_validate_integrity() {
  local plugin_name="$1"
  local plugin_dir="$PLUGIN_DIR/$plugin_name"
  local expected_checksum="${2:-}"

  if [[ -z "$expected_checksum" ]]; then
    return 0
  fi

  printf "Validating plugin integrity...\n"

  # Calculate checksum of plugin directory
  local actual_checksum
  if command -v sha256sum >/dev/null 2>&1; then
    actual_checksum=$(find "$plugin_dir" -type f -exec sha256sum {} \; | sort | sha256sum | cut -d' ' -f1)
  elif command -v shasum >/dev/null 2>&1; then
    actual_checksum=$(find "$plugin_dir" -type f -exec shasum -a 256 {} \; | sort | shasum -a 256 | cut -d' ' -f1)
  else
    printf "Warning: No checksum tool available\n" >&2
    return 0
  fi

  if [[ "$actual_checksum" != "$expected_checksum" ]]; then
    printf "Error: Plugin integrity check failed\n" >&2
    printf "  Expected: %s\n" "$expected_checksum" >&2
    printf "  Actual:   %s\n" "$actual_checksum" >&2
    return 1
  fi

  printf "  Plugin integrity verified\n"
  return 0
}

# Scan plugin for security issues
plugin_security_scan() {
  local plugin_name="$1"
  local plugin_dir="$PLUGIN_DIR/$plugin_name"

  printf "Scanning plugin for security issues...\n"

  local issues=0

  # Check for suspicious patterns in shell scripts
  if find "$plugin_dir" -name "*.sh" -type f -exec grep -l "eval\|exec\|curl.*|.*bash\|wget.*|.*bash" {} \; 2>/dev/null | grep -q .; then
    printf "  Warning: Found potentially dangerous shell patterns\n"
    issues=$((issues + 1))
  fi

  # Check for hardcoded credentials
  if find "$plugin_dir" -type f -exec grep -l "password\s*=\|api_key\s*=\|secret\s*=" {} \; 2>/dev/null | grep -q .; then
    printf "  Warning: Found potential hardcoded credentials\n"
    issues=$((issues + 1))
  fi

  # Check for network access in unexpected files
  if find "$plugin_dir" -name "*.sh" -type f -exec grep -l "curl\|wget\|nc\|telnet" {} \; 2>/dev/null | grep -q .; then
    printf "  Info: Plugin makes network requests\n"
  fi

  if [[ $issues -eq 0 ]]; then
    printf "  No obvious security issues found\n"
  else
    printf "  Found %d potential security issues\n" $issues
  fi

  return 0
}

# nself release signing identity (placeholder — replace with real key email/fingerprint
# once the nself signing key pair is generated and published to keys.openpgp.org).
NSELF_SIGNING_KEY="${NSELF_SIGNING_KEY:-nself-releases@nself.org}"

# Keyserver used to import the nself signing key when it is not already in the local
# GPG keyring.
NSELF_GPG_KEYSERVER="${NSELF_GPG_KEYSERVER:-hkps://keys.openpgp.org}"

# Validate the GPG detached signature of a plugin tarball.
#
# Usage:
#   plugin_validate_signature <plugin_name> <tarball_path> [sig_path]
#
# Arguments:
#   plugin_name   Human-readable name used in log messages.
#   tarball_path  Absolute or relative path to the downloaded tarball.
#   sig_path      Optional path to the detached .asc signature file.
#                 Defaults to <tarball_path>.asc when omitted.
#
# Returns:
#   0  Signature is valid (or gpg is absent — non-blocking warning).
#   1  Signature is present but invalid.
plugin_validate_signature() {
  local plugin_name="$1"
  local tarball="${2:-}"
  local sig_file="${3:-}"

  # ── 1. Require a tarball path ─────────────────────────────────────────────
  if [[ -z "$tarball" ]]; then
    printf "Info: No tarball path supplied to plugin_validate_signature — skipping\n"
    return 0
  fi

  # ── 2. Derive default sig path when not provided ──────────────────────────
  if [[ -z "$sig_file" ]]; then
    sig_file="${tarball}.asc"
  fi

  # ── 3. Skip silently when no signature file is present ────────────────────
  if [[ ! -f "$sig_file" ]]; then
    printf "Info: No signature file found for '%s' — skipping GPG verification\n" "$plugin_name"
    return 0
  fi

  # ── 4. Require gpg; warn but do not block when absent ─────────────────────
  if ! command -v gpg >/dev/null 2>&1; then
    printf "Warning: gpg not found — cannot verify signature for '%s'\n" "$plugin_name"
    printf "         Install gpg to enable signature verification.\n"
    return 0
  fi

  # ── 5. Import the nself signing key if it is not already trusted ──────────
  local key_in_ring
  key_in_ring=$(gpg --list-keys "$NSELF_SIGNING_KEY" 2>/dev/null)
  if [[ -z "$key_in_ring" ]]; then
    printf "Info: Importing nself signing key from %s ...\n" "$NSELF_GPG_KEYSERVER"
    if ! gpg --keyserver "$NSELF_GPG_KEYSERVER" \
             --recv-keys "$NSELF_SIGNING_KEY" >/dev/null 2>&1; then
      printf "Warning: Could not import nself signing key — skipping verification\n"
      printf "         Ensure network access to %s\n" "$NSELF_GPG_KEYSERVER"
      return 0
    fi
  fi

  # ── 6. Verify the detached signature ─────────────────────────────────────
  local gpg_output
  if gpg_output=$(gpg --verify "$sig_file" "$tarball" 2>&1); then
    printf "Info: GPG signature verified for '%s'\n" "$plugin_name"
    return 0
  else
    printf "Error: GPG signature verification FAILED for '%s'\n" "$plugin_name" >&2
    printf "       Signature file : %s\n" "$sig_file" >&2
    printf "       Tarball        : %s\n" "$tarball" >&2
    printf "       GPG output:\n%s\n" "$gpg_output" >&2
    printf "       Do NOT install this plugin — the download may be corrupted\n" >&2
    printf "       or tampered with. Report this at https://github.com/nself-org/cli/issues\n" >&2
    return 1
  fi
}
