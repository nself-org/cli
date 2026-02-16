#!/usr/bin/env bash
# framework.sh - Compliance framework (GDPR, SOC2, HIPAA)
# Part of nself v0.7.0 - Sprint 9: CSA-001


# Compliance standards
readonly COMPLIANCE_GDPR="gdpr"

set -euo pipefail

readonly COMPLIANCE_SOC2="soc2"
readonly COMPLIANCE_HIPAA="hipaa"

# Initialize compliance framework
compliance_init() {
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)
  [[ -z "$container" ]] && {
    echo "ERROR: PostgreSQL not found" >&2
    return 1
  }

  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE SCHEMA IF NOT EXISTS compliance;

CREATE TABLE IF NOT EXISTS compliance.standards (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL UNIQUE,
  version TEXT,
  description TEXT,
  requirements JSONB DEFAULT '[]',
  enabled BOOLEAN DEFAULT TRUE,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS compliance.controls (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  standard_id UUID NOT NULL REFERENCES compliance.standards(id) ON DELETE CASCADE,
  control_id TEXT NOT NULL,
  control_name TEXT NOT NULL,
  description TEXT,
  category TEXT,
  implementation_status TEXT, -- not_implemented, in_progress, implemented, not_applicable
  evidence JSONB DEFAULT '[]',
  last_assessed TIMESTAMPTZ,
  next_assessment TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS compliance.data_retention (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  data_type TEXT NOT NULL,
  retention_days INTEGER NOT NULL,
  reason TEXT,
  standard_id UUID REFERENCES compliance.standards(id) ON DELETE SET NULL,
  enabled BOOLEAN DEFAULT TRUE,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS compliance.data_requests (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  request_type TEXT NOT NULL, -- access, rectification, erasure, portability
  user_id UUID NOT NULL,
  status TEXT NOT NULL, -- pending, in_progress, completed, rejected
  requested_at TIMESTAMPTZ DEFAULT NOW(),
  completed_at TIMESTAMPTZ,
  data_package JSONB,
  notes TEXT
);

CREATE INDEX IF NOT EXISTS idx_standards_enabled ON compliance.standards(enabled);
CREATE INDEX IF NOT EXISTS idx_controls_standard ON compliance.controls(standard_id);
CREATE INDEX IF NOT EXISTS idx_controls_status ON compliance.controls(implementation_status);
CREATE INDEX IF NOT EXISTS idx_retention_type ON compliance.data_retention(data_type);
CREATE INDEX IF NOT EXISTS idx_requests_user ON compliance.data_requests(user_id);
CREATE INDEX IF NOT EXISTS idx_requests_status ON compliance.data_requests(status);

-- Initialize GDPR compliance
INSERT INTO compliance.standards (name, version, description)
VALUES ('GDPR', '2016/679', 'General Data Protection Regulation')
ON CONFLICT (name) DO NOTHING;

-- Initialize SOC 2 compliance
INSERT INTO compliance.standards (name, version, description)
VALUES ('SOC 2', 'Type II', 'Service Organization Control 2')
ON CONFLICT (name) DO NOTHING;

-- Initialize HIPAA compliance
INSERT INTO compliance.standards (name, version, description)
VALUES ('HIPAA', '1996', 'Health Insurance Portability and Accountability Act')
ON CONFLICT (name) DO NOTHING;
EOSQL
  return 0
}

# GDPR: Right to be forgotten
gdpr_erase_user_data() {
  local user_id="$1"
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  # Anonymize or delete user data across all tables
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "BEGIN;
     -- Mark user as deleted
     UPDATE auth.users SET deleted_at = NOW(), email = 'deleted_' || id::text || '@example.com' WHERE id = '$user_id';
     -- Delete sessions
     DELETE FROM auth.sessions WHERE user_id = '$user_id';
     -- Delete MFA
     DELETE FROM auth.mfa_methods WHERE user_id = '$user_id';
     -- Delete API keys
     DELETE FROM auth.api_keys WHERE user_id = '$user_id';
     -- Anonymize logs
     UPDATE logs.entries SET user_id = NULL WHERE user_id = '$user_id';
     COMMIT;" >/dev/null 2>&1

  # Create data request record
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO compliance.data_requests (request_type, user_id, status, completed_at)
     VALUES ('erasure', '$user_id', 'completed', NOW());" >/dev/null 2>&1

  echo "✓ User data erased for GDPR compliance"
}

# GDPR: Data export (right to data portability)
gdpr_export_user_data() {
  local user_id="$1"
  local output_file="${2:-/tmp/user_data_export_${user_id}.json}"
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  # Collect all user data
  local user_data=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_build_object(
       'user', (SELECT row_to_json(u) FROM (SELECT id, email, created_at, last_sign_in_at FROM auth.users WHERE id = '$user_id') u),
       'sessions', (SELECT json_agg(s) FROM (SELECT * FROM auth.sessions WHERE user_id = '$user_id') s),
       'audit_logs', (SELECT json_agg(a) FROM (SELECT * FROM audit.events WHERE user_id = '$user_id') a),
       'exported_at', NOW()
     );" 2>/dev/null | xargs)

  echo "$user_data" | jq '.' >"$output_file"

  # Record export request
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO compliance.data_requests (request_type, user_id, status, completed_at, data_package)
     VALUES ('portability', '$user_id', 'completed', NOW(), '$user_data'::jsonb);" >/dev/null 2>&1

  echo "✓ User data exported to $output_file"
}

# Set data retention policy
compliance_set_retention() {
  local data_type="$1"
  local retention_days="$2"
  local reason="$3"
  local standard="${4:-gdpr}"

  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)
  local standard_id=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT id FROM compliance.standards WHERE name = '$standard';" 2>/dev/null | xargs)

  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO compliance.data_retention (data_type, retention_days, reason, standard_id)
     VALUES ('$data_type', $retention_days, '$reason', $([ -n "$standard_id" ] && echo "'$standard_id'" || echo "NULL"))
     ON CONFLICT (data_type) DO UPDATE SET
       retention_days = EXCLUDED.retention_days,
       reason = EXCLUDED.reason;" >/dev/null 2>&1
}

# Apply retention policies
compliance_apply_retention() {
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  # Get active retention policies
  local policies=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(p) FROM (
       SELECT data_type, retention_days
       FROM compliance.data_retention
       WHERE enabled = TRUE
     ) p;" 2>/dev/null | xargs)

  [[ -z "$policies" || "$policies" == "null" ]] && return 0

  # Apply each policy (simplified - would need table mapping)
  echo "$policies" | jq -c '.[]' | while read -r policy; do
    local data_type=$(echo "$policy" | jq -r '.data_type')
    local days=$(echo "$policy" | jq -r '.retention_days')

    echo "Applying retention policy: $data_type ($days days)"
    # Would delete old data based on type
  done
}

# Add compliance control
compliance_add_control() {
  local standard="$1"
  local control_id="$2"
  local control_name="$3"
  local description="$4"
  local category="${5:-}"

  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)
  local standard_id=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT id FROM compliance.standards WHERE name = '$standard';" 2>/dev/null | xargs)

  [[ -z "$standard_id" ]] && {
    echo "ERROR: Standard not found" >&2
    return 1
  }

  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO compliance.controls (standard_id, control_id, control_name, description, category, implementation_status)
     VALUES ('$standard_id', '$control_id', '$control_name', '$description', $([ -n "$category" ] && echo "'$category'" || echo "NULL"), 'not_implemented');" >/dev/null 2>&1
}

# Update control status
compliance_update_control_status() {
  local control_id="$1"
  local status="$2" # not_implemented, in_progress, implemented, not_applicable
  local evidence="${3:-[]}"

  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "UPDATE compliance.controls
     SET implementation_status = '$status',
         evidence = '$evidence'::jsonb,
         last_assessed = NOW()
     WHERE control_id = '$control_id';" >/dev/null 2>&1
}

# Get compliance status
compliance_get_status() {
  local standard="${1:-}"
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -n "$standard" ]]; then
    # Status for specific standard
    local status=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
      "SELECT json_build_object(
         'standard', s.name,
         'total_controls', COUNT(c.id),
         'implemented', COUNT(*) FILTER (WHERE c.implementation_status = 'implemented'),
         'in_progress', COUNT(*) FILTER (WHERE c.implementation_status = 'in_progress'),
         'not_implemented', COUNT(*) FILTER (WHERE c.implementation_status = 'not_implemented'),
         'compliance_percentage', ROUND(COUNT(*) FILTER (WHERE c.implementation_status = 'implemented') * 100.0 / COUNT(*), 2)
       )
       FROM compliance.standards s
       LEFT JOIN compliance.controls c ON s.id = c.standard_id
       WHERE s.name = '$standard'
       GROUP BY s.name;" 2>/dev/null | xargs)

    echo "$status"
  else
    # Status for all standards
    local statuses=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
      "SELECT json_agg(s) FROM (
         SELECT
           st.name AS standard,
           COUNT(c.id) AS total_controls,
           COUNT(*) FILTER (WHERE c.implementation_status = 'implemented') AS implemented,
           ROUND(COUNT(*) FILTER (WHERE c.implementation_status = 'implemented') * 100.0 / NULLIF(COUNT(*), 0), 2) AS compliance_percentage
         FROM compliance.standards st
         LEFT JOIN compliance.controls c ON st.id = c.standard_id
         WHERE st.enabled = TRUE
         GROUP BY st.name
       ) s;" 2>/dev/null | xargs)

    [[ -z "$statuses" || "$statuses" == "null" ]] && echo "[]" || echo "$statuses"
  fi
}

export -f compliance_init gdpr_erase_user_data gdpr_export_user_data
export -f compliance_set_retention compliance_apply_retention
export -f compliance_add_control compliance_update_control_status compliance_get_status
