#!/usr/bin/env bash
# plugin_config.sh - nself plugin config <name> [get|set|show|reset] [key] [value]
# Interactive and scriptable plugin environment variable management
# Dispatched from plugin.sh as: cmd_config "$@"
# Bash 3.2+ compatible

set -euo pipefail

CLI_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

source "$CLI_SCRIPT_DIR/../lib/utils/display.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/utils/platform-compat.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/plugin/core.sh" 2>/dev/null || true

if ! declare -f log_info >/dev/null 2>&1; then
  log_info()    { printf "\033[0;34m[INFO]\033[0m %s\n" "$1"; }
  log_error()   { printf "\033[0;31m[ERROR]\033[0m %s\n" "$1" >&2; }
  log_success() { printf "\033[0;32m[SUCCESS]\033[0m %s\n" "$1"; }
  log_warning() { printf "\033[0;33m[WARNING]\033[0m %s\n" "$1"; }
fi

PLUGIN_DIR="${NSELF_PLUGIN_DIR:-$HOME/.nself/plugins}"

# ============================================================================
# HELPERS
# ============================================================================

# List of known secret patterns (mask when displaying)
_is_secret_key() {
  local key="$1"
  local lower_key
  lower_key=$(printf '%s' "$key" | tr '[:upper:]' '[:lower:]')
  case "$lower_key" in
    *key*|*secret*|*token*|*password*|*passwd*|*api_key*|*apikey*) return 0 ;;
  esac
  return 1
}

_mask_value() {
  local val="$1"
  if [[ -z "$val" ]]; then
    printf "(not set)"
    return
  fi
  local len="${#val}"
  if [[ $len -le 4 ]]; then
    printf "****"
  else
    printf "%s****" "${val:0:4}"
  fi
}

# Read a single key from config.env (or .env.local as fallback)
_read_config_key() {
  local plugin_dir="$1"
  local key="$2"
  local config_file="$plugin_dir/config.env"
  local env_local=".env.local"

  local val=""

  # Check config.env first
  if [[ -f "$config_file" ]]; then
    val=$(grep "^${key}=" "$config_file" 2>/dev/null | tail -1 | cut -d'=' -f2- | tr -d '"' | tr -d "'")
  fi

  # Fall back to .env.local
  if [[ -z "$val" ]] && [[ -f "$env_local" ]]; then
    val=$(grep "^${key}=" "$env_local" 2>/dev/null | tail -1 | cut -d'=' -f2- | tr -d '"' | tr -d "'")
  fi

  printf '%s' "$val"
}

# Write a key=value to config.env (creating or updating)
_write_config_key() {
  local plugin_dir="$1"
  local key="$2"
  local value="$3"
  local config_file="$plugin_dir/config.env"

  mkdir -p "$plugin_dir"

  if [[ -f "$config_file" ]] && grep -q "^${key}=" "$config_file" 2>/dev/null; then
    # Update existing key using platform-safe sed
    if declare -f safe_sed_inline >/dev/null 2>&1; then
      safe_sed_inline "s|^${key}=.*|${key}=${value}|" "$config_file"
    else
      # Portable fallback: rewrite file
      local tmp_file="${config_file}.tmp.$$"
      while IFS= read -r line; do
        case "$line" in
          "${key}="*) printf '%s=%s\n' "$key" "$value" ;;
          *) printf '%s\n' "$line" ;;
        esac
      done < "$config_file" > "$tmp_file"
      mv "$tmp_file" "$config_file"
    fi
  else
    # Append new key
    printf '%s=%s\n' "$key" "$value" >> "$config_file"
  fi
}

# Validate a value (basic checks)
_validate_value() {
  local key="$1"
  local value="$2"
  local lower_key
  lower_key=$(printf '%s' "$key" | tr '[:upper:]' '[:lower:]')

  if [[ -z "$value" ]]; then
    log_warning "Value for $key is empty"
    return 1
  fi

  case "$lower_key" in
    *url*|*endpoint*)
      if ! printf '%s' "$value" | grep -qE '^https?://'; then
        log_warning "$key should be a URL starting with http:// or https://"
      fi
      ;;
    *key*|*secret*|*token*)
      if [[ "${#value}" -lt 8 ]]; then
        log_warning "$key is very short — ensure this is intentional"
      fi
      ;;
  esac
  return 0
}

# ============================================================================
# SUBCOMMANDS
# ============================================================================

# Show all config vars (current values, masked)
_cmd_show() {
  local plugin_name="$1"
  local plugin_dir="$PLUGIN_DIR/$plugin_name"
  local config_file="$plugin_dir/config.env"
  local manifest="$plugin_dir/plugin.json"

  printf "\n\033[1mPlugin config: %s\033[0m\n\n" "$plugin_name"
  printf "  Config file: %s\n\n" "$config_file"

  # Show required vars from manifest
  local required_vars=""
  if command -v jq >/dev/null 2>&1; then
    required_vars=$(jq -r '.env.required // {} | keys[]' "$manifest" 2>/dev/null || \
      jq -r '.env_vars[]?.name // empty' "$manifest" 2>/dev/null || true)
  else
    required_vars=$(grep -A2 '"required"' "$manifest" 2>/dev/null | grep -o '"[A-Z_][A-Z0-9_]*"' | tr -d '"' || true)
  fi

  if [[ -n "${required_vars:-}" ]]; then
    printf "  %-40s  %s\n" "KEY" "VALUE"
    printf "  %-40s  %s\n" "---" "-----"
    while IFS= read -r var; do
      [[ -z "$var" ]] && continue
      local current_val
      current_val=$(_read_config_key "$plugin_dir" "$var")
      local display_val
      if _is_secret_key "$var"; then
        display_val=$(_mask_value "$current_val")
      else
        display_val="${current_val:-(not set)}"
      fi
      printf "  %-40s  %s\n" "$var" "$display_val"
    done <<EOF
$required_vars
EOF
  fi

  # Also show anything currently in config.env that isn't in manifest
  if [[ -f "$config_file" ]]; then
    printf "\n  \033[2m(additional values in config.env)\033[0m\n"
    while IFS= read -r line; do
      [[ -z "$line" ]] && continue
      [[ "$line" == "#"* ]] && continue
      local var="${line%%=*}"
      local val="${line#*=}"
      if _is_secret_key "$var"; then
        val=$(_mask_value "$val")
      fi
      printf "  %-40s  %s\n" "$var" "$val"
    done < "$config_file"
  fi

  printf "\n"
  printf "  Set a value : nself plugin config %s set KEY value\n" "$plugin_name"
  printf "  Interactive : nself plugin config %s\n\n" "$plugin_name"
}

# Get a single key
_cmd_get() {
  local plugin_name="$1"
  local key="$2"
  local plugin_dir="$PLUGIN_DIR/$plugin_name"

  if [[ -z "$key" ]]; then
    _cmd_show "$plugin_name"
    return 0
  fi

  local val
  val=$(_read_config_key "$plugin_dir" "$key")

  if [[ -z "$val" ]]; then
    log_warning "$key is not set"
    return 1
  fi

  if _is_secret_key "$key"; then
    printf "%s\n" "$(_mask_value "$val")"
  else
    printf "%s\n" "$val"
  fi
}

# Set a single key
_cmd_set() {
  local plugin_name="$1"
  local key="$2"
  local value="$3"
  local plugin_dir="$PLUGIN_DIR/$plugin_name"

  if [[ -z "$key" ]]; then
    log_error "Key name required"
    printf "Usage: nself plugin config %s set KEY value\n" "$plugin_name"
    return 1
  fi

  if [[ -z "$value" ]]; then
    log_error "Value required"
    printf "Usage: nself plugin config %s set KEY value\n" "$plugin_name"
    return 1
  fi

  # Validate key format
  if ! printf '%s' "$key" | grep -qE '^[A-Za-z_][A-Za-z0-9_]*$'; then
    log_error "Invalid key name: $key (must be alphanumeric + underscore)"
    return 1
  fi

  _validate_value "$key" "$value" || true

  _write_config_key "$plugin_dir" "$key" "$value"
  log_success "Set $key for plugin '$plugin_name'"
  printf "  Apply with: nself build && nself restart\n"
}

# Reset all plugin-specific config vars
_cmd_reset() {
  local plugin_name="$1"
  local plugin_dir="$PLUGIN_DIR/$plugin_name"
  local config_file="$plugin_dir/config.env"

  if [[ ! -f "$config_file" ]]; then
    log_info "No config.env found for plugin '$plugin_name' — nothing to reset"
    return 0
  fi

  printf "Reset all config for plugin '%s'? This cannot be undone. [y/N] " "$plugin_name"
  local answer
  read -r answer
  case "$answer" in
    y|Y|yes|YES)
      rm -f "$config_file"
      log_success "Config reset for plugin '$plugin_name'"
      ;;
    *)
      log_info "Reset cancelled"
      ;;
  esac
}

# Interactive prompt for all required vars
_cmd_interactive() {
  local plugin_name="$1"
  local plugin_dir="$PLUGIN_DIR/$plugin_name"
  local manifest="$plugin_dir/plugin.json"

  printf "\n\033[1mConfiguring plugin: %s\033[0m\n\n" "$plugin_name"

  local required_vars=""
  if command -v jq >/dev/null 2>&1; then
    required_vars=$(jq -r '.env.required // {} | keys[]' "$manifest" 2>/dev/null || \
      jq -r '.env_vars[]?.name // empty' "$manifest" 2>/dev/null || true)
  else
    required_vars=$(grep -A2 '"required"' "$manifest" 2>/dev/null | grep -o '"[A-Z_][A-Z0-9_]*"' | tr -d '"' || true)
  fi

  if [[ -z "${required_vars:-}" ]]; then
    log_info "No required variables defined in plugin manifest"
    return 0
  fi

  local set_count=0
  while IFS= read -r var; do
    [[ -z "$var" ]] && continue

    # Get description from manifest
    local desc=""
    if command -v jq >/dev/null 2>&1; then
      desc=$(jq -r ".env.required.\"${var}\".description // empty" "$manifest" 2>/dev/null || true)
    fi

    local current_val
    current_val=$(_read_config_key "$plugin_dir" "$var")

    # Show prompt
    printf "  %s" "$var"
    [[ -n "$desc" ]] && printf "\n    %s" "$desc"
    if [[ -n "$current_val" ]]; then
      if _is_secret_key "$var"; then
        printf "\n    Current: %s" "$(_mask_value "$current_val")"
      else
        printf "\n    Current: %s" "$current_val"
      fi
    fi

    local prompt_suffix="? "
    if [[ -n "$current_val" ]]; then
      prompt_suffix=" [press Enter to keep current]: "
    fi

    printf "\n    Value%s" "$prompt_suffix"

    local new_val=""
    if _is_secret_key "$var"; then
      # Read without echo for secrets
      # stty may not be available in all contexts
      if command -v stty >/dev/null 2>&1; then
        stty -echo 2>/dev/null || true
        read -r new_val
        stty echo 2>/dev/null || true
        printf "\n"
      else
        read -r new_val
      fi
    else
      read -r new_val
    fi

    if [[ -z "$new_val" ]] && [[ -n "$current_val" ]]; then
      log_info "  Keeping current value for $var"
      continue
    fi

    if [[ -n "$new_val" ]]; then
      _validate_value "$var" "$new_val" || true
      _write_config_key "$plugin_dir" "$var" "$new_val"
      log_success "  Set $var"
      set_count=$((set_count + 1))
    fi
  done <<EOF
$required_vars
EOF

  printf "\n"
  if [[ $set_count -gt 0 ]]; then
    log_success "Updated $set_count variable(s) for plugin '$plugin_name'"
    printf "  Apply with: nself build && nself restart\n"
  else
    log_info "No changes made"
  fi
  printf "\n"
}

# ============================================================================
# MAIN COMMAND
# ============================================================================

cmd_config() {
  local plugin_name=""
  local subcommand=""
  local key=""
  local value=""

  # Parse first arg as plugin name
  if [[ $# -gt 0 ]] && [[ "$1" != -* ]]; then
    plugin_name="$1"
    shift
  fi

  if [[ -z "$plugin_name" ]]; then
    log_error "Plugin name required"
    printf "\nUsage: nself plugin config <name> [get|set|show|reset] [key] [value]\n"
    return 1
  fi

  # Validate plugin is installed
  if ! declare -f is_plugin_installed >/dev/null 2>&1 || ! is_plugin_installed "$plugin_name"; then
    if [[ ! -d "$PLUGIN_DIR/$plugin_name" ]]; then
      log_error "Plugin '$plugin_name' is not installed"
      return 1
    fi
  fi

  # Parse subcommand
  if [[ $# -gt 0 ]] && [[ "$1" != -* ]]; then
    subcommand="$1"
    shift
  fi

  # Parse key + value
  if [[ $# -gt 0 ]] && [[ "$1" != -* ]]; then
    key="$1"
    shift
  fi
  if [[ $# -gt 0 ]]; then
    value="$1"
    shift
  fi

  # Handle --show flag
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --show)
        subcommand="show"
        shift
        ;;
      --reset)
        subcommand="reset"
        shift
        ;;
      -h|--help)
        printf "Usage: nself plugin config <name> [subcommand] [key] [value]\n\n"
        printf "Subcommands:\n"
        printf "  (none)       Interactive prompt for all required vars\n"
        printf "  get [key]    Get value of a key (or show all if no key)\n"
        printf "  set key val  Set a key to a value\n"
        printf "  show         Show all current values (secrets masked)\n"
        printf "  reset        Clear all plugin config vars\n\n"
        printf "Flags:\n"
        printf "  --show       Alias for 'show' subcommand\n"
        printf "  --reset      Alias for 'reset' subcommand\n"
        return 0
        ;;
      *)
        shift
        ;;
    esac
  done

  case "${subcommand:-}" in
    get)
      _cmd_get "$plugin_name" "$key"
      ;;
    set)
      _cmd_set "$plugin_name" "$key" "$value"
      ;;
    show)
      _cmd_show "$plugin_name"
      ;;
    reset)
      _cmd_reset "$plugin_name"
      ;;
    "")
      # Interactive mode
      _cmd_interactive "$plugin_name"
      ;;
    *)
      log_error "Unknown subcommand: $subcommand"
      printf "Valid subcommands: get, set, show, reset\n"
      return 1
      ;;
  esac
}

# ── Standalone invocation ────────────────────────────────────────────────────
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  cmd_config "$@"
fi
