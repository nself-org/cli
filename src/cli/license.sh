#!/usr/bin/env bash
# license.sh — nself license management
# Subcommands: set, show, validate, clear, upgrade
#
# Bash 3.2 compatible. No echo -e, no ${var,,}, no declare -A.

set -o pipefail

CLI_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

source "$CLI_SCRIPT_DIR/../lib/utils/display.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/plugin/licensing.sh" 2>/dev/null || true

# Fallback display helpers
if ! declare -f log_success >/dev/null 2>&1; then
  log_success() { printf "\033[0;32m[SUCCESS]\033[0m %s\n" "$1"; }
fi
if ! declare -f log_error >/dev/null 2>&1; then
  log_error() { printf "\033[0;31m[ERROR]\033[0m %s\n" "$1" >&2; }
fi
if ! declare -f log_info >/dev/null 2>&1; then
  log_info() { printf "\033[0;34m[INFO]\033[0m %s\n" "$1"; }
fi
if ! declare -f log_warning >/dev/null 2>&1; then
  log_warning() { printf "\033[0;33m[WARNING]\033[0m %s\n" "$1"; }
fi

NSELF_PING_URL="${NSELF_PING_API_URL:-https://ping.nself.org}"
NSELF_PRICING_URL="https://nself.org/pricing"
NSELF_LICENSE_KEY_FILE="${HOME}/.nself/license/key"

# ---------------------------------------------------------------------------
# _license_mask_key <key>
# Prints first 12 chars + "****" + last 4 chars (Bash 3.2 safe)
# ---------------------------------------------------------------------------
_license_mask_key() {
  local key="$1"
  local len
  len=${#key}
  if [ "$len" -le 16 ]; then
    printf '%s****\n' "$(printf '%s' "$key" | cut -c1-4)"
    return 0
  fi
  local prefix
  local suffix
  prefix=$(printf '%s' "$key" | cut -c1-12)
  suffix=$(printf '%s' "$key" | rev | cut -c1-4 | rev)
  printf '%s****%s\n' "$prefix" "$suffix"
}

# ---------------------------------------------------------------------------
# cmd_set <key>
# Save a license key to disk.
# ---------------------------------------------------------------------------
cmd_set() {
  case "${1:-}" in
    --help|-h)
      cmd_help
      return 0
      ;;
  esac
  local key="${1:-}"
  if [ -z "$key" ]; then
    log_error "Usage: nself license set <key>"
    return 1
  fi

  # Basic format validation
  case "$key" in
    nself_pro_*|nself_max_*|nself_ent_*|nself_owner_*)
      if [ ${#key} -lt 32 ]; then
        log_error "Invalid license key format (too short)"
        return 1
      fi
      ;;
    *)
      log_error "Invalid license key format (must start with nself_pro_, nself_max_, etc.)"
      return 1
      ;;
  esac

  mkdir -p "$(dirname "$NSELF_LICENSE_KEY_FILE")" 2>/dev/null
  printf '%s\n' "$key" > "$NSELF_LICENSE_KEY_FILE"
  chmod 600 "$NSELF_LICENSE_KEY_FILE" 2>/dev/null

  local masked
  masked=$(_license_mask_key "$key")
  log_success "License key saved: $masked"
  printf '\n'
  printf '  Run: nself license validate   — to verify the key against the server\n'
  printf '  Run: nself plugin install ai  — to install a paid plugin\n'
}

# ---------------------------------------------------------------------------
# cmd_show
# Display the saved license key (masked) and tier info.
# ---------------------------------------------------------------------------
cmd_show() {
  local key=""
  if [ -n "${NSELF_PLUGIN_LICENSE_KEY:-}" ]; then
    key="$NSELF_PLUGIN_LICENSE_KEY"
  elif [ -f "$NSELF_LICENSE_KEY_FILE" ]; then
    key=$(tr -d '[:space:]' < "$NSELF_LICENSE_KEY_FILE" 2>/dev/null)
  fi

  if [ -z "$key" ]; then
    log_warning "No license key configured."
    printf '\n'
    printf '  Set one with: nself license set <key>\n'
    printf '  Get a license at: %s\n' "$NSELF_PRICING_URL"
    return 0
  fi

  local masked
  masked=$(_license_mask_key "$key")
  printf '\n'
  printf '  License key:  %s\n' "$masked"

  # Try to read tier from local cache if available
  local cache_file="${HOME}/.nself/license/cache"
  if [ -f "$cache_file" ]; then
    local cached_tier
    cached_tier=$(grep -m1 '^tier=' "$cache_file" 2>/dev/null | cut -d'=' -f2)
    if [ -n "$cached_tier" ]; then
      printf '  Tier:         %s\n' "$cached_tier"
    fi
    local cached_renewal
    cached_renewal=$(grep -m1 '^renewal_date=' "$cache_file" 2>/dev/null | cut -d'=' -f2)
    if [ -n "$cached_renewal" ]; then
      printf '  Renewal date: %s\n' "$cached_renewal"
    fi
  fi

  printf '\n'
  printf '  Run: nself license validate   — to check status with the server\n'
}

# ---------------------------------------------------------------------------
# cmd_validate
# Call ping_api /license/validate and display tier + entitlements.
# ---------------------------------------------------------------------------
cmd_validate() {
  case "${1:-}" in
    --help|-h)
      cmd_help
      return 0
      ;;
  esac
  local key=""
  if [ -n "${NSELF_PLUGIN_LICENSE_KEY:-}" ]; then
    key="$NSELF_PLUGIN_LICENSE_KEY"
  elif [ -f "$NSELF_LICENSE_KEY_FILE" ]; then
    key=$(tr -d '[:space:]' < "$NSELF_LICENSE_KEY_FILE" 2>/dev/null)
  fi

  if [ -z "$key" ]; then
    log_error "No license key configured. Run: nself license set <key>"
    return 1
  fi

  if ! command -v curl >/dev/null 2>&1; then
    log_error "curl is required for license validation"
    return 1
  fi

  log_info "Validating license with server..."

  local response http_status body
  response=$(curl -s -w '\n%{http_code}' \
    --max-time 10 \
    --connect-timeout 5 \
    -X POST \
    -H "Content-Type: application/json" \
    -d "{\"license_key\":\"${key}\"}" \
    "${NSELF_PING_URL}/license/validate" 2>/dev/null)

  http_status=$(printf '%s' "$response" | tail -1)
  body=$(printf '%s' "$response" | head -1)

  if [ "$http_status" != "200" ]; then
    log_error "Server returned HTTP $http_status"
    if [ -n "$body" ]; then
      printf '  Response: %s\n' "$body"
    fi
    return 1
  fi

  # Parse key fields from JSON using basic shell (no jq dependency)
  local valid tier renewal_date upgrade_required
  valid=$(printf '%s' "$body" | grep -o '"valid":[^,}]*' | cut -d':' -f2 | tr -d ' "')
  tier=$(printf '%s' "$body" | grep -o '"tier":"[^"]*"' | head -1 | cut -d'"' -f4)
  renewal_date=$(printf '%s' "$body" | grep -o '"renewal_date":"[^"]*"' | cut -d'"' -f4)
  upgrade_required=$(printf '%s' "$body" | grep -o '"upgrade_required":[^,}]*' | cut -d':' -f2 | tr -d ' "')

  if [ "$valid" != "true" ]; then
    local err_msg
    err_msg=$(printf '%s' "$body" | grep -o '"error":"[^"]*"' | cut -d'"' -f4)
    log_error "License invalid: ${err_msg:-unknown error}"
    return 1
  fi

  local masked
  masked=$(_license_mask_key "$key")

  log_success "License is valid"
  printf '\n'
  printf '  Key:          %s\n' "$masked"
  printf '  Tier:         %s\n' "${tier:-unknown}"
  if [ -n "$renewal_date" ]; then
    printf '  Renewal:      %s\n' "$renewal_date"
  fi
  if [ "$upgrade_required" = "true" ]; then
    printf '\n'
    log_warning "Some plugins require Max tier."
    printf '  Upgrade at: %s\n' "$NSELF_PRICING_URL"
  fi
  printf '\n'

  # Cache the tier locally for cmd_show
  local cache_dir="${HOME}/.nself/license"
  mkdir -p "$cache_dir" 2>/dev/null
  {
    printf 'tier=%s\n' "${tier:-unknown}"
    if [ -n "$renewal_date" ]; then
      printf 'renewal_date=%s\n' "$renewal_date"
    fi
  } > "${cache_dir}/cache"
  chmod 600 "${cache_dir}/cache" 2>/dev/null
}

# ---------------------------------------------------------------------------
# cmd_clear
# Remove saved license key and cache.
# ---------------------------------------------------------------------------
cmd_clear() {
  case "${1:-}" in
    --help|-h)
      cmd_help
      return 0
      ;;
  esac
  # Safe removal: verify the parent dir is actually a directory before trying to
  # remove files inside it. On macOS (case-insensitive FS), ~/.nself/license may
  # resolve to ~/.nself/LICENSE (the MIT licence text file), which is not a
  # directory — rm would fail with "Not a directory".
  local key_dir
  key_dir="$(dirname "$NSELF_LICENSE_KEY_FILE")"
  if [ -d "$key_dir" ]; then
    rm -f "$NSELF_LICENSE_KEY_FILE" "${key_dir}/cache" 2>/dev/null || true
  fi
  log_success "License key cleared."
}

# ---------------------------------------------------------------------------
# cmd_upgrade
# Open the upgrade URL in the system browser (or print it).
# ---------------------------------------------------------------------------
cmd_upgrade() {
  local upgrade_url="$NSELF_PRICING_URL"

  printf '\n'
  printf '  Upgrade to Max tier at:\n'
  printf '  %s\n\n' "$upgrade_url"

  # Try to open browser
  if command -v xdg-open >/dev/null 2>&1; then
    xdg-open "$upgrade_url" 2>/dev/null &
    log_info "Opening browser..."
  elif command -v open >/dev/null 2>&1; then
    open "$upgrade_url" 2>/dev/null &
    log_info "Opening browser..."
  else
    log_info "Open the URL above in your browser to upgrade."
  fi
}

# ---------------------------------------------------------------------------
# cmd_help
# ---------------------------------------------------------------------------
cmd_help() {
  cat <<'HELP'
nself license — Manage your nself plugin license

Usage:
  nself license set <key>    Save a license key
  nself license show         Display current key (masked) and tier
  nself license validate     Validate key against the server
  nself license clear        Remove saved license key
  nself license upgrade      Open the upgrade page in your browser

Environment:
  NSELF_PLUGIN_LICENSE_KEY   Override the saved key (env takes precedence)
  NSELF_PING_API_URL         Override ping API URL (default: https://ping.nself.org)

Examples:
  nself license set nself_pro_abc123...
  nself license validate
  nself license upgrade

Get a license at: https://nself.org/pricing
HELP
}

# ---------------------------------------------------------------------------
# Main dispatcher
# ---------------------------------------------------------------------------
main() {
  local subcmd="${1:-help}"
  shift 2>/dev/null || true

  case "$subcmd" in
    set)        cmd_set "$@" ;;
    show)       cmd_show "$@" ;;
    validate)   cmd_validate "$@" ;;
    clear)      cmd_clear "$@" ;;
    upgrade)    cmd_upgrade "$@" ;;
    --help|-h|help) cmd_help ;;
    *)
      log_error "Unknown subcommand: $subcmd"
      printf '\n'
      cmd_help
      return 1
      ;;
  esac
}

main "$@"
