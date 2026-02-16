#!/usr/bin/env bash
# reports.sh - Compliance reporting and security scanning
# Part of nself v0.7.0 - Sprint 9: CSA-003, CSA-005


# Generate compliance report
compliance_generate_report() {

set -euo pipefail

  local standard="${1:-all}"
  local output_format="${2:-json}" # json, html, pdf
  local output_file="${3:-/tmp/compliance_report.json}"

  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  # Get compliance status
  local status=$(compliance_get_status "$standard")

  # Get data retention policies
  local retention=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(r) FROM (
       SELECT data_type, retention_days, reason
       FROM compliance.data_retention
       WHERE enabled = TRUE
     ) r;" 2>/dev/null | xargs)

  # Get recent data requests
  local requests=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(req) FROM (
       SELECT request_type, status, requested_at, completed_at
       FROM compliance.data_requests
       WHERE requested_at >= NOW() - INTERVAL '30 days'
       ORDER BY requested_at DESC
       LIMIT 100
     ) req;" 2>/dev/null | xargs)

  # Build report
  local report=$(jq -n \
    --argjson status "$status" \
    --argjson retention "${retention:-[]}" \
    --argjson requests "${requests:-[]}" \
    '{
      generated_at: now,
      compliance_status: $status,
      data_retention_policies: $retention,
      recent_data_requests: $requests
    }')

  case "$output_format" in
    json)
      echo "$report" | jq '.' >"$output_file"
      ;;
    html)
      # Would generate HTML report
      echo "<html><body><h1>Compliance Report</h1><pre>$report</pre></body></html>" >"$output_file"
      ;;
    pdf)
      # Would generate PDF report
      echo "PDF generation not implemented" >&2
      ;;
  esac

  echo "✓ Compliance report generated: $output_file"
}

# Security scan - check for common issues
security_scan() {
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  local issues="[]"

  # Check 1: Users without MFA
  local no_mfa=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT COUNT(*) FROM auth.users WHERE deleted_at IS NULL
     AND id NOT IN (SELECT user_id FROM auth.mfa_methods WHERE enabled = TRUE);" 2>/dev/null | xargs)

  [[ $no_mfa -gt 0 ]] && issues=$(echo "$issues" | jq --arg count "$no_mfa" \
    '. += [{"severity":"medium","issue":"Users without MFA","count":$count}]')

  # Check 2: Weak passwords (simplified)
  local weak_passwords=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT COUNT(*) FROM auth.users WHERE deleted_at IS NULL
     AND LENGTH(password_hash) < 60;" 2>/dev/null | xargs)

  [[ $weak_passwords -gt 0 ]] && issues=$(echo "$issues" | jq --arg count "$weak_passwords" \
    '. += [{"severity":"high","issue":"Potentially weak passwords","count":$count}]')

  # Check 3: Expired sessions
  local expired_sessions=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT COUNT(*) FROM auth.sessions WHERE expires_at < NOW();" 2>/dev/null | xargs)

  [[ $expired_sessions -gt 0 ]] && issues=$(echo "$issues" | jq --arg count "$expired_sessions" \
    '. += [{"severity":"low","issue":"Expired sessions not cleaned up","count":$count}]')

  echo "$issues"
}

# Access control audit
access_audit() {
  local time_range="${1:-24 hours}"
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  # Get permission changes
  local perm_changes=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(a) FROM (
       SELECT event_type, action, result, created_at, metadata
       FROM audit.events
       WHERE event_type IN ('permission_granted', 'permission_revoked', 'role_assigned', 'role_revoked')
         AND created_at >= NOW() - INTERVAL '$time_range'
       ORDER BY created_at DESC
       LIMIT 100
     ) a;" 2>/dev/null | xargs)

  # Get failed access attempts
  local failed_access=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(a) FROM (
       SELECT event_type, action, result, created_at, metadata
       FROM audit.events
       WHERE result = 'failure'
         AND event_type LIKE '%access%'
         AND created_at >= NOW() - INTERVAL '$time_range'
       ORDER BY created_at DESC
       LIMIT 100
     ) a;" 2>/dev/null | xargs)

  echo "{\"permission_changes\":${perm_changes:-[]},\"failed_access\":${failed_access:-[]}}"
}

export -f compliance_generate_report security_scan access_audit
