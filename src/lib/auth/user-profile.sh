#!/usr/bin/env bash
# user-profile.sh - User profile management (USER-002)
# Part of nself v0.6.0 - Phase 1 Sprint 2
#
# Implements user profile management features
# Avatar, display name, bio, custom fields


# ============================================================================
# Profile Creation & Retrieval
# ============================================================================

# Create or update user profile
# Usage: profile_set <user_id> <field> <value>
profile_set() {

set -euo pipefail

  local user_id="$1"
  local field="$2"
  local value="$3"

  if [[ -z "$user_id" ]] || [[ -z "$field" ]]; then
    echo "ERROR: User ID and field required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Create user_profiles table if it doesn't exist
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE TABLE IF NOT EXISTS auth.user_profiles (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID UNIQUE REFERENCES auth.users(id) ON DELETE CASCADE,
  display_name TEXT,
  avatar_url TEXT,
  bio TEXT,
  location TEXT,
  website TEXT,
  timezone TEXT,
  language TEXT DEFAULT 'en',
  custom_fields JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_user_profiles_user_id ON auth.user_profiles(user_id);
EOSQL

  # Escape single quotes in value
  value=$(echo "$value" | sed "s/'/''/g")

  # Update or insert profile field
  case "$field" in
    display_name | avatar_url | bio | location | website | timezone | language)
      docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
        "INSERT INTO auth.user_profiles (user_id, $field, updated_at)
         VALUES ('$user_id', '$value', NOW())
         ON CONFLICT (user_id) DO UPDATE SET
           $field = EXCLUDED.$field,
           updated_at = NOW();" \
        >/dev/null 2>&1
      ;;
    *)
      # Custom field - store in JSONB
      docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
        "INSERT INTO auth.user_profiles (user_id, custom_fields, updated_at)
         VALUES ('$user_id', jsonb_build_object('$field', '$value'), NOW())
         ON CONFLICT (user_id) DO UPDATE SET
           custom_fields = user_profiles.custom_fields || jsonb_build_object('$field', '$value'),
           updated_at = NOW();" \
        >/dev/null 2>&1
      ;;
  esac

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to update profile" >&2
    return 1
  fi

  echo "✓ Profile field '$field' updated" >&2
  return 0
}

# Get user profile
# Usage: profile_get <user_id>
# Returns: JSON profile object
profile_get() {
  local user_id="$1"

  if [[ -z "$user_id" ]]; then
    echo "ERROR: User ID required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Get profile data
  local profile_json
  profile_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT row_to_json(p) FROM (
       SELECT
         user_id,
         display_name,
         avatar_url,
         bio,
         location,
         website,
         timezone,
         language,
         custom_fields,
         created_at,
         updated_at
       FROM auth.user_profiles
       WHERE user_id = '$user_id'
     ) p;" \
    2>/dev/null | xargs)

  if [[ -z "$profile_json" ]] || [[ "$profile_json" == "null" ]]; then
    echo "{}"
    return 0
  fi

  echo "$profile_json"
  return 0
}

# Get profile field
# Usage: profile_get_field <user_id> <field>
# Returns: Field value
profile_get_field() {
  local user_id="$1"
  local field="$2"

  if [[ -z "$user_id" ]] || [[ -z "$field" ]]; then
    echo "ERROR: User ID and field required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Get field value
  local value
  case "$field" in
    display_name | avatar_url | bio | location | website | timezone | language)
      value=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
        "SELECT $field FROM auth.user_profiles WHERE user_id = '$user_id' LIMIT 1;" \
        2>/dev/null | xargs)
      ;;
    *)
      # Custom field - get from JSONB
      value=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
        "SELECT custom_fields->>'$field' FROM auth.user_profiles WHERE user_id = '$user_id' LIMIT 1;" \
        2>/dev/null | xargs)
      ;;
  esac

  echo "$value"
  return 0
}

# ============================================================================
# Batch Profile Updates
# ============================================================================

# Update multiple profile fields at once
# Usage: profile_update <user_id> <json_updates>
# Example: profile_update "uuid" '{"display_name": "John Doe", "bio": "Hello world"}'
profile_update() {
  local user_id="$1"
  local json_updates="$2"

  if [[ -z "$user_id" ]] || [[ -z "$json_updates" ]]; then
    echo "ERROR: User ID and JSON updates required" >&2
    return 1
  fi

  # Parse JSON and update fields
  local fields=(display_name avatar_url bio location website timezone language)

  for field in "${fields[@]}"; do
    local value
    value=$(echo "$json_updates" | jq -r ".$field // empty" 2>/dev/null || echo "")

    if [[ -n "$value" ]] && [[ "$value" != "null" ]]; then
      profile_set "$user_id" "$field" "$value" 2>/dev/null
    fi
  done

  # Handle custom fields
  local custom_fields
  custom_fields=$(echo "$json_updates" | jq -r '.custom_fields // empty' 2>/dev/null || echo "")

  if [[ -n "$custom_fields" ]] && [[ "$custom_fields" != "null" ]]; then
    # Get all keys from custom_fields
    local keys
    keys=$(echo "$custom_fields" | jq -r 'keys[]' 2>/dev/null || echo "")

    while IFS= read -r key; do
      if [[ -n "$key" ]]; then
        local value
        value=$(echo "$custom_fields" | jq -r ".$key" 2>/dev/null || echo "")
        profile_set "$user_id" "$key" "$value" 2>/dev/null
      fi
    done <<<"$keys"
  fi

  echo "✓ Profile updated successfully" >&2
  return 0
}

# ============================================================================
# Avatar Management
# ============================================================================

# Set user avatar
# Usage: profile_set_avatar <user_id> <avatar_url>
profile_set_avatar() {
  local user_id="$1"
  local avatar_url="$2"

  if [[ -z "$user_id" ]] || [[ -z "$avatar_url" ]]; then
    echo "ERROR: User ID and avatar URL required" >&2
    return 1
  fi

  # Validate URL format (basic)
  if ! echo "$avatar_url" | grep -qE '^https?://'; then
    echo "ERROR: Invalid avatar URL (must be http:// or https://)" >&2
    return 1
  fi

  profile_set "$user_id" "avatar_url" "$avatar_url"
  return $?
}

# Remove user avatar
# Usage: profile_remove_avatar <user_id>
profile_remove_avatar() {
  local user_id="$1"

  if [[ -z "$user_id" ]]; then
    echo "ERROR: User ID required" >&2
    return 1
  fi

  profile_set "$user_id" "avatar_url" ""
  return $?
}

# ============================================================================
# Profile Deletion
# ============================================================================

# Delete user profile
# Usage: profile_delete <user_id>
profile_delete() {
  local user_id="$1"

  if [[ -z "$user_id" ]]; then
    echo "ERROR: User ID required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Delete profile
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "DELETE FROM auth.user_profiles WHERE user_id = '$user_id';" \
    >/dev/null 2>&1

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to delete profile" >&2
    return 1
  fi

  echo "✓ Profile deleted successfully" >&2
  return 0
}

# ============================================================================
# Profile Validation
# ============================================================================

# Validate profile data
# Usage: profile_validate <field> <value>
# Returns: 0 if valid, 1 if invalid
profile_validate() {
  local field="$1"
  local value="$2"

  case "$field" in
    display_name)
      # Max 100 characters
      if [[ ${#value} -gt 100 ]]; then
        echo "ERROR: Display name too long (max 100 characters)" >&2
        return 1
      fi
      ;;
    bio)
      # Max 500 characters
      if [[ ${#value} -gt 500 ]]; then
        echo "ERROR: Bio too long (max 500 characters)" >&2
        return 1
      fi
      ;;
    website)
      # Valid URL
      if ! echo "$value" | grep -qE '^https?://'; then
        echo "ERROR: Invalid website URL" >&2
        return 1
      fi
      ;;
    timezone)
      # Valid timezone (basic check)
      if ! echo "$value" | grep -qE '^[A-Z][a-z]+/[A-Z][a-z_]+$'; then
        echo "ERROR: Invalid timezone format (use IANA timezone format, e.g., America/New_York)" >&2
        return 1
      fi
      ;;
    language)
      # Valid language code (ISO 639-1)
      if ! echo "$value" | grep -qE '^[a-z]{2}(-[A-Z]{2})?$'; then
        echo "ERROR: Invalid language code (use ISO 639-1 format, e.g., en, en-US)" >&2
        return 1
      fi
      ;;
  esac

  return 0
}

# ============================================================================
# Profile Search
# ============================================================================

# Search profiles by field
# Usage: profile_search <field> <query> [limit]
# Returns: JSON array of profiles
profile_search() {
  local field="$1"
  local query="$2"
  local limit="${3:-50}"

  if [[ -z "$field" ]] || [[ -z "$query" ]]; then
    echo "ERROR: Field and query required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Search profiles
  local profiles_json
  case "$field" in
    display_name | bio | location)
      profiles_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
        "SELECT json_agg(p) FROM (
           SELECT
             user_id,
             display_name,
             avatar_url,
             bio,
             location,
             website,
             timezone,
             language
           FROM auth.user_profiles
           WHERE $field ILIKE '%$query%'
           ORDER BY updated_at DESC
           LIMIT $limit
         ) p;" \
        2>/dev/null | xargs)
      ;;
    *)
      # Search custom fields
      profiles_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
        "SELECT json_agg(p) FROM (
           SELECT
             user_id,
             display_name,
             avatar_url,
             bio,
             custom_fields
           FROM auth.user_profiles
           WHERE custom_fields->>'$field' ILIKE '%$query%'
           ORDER BY updated_at DESC
           LIMIT $limit
         ) p;" \
        2>/dev/null | xargs)
      ;;
  esac

  if [[ -z "$profiles_json" ]] || [[ "$profiles_json" == "null" ]]; then
    echo "[]"
    return 0
  fi

  echo "$profiles_json"
  return 0
}

# ============================================================================
# Export functions
# ============================================================================

export -f profile_set
export -f profile_get
export -f profile_get_field
export -f profile_update
export -f profile_set_avatar
export -f profile_remove_avatar
export -f profile_delete
export -f profile_validate
export -f profile_search
