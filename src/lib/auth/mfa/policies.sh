#!/usr/bin/env bash
# policies.sh - MFA policy management (MFA-006)
# Part of nself v0.6.0 - Phase 1 Sprint 2
#
# Implements MFA enforcement policies, exemptions, and rules
# Allows flexible MFA requirements based on user roles, IP, etc.


# MFA policy types
readonly MFA_POLICY_REQUIRED="required"     # MFA always required

set -euo pipefail

readonly MFA_POLICY_OPTIONAL="optional"     # MFA optional
readonly MFA_POLICY_ROLE_BASED="role_based" # Required for certain roles
readonly MFA_POLICY_IP_BASED="ip_based"     # Required for certain IPs
readonly MFA_POLICY_TIME_BASED="time_based" # Required during certain times

# ============================================================================
# Global MFA Policy
# ============================================================================

# Set global MFA policy
# Usage: mfa_policy_set_global <policy_type> [policy_data]
mfa_policy_set_global() {
  local policy_type="$1"
  local policy_data="${2:-{}}"

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Create mfa_policies table if it doesn't exist
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE TABLE IF NOT EXISTS auth.mfa_policies (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  policy_type TEXT NOT NULL,
  policy_data JSONB DEFAULT '{}',
  enabled BOOLEAN DEFAULT TRUE,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_mfa_policies_type ON auth.mfa_policies(policy_type);
EOSQL

  # Upsert global policy
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO auth.mfa_policies (policy_type, policy_data, enabled)
     VALUES ('global', '{\"type\": \"$policy_type\", \"data\": $policy_data}'::jsonb, TRUE)
     ON CONFLICT (id) DO UPDATE SET
       policy_type = 'global',
       policy_data = '{\"type\": \"$policy_type\", \"data\": $policy_data}'::jsonb,
       updated_at = NOW();" \
    >/dev/null 2>&1

  echo "✓ Global MFA policy set to: $policy_type" >&2
  return 0
}

# Get global MFA policy
# Usage: mfa_policy_get_global
# Returns: JSON with policy type and data
mfa_policy_get_global() {
  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Get global policy
  local policy
  policy=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT policy_data FROM auth.mfa_policies WHERE policy_type = 'global' AND enabled = TRUE LIMIT 1;" \
    2>/dev/null | xargs)

  if [[ -z "$policy" ]]; then
    echo '{"type": "optional", "data": {}}'
    return 0
  fi

  echo "$policy"
  return 0
}

# ============================================================================
# User-Specific MFA Exemptions
# ============================================================================

# Add MFA exemption for user
# Usage: mfa_exemption_add <user_id> <reason> [expires_at]
mfa_exemption_add() {
  local user_id="$1"
  local reason="$2"
  local expires_at="${3:-}"

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Create mfa_exemptions table if it doesn't exist
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE TABLE IF NOT EXISTS auth.mfa_exemptions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID REFERENCES auth.users(id) ON DELETE CASCADE,
  reason TEXT NOT NULL,
  expires_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  created_by UUID,
  revoked BOOLEAN DEFAULT FALSE,
  revoked_at TIMESTAMPTZ,
  revoked_by UUID,
  UNIQUE(user_id)
);
CREATE INDEX IF NOT EXISTS idx_mfa_exemptions_user_id ON auth.mfa_exemptions(user_id);
EOSQL

  # Add exemption
  local expires_clause=""
  if [[ -n "$expires_at" ]]; then
    expires_clause=", expires_at = '$expires_at'::timestamptz"
  fi

  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO auth.mfa_exemptions (user_id, reason${expires_clause:+, expires_at})
     VALUES ('$user_id', '$reason'${expires_at:+, '$expires_at'::timestamptz})
     ON CONFLICT (user_id) DO UPDATE SET
       reason = EXCLUDED.reason,
       expires_at = EXCLUDED.expires_at,
       revoked = FALSE,
       created_at = NOW();" \
    >/dev/null 2>&1

  echo "✓ MFA exemption added for user" >&2
  return 0
}

# Revoke MFA exemption for user
# Usage: mfa_exemption_revoke <user_id>
mfa_exemption_revoke() {
  local user_id="$1"

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Revoke exemption
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "UPDATE auth.mfa_exemptions
     SET revoked = TRUE,
         revoked_at = NOW()
     WHERE user_id = '$user_id';" \
    >/dev/null 2>&1

  echo "✓ MFA exemption revoked" >&2
  return 0
}

# Check if user has MFA exemption
# Usage: mfa_exemption_check <user_id>
# Returns: 0 if exempt, 1 if not exempt
mfa_exemption_check() {
  local user_id="$1"

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Check for active exemption
  local has_exemption
  has_exemption=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT COUNT(*) FROM auth.mfa_exemptions
     WHERE user_id = '$user_id'
       AND revoked = FALSE
       AND (expires_at IS NULL OR expires_at > NOW());" \
    2>/dev/null | xargs)

  if [[ "$has_exemption" == "1" ]]; then
    return 0 # Exempt
  else
    return 1 # Not exempt
  fi
}

# ============================================================================
# Role-Based MFA Requirements
# ============================================================================

# Set MFA requirement for role
# Usage: mfa_role_requirement_set <role_name> <required>
mfa_role_requirement_set() {
  local role_name="$1"
  local required="$2" # true or false

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Create mfa_role_requirements table if it doesn't exist
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE TABLE IF NOT EXISTS auth.mfa_role_requirements (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  role_name TEXT UNIQUE NOT NULL,
  required BOOLEAN DEFAULT TRUE,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_mfa_role_requirements_role ON auth.mfa_role_requirements(role_name);
EOSQL

  # Upsert role requirement
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO auth.mfa_role_requirements (role_name, required)
     VALUES ('$role_name', $required)
     ON CONFLICT (role_name) DO UPDATE SET
       required = EXCLUDED.required,
       updated_at = NOW();" \
    >/dev/null 2>&1

  echo "✓ MFA requirement for role '$role_name' set to: $required" >&2
  return 0
}

# Check if MFA is required for role
# Usage: mfa_role_requirement_check <role_name>
# Returns: 0 if required, 1 if not required
mfa_role_requirement_check() {
  local role_name="$1"

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Check role requirement
  local is_required
  is_required=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT required FROM auth.mfa_role_requirements
     WHERE role_name = '$role_name';" \
    2>/dev/null | xargs)

  if [[ "$is_required" == "t" ]]; then
    return 0 # Required
  else
    return 1 # Not required
  fi
}

# ============================================================================
# MFA Enforcement Logic
# ============================================================================

# Check if MFA is required for user
# Usage: mfa_is_required <user_id> [role_name]
# Returns: 0 if required, 1 if not required
mfa_is_required() {
  local user_id="$1"
  local role_name="${2:-}"

  # Check for user exemption first
  if mfa_exemption_check "$user_id" 2>/dev/null; then
    return 1 # Not required (exempt)
  fi

  # Get global policy
  local global_policy
  global_policy=$(mfa_policy_get_global)
  local policy_type
  policy_type=$(echo "$global_policy" | jq -r '.type // "optional"')

  case "$policy_type" in
    required)
      return 0 # Always required
      ;;
    optional)
      return 1 # Never required
      ;;
    role_based)
      if [[ -n "$role_name" ]]; then
        if mfa_role_requirement_check "$role_name" 2>/dev/null; then
          return 0 # Required for this role
        else
          return 1 # Not required for this role
        fi
      else
        return 1 # No role specified, not required
      fi
      ;;
    *)
      return 1 # Default to not required
      ;;
  esac
}

# Get MFA status for user
# Usage: mfa_get_user_status <user_id>
# Returns: JSON with MFA status
mfa_get_user_status() {
  local user_id="$1"

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Get user MFA status
  local mfa_enabled
  mfa_enabled=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT mfa_enabled FROM auth.users WHERE id = '$user_id' LIMIT 1;" \
    2>/dev/null | xargs)

  # Check available MFA methods
  local has_totp has_sms has_email backup_codes_count

  has_totp=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT COUNT(*) FROM auth.mfa_totp WHERE user_id = '$user_id' AND enabled = TRUE;" \
    2>/dev/null | xargs)

  has_sms=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT COUNT(*) FROM auth.mfa_sms WHERE user_id = '$user_id' AND enabled = TRUE;" \
    2>/dev/null | xargs)

  has_email=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT COUNT(*) FROM auth.mfa_email WHERE user_id = '$user_id' AND enabled = TRUE;" \
    2>/dev/null | xargs)

  backup_codes_count=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT COUNT(*) FROM auth.mfa_backup_codes WHERE user_id = '$user_id' AND used = FALSE;" \
    2>/dev/null | xargs)

  # Check if exempt
  local is_exempt=false
  if mfa_exemption_check "$user_id" 2>/dev/null; then
    is_exempt=true
  fi

  # Check if required
  local is_required=false
  if mfa_is_required "$user_id" 2>/dev/null; then
    is_required=true
  fi

  cat <<EOF
{
  "enabled": $([ "$mfa_enabled" == "t" ] && echo "true" || echo "false"),
  "required": $is_required,
  "exempt": $is_exempt,
  "methods": {
    "totp": $([ "${has_totp:-0}" -gt 0 ] && echo "true" || echo "false"),
    "sms": $([ "${has_sms:-0}" -gt 0 ] && echo "true" || echo "false"),
    "email": $([ "${has_email:-0}" -gt 0 ] && echo "true" || echo "false")
  },
  "backup_codes_remaining": ${backup_codes_count:-0}
}
EOF

  return 0
}

# ============================================================================
# Export functions
# ============================================================================

export -f mfa_policy_set_global
export -f mfa_policy_get_global
export -f mfa_exemption_add
export -f mfa_exemption_revoke
export -f mfa_exemption_check
export -f mfa_role_requirement_set
export -f mfa_role_requirement_check
export -f mfa_is_required
export -f mfa_get_user_status
