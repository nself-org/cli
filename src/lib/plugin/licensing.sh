#!/usr/bin/env bash
# licensing.sh — Plugin license validation
#
# Validates nself Pro Plugins licenses before paid plugin installation.
# Bash 3.2 compatible (macOS default shell).
#
# Usage (sourced by plugin.sh):
#   license_check_entitlement <plugin_name>   → 0=allowed, 1=denied
#
# Environment:
#   NSELF_PLUGIN_LICENSE_KEY  License key (nself_pro_xxxx...)
#   NSELF_API_URL             Override API base URL (default: https://api.nself.org)
#   NSELF_LICENSE_SKIP_VERIFY Set to 1 to skip remote validation (offline mode)

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

NSELF_LICENSE_CACHE_DIR="${HOME}/.nself/license"
NSELF_LICENSE_CACHE_FILE="${NSELF_LICENSE_CACHE_DIR}/cache"
NSELF_LICENSE_KEY_FILE="${NSELF_LICENSE_CACHE_DIR}/key"
NSELF_LICENSE_CACHE_TTL=86400  # 24 hours
NSELF_LICENSE_KEY_PREFIX="nself_pro_"
NSELF_PING_URL="${NSELF_PING_API_URL:-https://ping.nself.org}"
NSELF_LICENSE_VALIDATE_ENDPOINT="${NSELF_PING_URL}/license/validate"
NSELF_PRICING_URL="https://nself.org/commercial"

# ---------------------------------------------------------------------------
# Paid plugin registry
# Space-separated list of plugin names that require a Pro license.
# Mirrors the plugins-pro/paid/ directory.
# ---------------------------------------------------------------------------

NSELF_PRO_PLUGINS="access-controls activity-feed admin-api ai analytics auth backup bots calendar cdn chat cloudflare cms compliance content-progress devices documents donorbox entitlements epg feature-flags-pro file-processing game-metadata geocoding geolocation idme knowledge-base livekit media-processing meetings moderation object-storage observability paypal photos podcast realtime recording retro-gaming rom-discovery search-pro shopify social sports stream-gateway streaming stripe support tmdb tokens-pro torrent-manager-pro vpn-pro web3 webhooks-pro workflows"

# ---------------------------------------------------------------------------
# Internal helpers
# ---------------------------------------------------------------------------

# _license_now — print current Unix timestamp (Bash 3.2 compatible)
_license_now() {
  date +%s
}

# _license_log — print to stderr using printf (portable)
_license_log() {
  printf '%s\n' "$1" >&2
}

# _license_in_list <item> <space-separated-list>
# Returns 0 if item is in list, 1 otherwise
_license_in_list() {
  local item="$1"
  local list="$2"
  local entry
  for entry in $list; do
    if [ "$entry" = "$item" ]; then
      return 0
    fi
  done
  return 1
}

# ---------------------------------------------------------------------------
# Public: license_get_key
# Returns the active license key — env var takes precedence over saved file.
# Prints the key to stdout; returns 1 if no key is configured.
# ---------------------------------------------------------------------------

license_get_key() {
  if [ -n "${NSELF_PLUGIN_LICENSE_KEY:-}" ]; then
    printf '%s' "$NSELF_PLUGIN_LICENSE_KEY"
    return 0
  fi
  if [ -f "$NSELF_LICENSE_KEY_FILE" ]; then
    local key
    key=$(tr -d '[:space:]' < "$NSELF_LICENSE_KEY_FILE" 2>/dev/null)
    if [ -n "$key" ]; then
      printf '%s' "$key"
      return 0
    fi
  fi
  return 1
}

# ---------------------------------------------------------------------------
# Public: license_save_key <key>
# Saves license key to ~/.nself/license/key for persistent storage.
# ---------------------------------------------------------------------------

license_save_key() {
  local key="$1"
  mkdir -p "$NSELF_LICENSE_CACHE_DIR" 2>/dev/null
  printf '%s\n' "$key" > "$NSELF_LICENSE_KEY_FILE"
  chmod 600 "$NSELF_LICENSE_KEY_FILE" 2>/dev/null
  # Invalidate cache so the new key is validated fresh
  rm -f "$NSELF_LICENSE_CACHE_FILE" 2>/dev/null
}

# ---------------------------------------------------------------------------
# Public: license_clear_key
# Removes saved license key file and validation cache.
# ---------------------------------------------------------------------------

license_clear_key() {
  rm -f "$NSELF_LICENSE_KEY_FILE" "$NSELF_LICENSE_CACHE_FILE" 2>/dev/null
}

# ---------------------------------------------------------------------------
# Public: license_is_paid_plugin <plugin_name>
# Returns 0 if the plugin requires a Pro license, 1 if free.
# ---------------------------------------------------------------------------

license_is_paid_plugin() {
  local plugin_name="$1"
  if [ -z "$plugin_name" ]; then
    return 1
  fi
  _license_in_list "$plugin_name" "$NSELF_PRO_PLUGINS"
}

# ---------------------------------------------------------------------------
# Public: license_validate_format <license_key>
# Offline format check — key must start with nself_pro_ or nself_ent_ and be >=32 chars.
# Returns 0 if valid format, 1 otherwise.
# ---------------------------------------------------------------------------

license_validate_format() {
  local key="$1"
  if [ -z "$key" ]; then
    return 1
  fi
  # Must start with nself_pro_ (Pro) or nself_ent_ (Elite)
  case "$key" in
    nself_pro_*|nself_ent_*)
      # Must be at least 32 characters total
      if [ ${#key} -ge 32 ]; then
        return 0
      fi
      ;;
  esac
  return 1
}

# ---------------------------------------------------------------------------
# Public: license_validate_remote <license_key>
# Calls web backend to validate key. Writes result to cache.
# Returns 0 if valid, 1 if invalid or API unreachable.
# ---------------------------------------------------------------------------

license_validate_remote() {
  local key="$1"

  # Skip remote validation if offline mode set
  if [ "${NSELF_LICENSE_SKIP_VERIFY:-0}" = "1" ]; then
    _license_log "License: remote validation skipped (NSELF_LICENSE_SKIP_VERIFY=1)"
    return 0
  fi

  # Require curl
  if ! command -v curl >/dev/null 2>&1; then
    _license_log "License: curl not found — cannot validate remotely"
    return 1
  fi

  local response
  local http_status

  # POST to validate endpoint (5s timeout)
  response=$(curl -s -w '\n%{http_code}' \
    --max-time 5 \
    --connect-timeout 3 \
    -X POST \
    -H "Content-Type: application/json" \
    -d "{\"license_key\":\"${key}\",\"product\":\"plugins-pro\"}" \
    "${NSELF_LICENSE_VALIDATE_ENDPOINT}" 2>/dev/null)

  http_status=$(printf '%s' "$response" | tail -1)
  local body
  body=$(printf '%s' "$response" | head -1)

  case "$http_status" in
    200)
      # Valid — cache the result
      license_cache_write "$key" "valid"
      return 0
      ;;
    401|403)
      # Invalid or expired key
      license_cache_write "$key" "invalid"
      return 1
      ;;
    404)
      # Key not found
      license_cache_write "$key" "invalid"
      return 1
      ;;
    *)
      # API unreachable or error — fail open with warning
      _license_log "License: could not reach validation server (HTTP ${http_status:-unreachable})"
      _license_log "License: proceeding with format-only validation"
      return 0
      ;;
  esac
}

# ---------------------------------------------------------------------------
# Public: license_cache_write <license_key> <status>
# Writes cache entry: key hash + status + timestamp
# ---------------------------------------------------------------------------

license_cache_write() {
  local key="$1"
  local status="$2"  # "valid" or "invalid"
  local now
  now=$(_license_now)

  mkdir -p "$NSELF_LICENSE_CACHE_DIR" 2>/dev/null
  # Store: status|timestamp (key is stored as truncated prefix for reference)
  local key_prefix
  key_prefix=$(printf '%s' "$key" | cut -c1-24)
  printf '%s|%s|%s\n' "$key_prefix" "$status" "$now" > "$NSELF_LICENSE_CACHE_FILE"
  chmod 600 "$NSELF_LICENSE_CACHE_FILE" 2>/dev/null
}

# ---------------------------------------------------------------------------
# Public: license_cache_read <license_key>
# Reads cache and checks TTL. Prints "valid", "invalid", or "expired".
# ---------------------------------------------------------------------------

license_cache_read() {
  local key="$1"
  if [ ! -f "$NSELF_LICENSE_CACHE_FILE" ]; then
    printf 'expired\n'
    return 1
  fi

  local line
  line=$(cat "$NSELF_LICENSE_CACHE_FILE" 2>/dev/null)
  if [ -z "$line" ]; then
    printf 'expired\n'
    return 1
  fi

  local cached_prefix
  local cached_status
  local cached_time
  cached_prefix=$(printf '%s' "$line" | cut -d'|' -f1)
  cached_status=$(printf '%s' "$line" | cut -d'|' -f2)
  cached_time=$(printf '%s' "$line" | cut -d'|' -f3)

  # Verify key prefix matches
  local key_prefix
  key_prefix=$(printf '%s' "$key" | cut -c1-24)
  if [ "$cached_prefix" != "$key_prefix" ]; then
    # Different key — cache invalid
    printf 'expired\n'
    return 1
  fi

  # Check TTL
  local now
  now=$(_license_now)
  local age
  age=$((now - cached_time))
  if [ "$age" -gt "$NSELF_LICENSE_CACHE_TTL" ]; then
    printf 'expired\n'
    return 1
  fi

  printf '%s\n' "$cached_status"
  return 0
}

# ---------------------------------------------------------------------------
# Public: license_check_entitlement <plugin_name>
# Main entry point. Called by plugin install before download.
# Returns 0 if installation is allowed, 1 if denied.
# ---------------------------------------------------------------------------

license_check_entitlement() {
  local plugin_name="$1"

  # Free plugin — always allowed
  if ! license_is_paid_plugin "$plugin_name"; then
    return 0
  fi

  # Paid plugin — check for license key (env var or saved key file)
  local license_key
  license_key=$(license_get_key) || true
  if [ -z "$license_key" ]; then
    printf '\n'
    printf '  %s requires a Pro Plugins license.\n' "$plugin_name"
    printf '\n'
    printf '  License:  %s\n' "$NSELF_PRICING_URL"
    printf '  Price:    $9.99/year — covers all 49 Pro Plugins\n'
    printf '\n'
    printf '  Save your key:  nself plugin license set nself_pro_xxxx...\n'
    printf '  Or add to .env: NSELF_PLUGIN_LICENSE_KEY=nself_pro_xxxx...\n'
    printf '\n'
    printf '  Alternatively, implement this plugin as a Custom Service:\n'
    printf '    https://docs.nself.org/custom-services\n'
    printf '\n'
    return 1
  fi

  # Validate key format
  if ! license_validate_format "$license_key"; then
    printf '\n'
    printf '  Invalid license key format.\n'
    printf '  Key must start with "%s" and be at least 32 characters.\n' "$NSELF_LICENSE_KEY_PREFIX"
    printf '  Get a key at: %s\n' "$NSELF_PRICING_URL"
    printf '\n'
    return 1
  fi

  # Check cache first
  local cached_status
  cached_status=$(license_cache_read "$license_key")

  case "$cached_status" in
    valid)
      return 0
      ;;
    invalid)
      printf '\n'
      printf '  License key is invalid or expired.\n'
      printf '  Renew at: %s\n' "$NSELF_PRICING_URL"
      printf '\n'
      return 1
      ;;
    expired|*)
      # Cache expired or missing — validate remotely
      if license_validate_remote "$license_key"; then
        return 0
      else
        printf '\n'
        printf '  License validation failed for plugin: %s\n' "$plugin_name"
        printf '  Check or renew your license at: %s\n' "$NSELF_PRICING_URL"
        printf '\n'
        return 1
      fi
      ;;
  esac
}

# ---------------------------------------------------------------------------
# Public: license_show_status
# Prints current license status for `nself plugin license` command.
# ---------------------------------------------------------------------------

license_show_status() {
  local license_key
  license_key=$(license_get_key) || true

  local key_source="env"
  if [ -z "${NSELF_PLUGIN_LICENSE_KEY:-}" ] && [ -n "$license_key" ]; then
    key_source="saved"
  fi

  printf '\n  Pro Plugins License Status\n'
  printf '  --------------------------\n'

  if [ -z "$license_key" ]; then
    printf '  Status:   No license key configured\n'
    printf '  Set key:  nself plugin license set nself_pro_xxxx...\n'
    printf '  Get one:  %s\n' "$NSELF_PRICING_URL"
    printf '\n'
    return 0
  fi

  if ! license_validate_format "$license_key"; then
    printf '  Status: Invalid key format\n'
    printf '  Key prefix: %s...\n' "$(printf '%s' "$license_key" | cut -c1-16)"
    printf '\n'
    return 1
  fi

  local key_display
  key_display=$(printf '%s' "$license_key" | cut -c1-20)
  printf '  Source:   %s\n' "$key_source"
  printf '  Key: %s...\n' "$key_display"

  local cached_status
  cached_status=$(license_cache_read "$license_key")

  case "$cached_status" in
    valid)
      printf '  Status: Valid (cached)\n'
      ;;
    invalid)
      printf '  Status: Invalid or expired\n'
      printf '  Renew: %s\n' "$NSELF_PRICING_URL"
      ;;
    expired)
      printf '  Status: Checking...\n'
      if license_validate_remote "$license_key" 2>/dev/null; then
        printf '  Status: Valid\n'
      else
        printf '  Status: Invalid or expired\n'
        printf '  Renew: %s\n' "$NSELF_PRICING_URL"
      fi
      ;;
  esac

  printf '\n  Pro Plugins covered: 49 plugins\n'
  printf '  Details: %s\n\n' "$NSELF_PRICING_URL"
}
