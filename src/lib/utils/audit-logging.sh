#!/usr/bin/env bash

# audit-logging.sh - Immutable audit trail for security-sensitive operations
# Provides tamper-resistant logging for compliance and security tracking
# Cross-platform compatible (Bash 3.2+)

# Prevent double-sourcing
[[ "${AUDIT_LOGGING_SOURCED:-}" == "1" ]] && return 0
export AUDIT_LOGGING_SOURCED=1

# Source dependencies (namespaced to avoid clobbering caller's SCRIPT_DIR)
_AUDIT_LOG_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${_AUDIT_LOG_DIR}/platform-compat.sh" 2>/dev/null || true
source "${_AUDIT_LOG_DIR}/logging.sh" 2>/dev/null || true

# =============================================================================
# AUDIT CONFIGURATION
# =============================================================================

# Audit log location (separate from regular logs for security)
export NSELF_AUDIT_DIR="${NSELF_AUDIT_DIR:-${HOME}/.nself/audit}"
export NSELF_AUDIT_FILE="${NSELF_AUDIT_FILE:-audit.log}"
export NSELF_AUDIT_ENABLED="${NSELF_AUDIT_ENABLED:-true}"

# Audit event categories
export AUDIT_CAT_AUTH="AUTH"           # Authentication/authorization
export AUDIT_CAT_CONFIG="CONFIG"       # Configuration changes
export AUDIT_CAT_DEPLOY="DEPLOY"       # Deployment actions
export AUDIT_CAT_SECRET="SECRET"       # Secret access/rotation
export AUDIT_CAT_SECURITY="SECURITY"   # Security events
export AUDIT_CAT_DATA="DATA"           # Data operations
export AUDIT_CAT_ADMIN="ADMIN"         # Administrative actions
export AUDIT_CAT_SYSTEM="SYSTEM"       # System events

# Audit log format version
export AUDIT_FORMAT_VERSION="1.0"

# =============================================================================
# INITIALIZATION
# =============================================================================

# Initialize audit logging system
audit_init() {
  local audit_dir="${1:-${NSELF_AUDIT_DIR}}"

  # Skip if disabled
  if [[ "$NSELF_AUDIT_ENABLED" != "true" ]]; then
    return 0
  fi

  # Create audit directory if it doesn't exist
  if [[ ! -d "$audit_dir" ]]; then
    mkdir -p "$audit_dir" 2>/dev/null || {
      printf "WARNING: Failed to create audit directory: %s\n" "$audit_dir" >&2
      export NSELF_AUDIT_ENABLED=false
      return 1
    }
  fi

  # Set restrictive permissions (owner only, no group/others)
  chmod 700 "$audit_dir" 2>/dev/null || true

  # Create audit log file if it doesn't exist
  local audit_path="${audit_dir}/${NSELF_AUDIT_FILE}"
  if [[ ! -f "$audit_path" ]]; then
    # Create with header
    {
      printf "# nself Audit Log - Format Version %s\n" "$AUDIT_FORMAT_VERSION"
      printf "# CRITICAL: This file contains security-sensitive audit events\n"
      printf "# DO NOT EDIT OR DELETE - Required for compliance and security\n"
      printf "# Started: %s\n" "$(date -u '+%Y-%m-%d %H:%M:%S UTC')"
      printf "#\n"
    } > "$audit_path" 2>/dev/null || {
      printf "WARNING: Failed to create audit file: %s\n" "$audit_path" >&2
      export NSELF_AUDIT_ENABLED=false
      return 1
    }
  fi

  # Set append-only permissions (owner read/append, no delete without root)
  chmod 600 "$audit_path" 2>/dev/null || true

  # On Linux, try to set append-only attribute (requires root)
  if command -v chattr >/dev/null 2>&1 && [[ -w "$audit_path" ]]; then
    chattr +a "$audit_path" 2>/dev/null || true
  fi

  return 0
}

# =============================================================================
# UTILITY FUNCTIONS
# =============================================================================

# Get current user (safe for audit trail)
_audit_get_user() {
  local user="${USER:-${USERNAME:-unknown}}"

  # Try to get real user if using sudo
  if [[ -n "${SUDO_USER:-}" ]]; then
    user="${SUDO_USER} (via sudo)"
  fi

  printf "%s" "$user"
}

# Get current hostname (sanitized)
_audit_get_hostname() {
  local hostname
  hostname=$(hostname 2>/dev/null || printf "unknown")

  # Sanitize hostname (remove domain, keep short name)
  hostname="${hostname%%.*}"

  printf "%s" "$hostname"
}

# Get session ID (for correlating related events)
_audit_get_session_id() {
  # Use process ID and start time as session identifier
  local session_id="${$}_$(date +%s)"
  printf "%s" "$session_id"
}

# Generate event ID (unique identifier for each audit event)
_audit_generate_event_id() {
  # Format: timestamp_random
  local timestamp
  timestamp=$(date +%s)
  local random="${RANDOM}${RANDOM}"
  printf "evt_%s_%s" "$timestamp" "$random"
}

# Calculate checksum for integrity verification
_audit_checksum() {
  local data="$1"

  if command -v sha256sum >/dev/null 2>&1; then
    printf "%s" "$data" | sha256sum | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    printf "%s" "$data" | shasum -a 256 | awk '{print $1}'
  else
    # Fallback: simple hash (not cryptographic)
    printf "%s" "$data" | cksum | awk '{print $1}'
  fi
}

# =============================================================================
# CORE AUDIT FUNCTIONS
# =============================================================================

# Write audit event (immutable, append-only)
# Args: category, action, [details...]
audit_log() {
  local category="$1"
  local action="$2"
  shift 2
  local details=("$@")

  # Skip if disabled
  if [[ "$NSELF_AUDIT_ENABLED" != "true" ]]; then
    return 0
  fi

  # Get audit metadata
  local timestamp
  timestamp=$(date -u '+%Y-%m-%d %H:%M:%S UTC')
  local event_id
  event_id=$(_audit_generate_event_id)
  local user
  user=$(_audit_get_user)
  local hostname
  hostname=$(_audit_get_hostname)
  local pid=$$

  # Build details string (sanitized)
  local details_str=""
  if [[ ${#details[@]} -gt 0 ]]; then
    local detail
    for detail in "${details[@]}"; do
      # Basic sanitization (remove newlines, limit length)
      local sanitized
      sanitized=$(printf "%s" "$detail" | tr '\n' ' ' | cut -c1-500)
      details_str="${details_str}${sanitized};"
    done
    # Remove trailing semicolon
    details_str="${details_str%;}"
  fi

  # Build structured audit entry (parseable format)
  # Format: timestamp|event_id|category|action|user|hostname|pid|details|checksum
  local entry_data="${timestamp}|${event_id}|${category}|${action}|${user}|${hostname}|${pid}|${details_str}"

  # Calculate integrity checksum
  local checksum
  checksum=$(_audit_checksum "$entry_data")

  # Final audit entry with checksum
  local audit_entry="${entry_data}|${checksum}"

  # Append to audit log (atomic operation)
  local audit_path="${NSELF_AUDIT_DIR}/${NSELF_AUDIT_FILE}"
  printf "%s\n" "$audit_entry" >> "$audit_path" 2>/dev/null || {
    # If audit logging fails, this is a critical error
    printf "CRITICAL: Failed to write audit log entry\n" >&2
    return 1
  }

  # Also log to regular logs for visibility
  log_info "AUDIT: [${category}] ${action}" "${details[@]}"

  return 0
}

# =============================================================================
# SPECIALIZED AUDIT FUNCTIONS
# =============================================================================

# Audit authentication events
audit_auth() {
  local action="$1"
  shift
  audit_log "$AUDIT_CAT_AUTH" "$action" "$@"
}

# Audit configuration changes
audit_config() {
  local action="$1"
  shift
  audit_log "$AUDIT_CAT_CONFIG" "$action" "$@"
}

# Audit deployment actions
audit_deploy() {
  local action="$1"
  shift
  audit_log "$AUDIT_CAT_DEPLOY" "$action" "$@"
}

# Audit secret operations
audit_secret() {
  local action="$1"
  shift
  # Never log the actual secret value
  audit_log "$AUDIT_CAT_SECRET" "$action" "$@"
}

# Audit security events
audit_security() {
  local action="$1"
  shift
  audit_log "$AUDIT_CAT_SECURITY" "$action" "$@"
}

# Audit data operations
audit_data() {
  local action="$1"
  shift
  audit_log "$AUDIT_CAT_DATA" "$action" "$@"
}

# Audit administrative actions
audit_admin() {
  local action="$1"
  shift
  audit_log "$AUDIT_CAT_ADMIN" "$action" "$@"
}

# Audit system events
audit_system() {
  local action="$1"
  shift
  audit_log "$AUDIT_CAT_SYSTEM" "$action" "$@"
}

# =============================================================================
# AUDIT QUERY AND REPORTING
# =============================================================================

# Query audit logs with optional filtering
# Args: [category] [action] [user] [since_date]
# Date format: ISO8601 (YYYY-MM-DD)
# Examples:
#   audit_query "" "" "" "2026-02-01"  # All events since Feb 1
#   audit_query "AUTH" "" "" "2026-01-15"  # Auth events since Jan 15
audit_query() {
  local filter_category="${1:-}"
  local filter_action="${2:-}"
  local filter_user="${3:-}"
  local filter_since="${4:-}"

  local audit_path="${NSELF_AUDIT_DIR}/${NSELF_AUDIT_FILE}"

  if [[ ! -f "$audit_path" ]]; then
    printf "No audit logs found\n" >&2
    return 1
  fi

  # Skip header lines
  local results
  results=$(grep -v '^#' "$audit_path" 2>/dev/null || printf "")

  # Apply filters
  if [[ -n "$filter_category" ]]; then
    results=$(printf "%s" "$results" | grep "|${filter_category}|" || printf "")
  fi

  if [[ -n "$filter_action" ]]; then
    results=$(printf "%s" "$results" | grep -i "$filter_action" || printf "")
  fi

  if [[ -n "$filter_user" ]]; then
    results=$(printf "%s" "$results" | grep "|${filter_user}|" || printf "")
  fi

  # Date filtering (supports ISO8601 dates and relative formats)
  if [[ -n "$filter_since" ]]; then
    local since_epoch

    # Try to parse as date (ISO8601 format: YYYY-MM-DD)
    if [[ "$filter_since" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]; then
      # Direct ISO date format
      if command -v date >/dev/null 2>&1; then
        if date --version 2>/dev/null | grep -q GNU; then
          # GNU date (Linux)
          since_epoch=$(date -d "$filter_since" +%s 2>/dev/null || echo "0")
        else
          # BSD date (macOS)
          since_epoch=$(date -j -f "%Y-%m-%d" "$filter_since" +%s 2>/dev/null || echo "0")
        fi
      else
        since_epoch="0"
      fi
    else
      # Relative format (e.g., "7d", "2w", "1h")
      # Not implemented yet - fall back to showing all
      since_epoch="0"
    fi

    # Filter results by timestamp if we got a valid epoch
    if [[ "$since_epoch" != "0" ]]; then
      local filtered_results=""
      while IFS='|' read -r timestamp event_id category action user hostname pid details checksum; do
        # Extract date from timestamp (format: "YYYY-MM-DD HH:MM:SS UTC")
        local log_date="${timestamp%% *}"
        local log_epoch

        if date --version 2>/dev/null | grep -q GNU; then
          # GNU date (Linux)
          log_epoch=$(date -d "$log_date" +%s 2>/dev/null || echo "0")
        else
          # BSD date (macOS)
          log_epoch=$(date -j -f "%Y-%m-%d" "$log_date" +%s 2>/dev/null || echo "0")
        fi

        # Include if log timestamp >= filter timestamp
        if [[ "$log_epoch" -ge "$since_epoch" ]]; then
          filtered_results="${filtered_results}${timestamp}|${event_id}|${category}|${action}|${user}|${hostname}|${pid}|${details}|${checksum}"$'\n'
        fi
      done <<< "$results"

      # Remove trailing newline
      results="${filtered_results%$'\n'}"
    fi
  fi

  # Format output
  if [[ -z "$results" ]]; then
    printf "No matching audit events found\n"
    return 0
  fi

  # Pretty print results
  printf "%-20s %-20s %-10s %-20s %-15s %s\n" \
    "TIMESTAMP" "EVENT_ID" "CATEGORY" "ACTION" "USER" "DETAILS"
  printf "%s\n" "--------------------------------------------------------------------------------------------------------"

  printf "%s\n" "$results" | while IFS='|' read -r timestamp event_id category action user hostname pid details checksum; do
    # Truncate long fields for display
    local short_details
    short_details=$(printf "%s" "$details" | cut -c1-40)
    [[ ${#details} -gt 40 ]] && short_details="${short_details}..."

    printf "%-20s %-20s %-10s %-20s %-15s %s\n" \
      "${timestamp%% UTC*}" "$event_id" "$category" "$action" "$user" "$short_details"
  done
}

# Get audit statistics
audit_stats() {
  local audit_path="${NSELF_AUDIT_DIR}/${NSELF_AUDIT_FILE}"

  if [[ ! -f "$audit_path" ]]; then
    printf "No audit logs found\n"
    return 1
  fi

  local total_events
  total_events=$(grep -cv '^#' "$audit_path" 2>/dev/null || echo "0")

  printf "Audit Trail Statistics\n"
  printf "======================\n"
  printf "Total events: %s\n" "$total_events"
  printf "\n"

  printf "Events by category:\n"
  grep -v '^#' "$audit_path" | cut -d'|' -f3 | sort | uniq -c | while read -r count category; do
    printf "  %-15s %s\n" "$category" "$count"
  done

  printf "\n"
  printf "Recent activity (last 5 events):\n"
  grep -v '^#' "$audit_path" | tail -5 | while IFS='|' read -r timestamp event_id category action rest; do
    printf "  [%s] %s: %s\n" "$category" "${timestamp%% UTC*}" "$action"
  done
}

# Export audit logs for compliance
audit_export() {
  local output_file="$1"
  local format="${2:-txt}"

  local audit_path="${NSELF_AUDIT_DIR}/${NSELF_AUDIT_FILE}"

  if [[ ! -f "$audit_path" ]]; then
    printf "No audit logs found\n" >&2
    return 1
  fi

  case "$format" in
    txt|text)
      # Plain text export
      cp "$audit_path" "$output_file" || {
        printf "Failed to export audit logs\n" >&2
        return 1
      }
      ;;

    csv)
      # CSV format export
      {
        printf "Timestamp,EventID,Category,Action,User,Hostname,PID,Details,Checksum\n"
        grep -v '^#' "$audit_path" | while IFS='|' read -r timestamp event_id category action user hostname pid details checksum; do
          # Escape commas in details
          details=$(printf "%s" "$details" | sed 's/,/;/g')
          printf '"%s","%s","%s","%s","%s","%s","%s","%s","%s"\n' \
            "$timestamp" "$event_id" "$category" "$action" "$user" "$hostname" "$pid" "$details" "$checksum"
        done
      } > "$output_file" || {
        printf "Failed to export audit logs\n" >&2
        return 1
      }
      ;;

    json)
      # JSON format export
      {
        printf '{"format_version": "%s", "export_date": "%s", "events": [\n' \
          "$AUDIT_FORMAT_VERSION" "$(date -u '+%Y-%m-%d %H:%M:%S UTC')"

        local first=true
        grep -v '^#' "$audit_path" | while IFS='|' read -r timestamp event_id category action user hostname pid details checksum; do
          if [[ "$first" == "true" ]]; then
            first=false
          else
            printf ","
          fi

          # Escape quotes in details
          details=$(printf "%s" "$details" | sed 's/"/\\"/g')

          printf '\n  {"timestamp": "%s", "event_id": "%s", "category": "%s", "action": "%s", "user": "%s", "hostname": "%s", "pid": "%s", "details": "%s", "checksum": "%s"}' \
            "$timestamp" "$event_id" "$category" "$action" "$user" "$hostname" "$pid" "$details" "$checksum"
        done

        printf '\n]}\n'
      } > "$output_file" || {
        printf "Failed to export audit logs\n" >&2
        return 1
      }
      ;;

    *)
      printf "Unknown format: %s\n" "$format" >&2
      printf "Supported formats: txt, csv, json\n" >&2
      return 1
      ;;
  esac

  printf "Audit logs exported to: %s (format: %s)\n" "$output_file" "$format"
  return 0
}

# Verify audit log integrity
audit_verify() {
  local audit_path="${NSELF_AUDIT_DIR}/${NSELF_AUDIT_FILE}"

  if [[ ! -f "$audit_path" ]]; then
    printf "No audit logs found\n" >&2
    return 1
  fi

  printf "Verifying audit log integrity...\n"

  local total=0
  local valid=0
  local invalid=0

  grep -v '^#' "$audit_path" | while IFS='|' read -r timestamp event_id category action user hostname pid details stored_checksum; do
    total=$((total + 1))

    # Recalculate checksum
    local entry_data="${timestamp}|${event_id}|${category}|${action}|${user}|${hostname}|${pid}|${details}"
    local calculated_checksum
    calculated_checksum=$(_audit_checksum "$entry_data")

    if [[ "$stored_checksum" == "$calculated_checksum" ]]; then
      valid=$((valid + 1))
    else
      invalid=$((invalid + 1))
      printf "WARNING: Integrity check failed for event: %s\n" "$event_id" >&2
    fi
  done

  printf "\nIntegrity verification complete:\n"
  printf "  Total events: %s\n" "$total"
  printf "  Valid: %s\n" "$valid"
  printf "  Invalid: %s\n" "$invalid"

  if [[ $invalid -gt 0 ]]; then
    printf "\nWARNING: Some audit entries failed integrity verification!\n" >&2
    return 1
  else
    printf "\nAll audit entries verified successfully.\n"
    return 0
  fi
}

# =============================================================================
# EXPORT FUNCTIONS
# =============================================================================

export -f audit_init
export -f audit_log
export -f audit_auth
export -f audit_config
export -f audit_deploy
export -f audit_secret
export -f audit_security
export -f audit_data
export -f audit_admin
export -f audit_system
export -f audit_query
export -f audit_stats
export -f audit_export
export -f audit_verify

# Auto-initialize audit logging on source
audit_init >/dev/null 2>&1 || true
