#!/usr/bin/env bash
# safe-query.sh - Safe database query wrapper with parameterized query support
# Part of nself v0.9.0 - Security Hardening
#
# Provides PostgreSQL parameterized query support to prevent SQL injection attacks
# Uses psql's native parameter binding via -v variables
#
# CRITICAL: All SQL queries that include user input MUST use these functions


# Prevent multiple sourcing
[[ -n "${NSELF_SAFE_QUERY_LOADED:-}" ]] && return 0

set -euo pipefail

NSELF_SAFE_QUERY_LOADED=1

# ============================================================================
# PostgreSQL Connection Helpers
# ============================================================================

# Get PostgreSQL container name
# Returns: Container name or error
pg_get_container() {
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  echo "$container"
  return 0
}

# ============================================================================
# Safe Query Execution (Parameterized Queries)
# ============================================================================

# Execute a parameterized query using psql -v for variable binding
# Usage: pg_query_safe <query> [param1] [param2] ... [paramN]
# Example: pg_query_safe "SELECT * FROM users WHERE email = :'email'" "$user_email"
#
# IMPORTANT: Use :'var' for string parameters and :var for numeric parameters
# This prevents SQL injection by using PostgreSQL's parameter binding
pg_query_safe() {
  local query="$1"
  shift

  local container
  container=$(pg_get_container) || return 1

  # Build psql command as array to prevent command injection
  local psql_cmd=(psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t)

  # Add parameters as psql variables
  local param_num=1
  for param in "$@"; do
    # Escape single quotes in parameter value by doubling them
    local escaped_param="${param//\'/\'\'}"
    psql_cmd+=(-v "param${param_num}=${escaped_param}")
    ((param_num++))
  done

  # Execute query with properly quoted array expansion
  docker exec -i "$container" "${psql_cmd[@]}" -c "$query" 2>/dev/null
  return $?
}

# Execute a parameterized query and return single value
# Usage: pg_query_value <query> [param1] [param2] ...
# Returns: Single trimmed value
pg_query_value() {
  local result
  result=$(pg_query_safe "$@" | xargs)
  echo "$result"
}

# Execute a parameterized query and return JSON
# Usage: pg_query_json <query> [param1] [param2] ...
# Returns: JSON string
pg_query_json() {
  local result
  result=$(pg_query_safe "$@" | xargs)

  if [[ -z "$result" ]] || [[ "$result" == "null" ]]; then
    echo "{}"
  else
    echo "$result"
  fi
}

# Execute a parameterized query and return JSON array
# Usage: pg_query_json_array <query> [param1] [param2] ...
# Returns: JSON array
pg_query_json_array() {
  local result
  result=$(pg_query_safe "$@" | xargs)

  if [[ -z "$result" ]] || [[ "$result" == "null" ]]; then
    echo "[]"
  else
    echo "$result"
  fi
}

# ============================================================================
# Input Sanitization Functions
# ============================================================================

# Escape string for use in SQL (doubles single quotes)
# Usage: sql_escape <string>
# Note: Prefer parameterized queries over manual escaping
sql_escape() {
  local input="$1"
  echo "${input//\'/\'\'}"
}

# Validate and sanitize UUID
# Usage: validate_uuid <uuid>
# Returns: Validated UUID or exits with error
validate_uuid() {
  local uuid="$1"

  if [[ ! "$uuid" =~ ^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$ ]]; then
    echo "ERROR: Invalid UUID format: $uuid" >&2
    return 1
  fi

  echo "$uuid"
  return 0
}

# Validate and sanitize email
# Usage: validate_email <email>
# Returns: Validated email or exits with error
validate_email() {
  local email="$1"

  if [[ ! "$email" =~ ^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$ ]]; then
    echo "ERROR: Invalid email format: $email" >&2
    return 1
  fi

  # Additional length check
  if [[ ${#email} -gt 254 ]]; then
    echo "ERROR: Email too long (max 254 characters)" >&2
    return 1
  fi

  echo "$email"
  return 0
}

# Validate integer
# Usage: validate_integer <value> [min] [max]
# Returns: Validated integer or exits with error
validate_integer() {
  local value="$1"
  local min="${2:-}"
  local max="${3:-}"

  if [[ ! "$value" =~ ^-?[0-9]+$ ]]; then
    echo "ERROR: Invalid integer: $value" >&2
    return 1
  fi

  if [[ -n "$min" ]] && [[ "$value" -lt "$min" ]]; then
    echo "ERROR: Value $value below minimum $min" >&2
    return 1
  fi

  if [[ -n "$max" ]] && [[ "$value" -gt "$max" ]]; then
    echo "ERROR: Value $value above maximum $max" >&2
    return 1
  fi

  echo "$value"
  return 0
}

# Validate alphanumeric identifier (for names, etc)
# Usage: validate_identifier <value> [max_length]
# Returns: Validated identifier or exits with error
validate_identifier() {
  local value="$1"
  local max_length="${2:-100}"

  if [[ ! "$value" =~ ^[a-zA-Z0-9_-]+$ ]]; then
    echo "ERROR: Invalid identifier (use only letters, numbers, underscore, hyphen): $value" >&2
    return 1
  fi

  if [[ ${#value} -gt $max_length ]]; then
    echo "ERROR: Identifier too long (max $max_length characters)" >&2
    return 1
  fi

  echo "$value"
  return 0
}

# Sanitize JSON input (validate it's valid JSON)
# Usage: validate_json <json_string>
# Returns: Validated JSON or exits with error
validate_json() {
  local json="$1"

  # Try to parse with jq
  if ! echo "$json" | jq . >/dev/null 2>&1; then
    echo "ERROR: Invalid JSON format" >&2
    return 1
  fi

  echo "$json"
  return 0
}

# ============================================================================
# Safe Query Builders (Common Patterns)
# ============================================================================

# Safe SELECT by ID
# Usage: pg_select_by_id <table> <id_column> <id_value> [columns]
# Example: pg_select_by_id "auth.users" "id" "$user_id" "id, email, created_at"
pg_select_by_id() {
  local table="$1"
  local id_column="$2"
  local id_value="$3"
  local columns="${4:-*}"

  # Validate UUID if it looks like one
  if [[ "$id_value" =~ ^[0-9a-f]{8}- ]]; then
    id_value=$(validate_uuid "$id_value") || return 1
  fi

  local query="SELECT $columns FROM $table WHERE $id_column = :'param1' LIMIT 1"
  pg_query_json "$query" "$id_value"
}

# Safe INSERT returning ID
# Usage: pg_insert_returning_id <table> <columns> <values...>
# Example: pg_insert_returning_id "auth.users" "email, phone" "$email" "$phone"
pg_insert_returning_id() {
  local table="$1"
  local columns="$2"
  shift 2

  # Build placeholders (:'param1', :'param2', etc.)
  local placeholders=""
  local param_num=1
  for _ in "$@"; do
    if [[ -n "$placeholders" ]]; then
      placeholders+=", "
    fi
    placeholders+=":'param${param_num}'"
    ((param_num++))
  done

  local query="INSERT INTO $table ($columns) VALUES ($placeholders) RETURNING id"
  pg_query_value "$query" "$@"
}

# Safe UPDATE by ID
# Usage: pg_update_by_id <table> <id_column> <id_value> <update_columns> <values...>
# Example: pg_update_by_id "auth.users" "id" "$user_id" "email, phone" "$new_email" "$new_phone"
pg_update_by_id() {
  local table="$1"
  local id_column="$2"
  local id_value="$3"
  local update_columns="$4"
  shift 4

  # Validate UUID if it looks like one
  if [[ "$id_value" =~ ^[0-9a-f]{8}- ]]; then
    id_value=$(validate_uuid "$id_value") || return 1
  fi

  # Build SET clause (col1 = :'param1', col2 = :'param2', etc.)
  IFS=',' read -ra cols <<<"$update_columns"
  local set_clause=""
  local param_num=1
  for col in "${cols[@]}"; do
    col=$(echo "$col" | xargs) # trim whitespace
    if [[ -n "$set_clause" ]]; then
      set_clause+=", "
    fi
    set_clause+="$col = :'param${param_num}'"
    ((param_num++))
  done

  # Add ID as last parameter
  local values=("$@")
  values+=("$id_value")

  local query="UPDATE $table SET $set_clause WHERE $id_column = :'param${param_num}'"
  pg_query_safe "$query" "${values[@]}"
}

# Safe DELETE by ID
# Usage: pg_delete_by_id <table> <id_column> <id_value>
# Example: pg_delete_by_id "auth.users" "id" "$user_id"
pg_delete_by_id() {
  local table="$1"
  local id_column="$2"
  local id_value="$3"

  # Validate UUID if it looks like one
  if [[ "$id_value" =~ ^[0-9a-f]{8}- ]]; then
    id_value=$(validate_uuid "$id_value") || return 1
  fi

  local query="DELETE FROM $table WHERE $id_column = :'param1'"
  pg_query_safe "$query" "$id_value"
}

# Safe COUNT query
# Usage: pg_count <table> [where_column] [where_value]
# Example: pg_count "auth.users" "deleted_at IS NULL"
pg_count() {
  local table="$1"
  local where_clause="${2:-}"
  local where_value="${3:-}"

  local query="SELECT COUNT(*) FROM $table"

  if [[ -n "$where_clause" ]]; then
    query+=" WHERE $where_clause"
  fi

  if [[ -n "$where_value" ]]; then
    pg_query_value "$query" "$where_value"
  else
    pg_query_value "$query"
  fi
}

# Safe EXISTS check
# Usage: pg_exists <table> <column> <value>
# Example: pg_exists "auth.users" "email" "$email"
# Returns: "t" or "f"
pg_exists() {
  local table="$1"
  local column="$2"
  local value="$3"

  local query="SELECT EXISTS(SELECT 1 FROM $table WHERE $column = :'param1' LIMIT 1)"
  pg_query_value "$query" "$value"
}

# ============================================================================
# Transaction Support
# ============================================================================

# Begin transaction
# Usage: pg_begin
pg_begin() {
  local container
  container=$(pg_get_container) || return 1

  # Use array for safe command execution
  local psql_cmd=(psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c "BEGIN;")
  docker exec -i "$container" "${psql_cmd[@]}" >/dev/null 2>&1
}

# Commit transaction
# Usage: pg_commit
pg_commit() {
  local container
  container=$(pg_get_container) || return 1

  # Use array for safe command execution
  local psql_cmd=(psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c "COMMIT;")
  docker exec -i "$container" "${psql_cmd[@]}" >/dev/null 2>&1
}

# Rollback transaction
# Usage: pg_rollback
pg_rollback() {
  local container
  container=$(pg_get_container) || return 1

  # Use array for safe command execution
  local psql_cmd=(psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c "ROLLBACK;")
  docker exec -i "$container" "${psql_cmd[@]}" >/dev/null 2>&1
}

# ============================================================================
# Export Functions
# ============================================================================

export -f pg_get_container
export -f pg_query_safe
export -f pg_query_value
export -f pg_query_json
export -f pg_query_json_array
export -f sql_escape
export -f validate_uuid
export -f validate_email
export -f validate_integer
export -f validate_identifier
export -f validate_json
export -f pg_select_by_id
export -f pg_insert_returning_id
export -f pg_update_by_id
export -f pg_delete_by_id
export -f pg_count
export -f pg_exists
export -f pg_begin
export -f pg_commit
export -f pg_rollback

# Mark as loaded
readonly NSELF_SAFE_QUERY_LOADED
