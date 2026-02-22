#!/usr/bin/env bash
# user-import-export.sh - User import/export functionality (USER-003)
# Part of nself v0.6.0 - Phase 1 Sprint 2
#
# Implements user data import/export in JSON and CSV formats
# Supports bulk operations and data migration


# Source user manager
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

if [[ -f "$SCRIPT_DIR/user-manager.sh" ]]; then
  source "$SCRIPT_DIR/user-manager.sh"
fi

# ============================================================================
# JSON Export
# ============================================================================

# Export all users to JSON
# Usage: user_export_json [output_file] [include_deleted]
user_export_json() {
  local output_file="${1:--}" # Default to stdout
  local include_deleted="${2:-false}"

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Build query
  local where_clause=""
  if [[ "$include_deleted" != "true" ]]; then
    where_clause="WHERE u.deleted_at IS NULL"
  fi

  # Export users with profiles
  local export_json
  export_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(
       json_build_object(
         'id', u.id,
         'email', u.email,
         'phone', u.phone,
         'mfa_enabled', u.mfa_enabled,
         'email_verified', u.email_verified,
         'phone_verified', u.phone_verified,
         'created_at', u.created_at,
         'last_sign_in_at', u.last_sign_in_at,
         'deleted_at', u.deleted_at,
         'profile', json_build_object(
           'display_name', p.display_name,
           'avatar_url', p.avatar_url,
           'bio', p.bio,
           'location', p.location,
           'website', p.website,
           'timezone', p.timezone,
           'language', p.language,
           'custom_fields', p.custom_fields
         )
       )
     )
     FROM auth.users u
     LEFT JOIN auth.user_profiles p ON u.id = p.user_id
     $where_clause;" \
    2>/dev/null | xargs)

  if [[ -z "$export_json" ]] || [[ "$export_json" == "null" ]]; then
    export_json="[]"
  fi

  # Write to file or stdout
  if [[ "$output_file" == "-" ]]; then
    echo "$export_json"
  else
    echo "$export_json" >"$output_file"
    echo "✓ Exported users to $output_file" >&2
  fi

  return 0
}

# ============================================================================
# JSON Import
# ============================================================================

# Import users from JSON
# Usage: user_import_json <input_file> [skip_existing]
user_import_json() {
  local input_file="$1"
  local skip_existing="${2:-true}"

  if [[ ! -f "$input_file" ]]; then
    echo "ERROR: Input file not found: $input_file" >&2
    return 1
  fi

  # Read JSON file
  local import_json
  import_json=$(cat "$input_file")

  if [[ -z "$import_json" ]]; then
    echo "ERROR: Empty or invalid JSON file" >&2
    return 1
  fi

  # Count users to import
  local user_count
  user_count=$(echo "$import_json" | jq 'length' 2>/dev/null || echo "0")

  echo "Importing $user_count users..." >&2

  # Import each user
  local imported=0
  local skipped=0
  local failed=0

  for ((i = 0; i < user_count; i++)); do
    local user_data
    user_data=$(echo "$import_json" | jq ".[$i]" 2>/dev/null)

    # Extract user fields
    local email phone

    email=$(echo "$user_data" | jq -r '.email // empty')
    phone=$(echo "$user_data" | jq -r '.phone // empty')

    if [[ -z "$email" ]]; then
      echo "⚠ Skipping user $i: Missing email" >&2
      skipped=$((skipped + 1))
      continue
    fi

    # Check if user exists
    if user_get_by_email "$email" >/dev/null 2>&1; then
      if [[ "$skip_existing" == "true" ]]; then
        echo "⚠ Skipping existing user: $email" >&2
        skipped=$((skipped + 1))
        continue
      fi
    fi

    # Create user (without password for security)
    local user_id
    user_id=$(user_create "$email" "" "$phone" "{}" 2>&1)

    if [[ $? -eq 0 ]] && [[ -n "$user_id" ]]; then
      imported=$((imported + 1))

      # Import profile if present
      local profile_data
      profile_data=$(echo "$user_data" | jq -r '.profile // empty')

      if [[ -n "$profile_data" ]] && [[ "$profile_data" != "null" ]]; then
        # Source profile manager
        if [[ -f "$SCRIPT_DIR/user-profile.sh" ]]; then
          source "$SCRIPT_DIR/user-profile.sh"
          profile_update "$user_id" "$profile_data" 2>/dev/null || true
        fi
      fi

      echo "✓ Imported user: $email" >&2
    else
      echo "✗ Failed to import user: $email" >&2
      failed=$((failed + 1))
    fi
  done

  echo "" >&2
  echo "Import complete:" >&2
  echo "  Imported: $imported" >&2
  echo "  Skipped:  $skipped" >&2
  echo "  Failed:   $failed" >&2

  return 0
}

# ============================================================================
# CSV Export
# ============================================================================

# Export users to CSV
# Usage: user_export_csv [output_file] [include_deleted]
user_export_csv() {
  local output_file="${1:--}"
  local include_deleted="${2:-false}"

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Build query
  local where_clause=""
  if [[ "$include_deleted" != "true" ]]; then
    where_clause="WHERE u.deleted_at IS NULL"
  fi

  # Export to CSV
  local csv_data
  csv_data=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -A -F',' -c \
    "SELECT
       u.id,
       u.email,
       u.phone,
       u.mfa_enabled,
       u.email_verified,
       u.phone_verified,
       u.created_at,
       u.last_sign_in_at,
       p.display_name,
       p.avatar_url,
       p.bio,
       p.location,
       p.website,
       p.timezone,
       p.language
     FROM auth.users u
     LEFT JOIN auth.user_profiles p ON u.id = p.user_id
     $where_clause
     ORDER BY u.created_at;" \
    2>/dev/null)

  # Add CSV header
  local csv_output="id,email,phone,mfa_enabled,email_verified,phone_verified,created_at,last_sign_in_at,display_name,avatar_url,bio,location,website,timezone,language
$csv_data"

  # Write to file or stdout
  if [[ "$output_file" == "-" ]]; then
    echo "$csv_output"
  else
    echo "$csv_output" >"$output_file"
    echo "✓ Exported users to $output_file" >&2
  fi

  return 0
}

# ============================================================================
# CSV Import
# ============================================================================

# Import users from CSV
# Usage: user_import_csv <input_file> [skip_existing]
user_import_csv() {
  local input_file="$1"
  local skip_existing="${2:-true}"

  if [[ ! -f "$input_file" ]]; then
    echo "ERROR: Input file not found: $input_file" >&2
    return 1
  fi

  # Read CSV file (skip header)
  local line_count=0
  local imported=0
  local skipped=0
  local failed=0

  while IFS=',' read -r id email phone mfa_enabled email_verified phone_verified created_at last_sign_in_at display_name avatar_url bio location website timezone language; do
    line_count=$((line_count + 1))

    # Skip header
    if [[ $line_count -eq 1 ]]; then
      continue
    fi

    if [[ -z "$email" ]]; then
      echo "⚠ Skipping line $line_count: Missing email" >&2
      skipped=$((skipped + 1))
      continue
    fi

    # Check if user exists
    if user_get_by_email "$email" >/dev/null 2>&1; then
      if [[ "$skip_existing" == "true" ]]; then
        echo "⚠ Skipping existing user: $email" >&2
        skipped=$((skipped + 1))
        continue
      fi
    fi

    # Create user
    local user_id
    user_id=$(user_create "$email" "" "$phone" "{}" 2>&1)

    if [[ $? -eq 0 ]] && [[ -n "$user_id" ]]; then
      imported=$((imported + 1))

      # Import profile if present
      if [[ -n "$display_name" ]] || [[ -n "$bio" ]]; then
        if [[ -f "$SCRIPT_DIR/user-profile.sh" ]]; then
          source "$SCRIPT_DIR/user-profile.sh"

          [[ -n "$display_name" ]] && profile_set "$user_id" "display_name" "$display_name" 2>/dev/null
          [[ -n "$avatar_url" ]] && profile_set "$user_id" "avatar_url" "$avatar_url" 2>/dev/null
          [[ -n "$bio" ]] && profile_set "$user_id" "bio" "$bio" 2>/dev/null
          [[ -n "$location" ]] && profile_set "$user_id" "location" "$location" 2>/dev/null
          [[ -n "$website" ]] && profile_set "$user_id" "website" "$website" 2>/dev/null
          [[ -n "$timezone" ]] && profile_set "$user_id" "timezone" "$timezone" 2>/dev/null
          [[ -n "$language" ]] && profile_set "$user_id" "language" "$language" 2>/dev/null
        fi
      fi

      echo "✓ Imported user: $email" >&2
    else
      echo "✗ Failed to import user: $email" >&2
      failed=$((failed + 1))
    fi
  done <"$input_file"

  echo "" >&2
  echo "Import complete:" >&2
  echo "  Imported: $imported" >&2
  echo "  Skipped:  $skipped" >&2
  echo "  Failed:   $failed" >&2

  return 0
}

# ============================================================================
# Bulk Operations
# ============================================================================

# Bulk delete users
# Usage: user_bulk_delete <user_ids_json_array> [hard_delete]
# Example: user_bulk_delete '["uuid1", "uuid2", "uuid3"]' false
user_bulk_delete() {
  local user_ids_json="$1"
  local hard_delete="${2:-false}"

  # Parse JSON array
  local user_count
  user_count=$(echo "$user_ids_json" | jq 'length' 2>/dev/null || echo "0")

  if [[ "$user_count" -eq 0 ]]; then
    echo "ERROR: No user IDs provided" >&2
    return 1
  fi

  echo "Deleting $user_count users..." >&2

  local deleted=0
  local failed=0

  for ((i = 0; i < user_count; i++)); do
    local user_id
    user_id=$(echo "$user_ids_json" | jq -r ".[$i]")

    if user_delete "$user_id" "$hard_delete" >/dev/null 2>&1; then
      deleted=$((deleted + 1))
    else
      failed=$((failed + 1))
    fi
  done

  echo "✓ Deleted: $deleted" >&2
  echo "✗ Failed: $failed" >&2

  return 0
}

# ============================================================================
# Export functions
# ============================================================================

export -f user_export_json
export -f user_import_json
export -f user_export_csv
export -f user_import_csv
export -f user_bulk_delete
