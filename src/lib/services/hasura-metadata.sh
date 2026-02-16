#!/usr/bin/env bash

# hasura-metadata.sh - Generate Hasura metadata for multi-app support

# Generate per-app schema configuration
hasura::generate_app_schemas() {

set -euo pipefail

  local base_domain="${BASE_DOMAIN:-localhost}"
  local app_count="${FRONTEND_APP_COUNT:-0}"

  # Collect all app schemas
  local schemas=()

  # Core schemas (auth, storage, public) are already created in core.sh
  # We only need to add app-specific schemas here

  # Add per-app schemas based on table prefixes
  if [[ "$app_count" -gt 0 ]]; then
    for ((i = 1; i <= app_count; i++)); do
      local table_prefix_var="FRONTEND_APP_${i}_TABLE_PREFIX"
      local table_prefix="${!table_prefix_var:-}"

      if [[ -n "$table_prefix" ]]; then
        # Remove trailing underscore if present
        local schema_name="${table_prefix%_}"
        # Ensure it's a valid schema name (lowercase, no special chars except underscore)
        schema_name=$(echo "$schema_name" | tr '[:upper:]' '[:lower:]' | tr -cd '[:alnum:]_')

        if [[ -n "$schema_name" && "$schema_name" != "public" && "$schema_name" != "auth" && "$schema_name" != "storage" ]]; then
          schemas+=("$schema_name")
        fi
      fi
    done
  fi

  # Return unique schemas
  if [ ${#schemas[@]} -gt 0 ]; then
    printf '%s\n' "${schemas[@]}" | sort -u
  fi
}

# Generate remote schema configuration for per-app APIs
hasura::generate_remote_schemas() {
  local base_domain="${BASE_DOMAIN:-localhost}"
  local app_count="${FRONTEND_APP_COUNT:-0}"
  local configs=()

  if [[ "$app_count" -gt 0 ]]; then
    for ((i = 1; i <= app_count; i++)); do
      local remote_schema_name_var="FRONTEND_APP_${i}_REMOTE_SCHEMA_NAME"
      local remote_schema_url_var="FRONTEND_APP_${i}_REMOTE_SCHEMA_URL"
      local system_name_var="FRONTEND_APP_${i}_SYSTEM_NAME"

      local remote_name="${!remote_schema_name_var:-}"
      local remote_url="${!remote_schema_url_var:-}"
      local system_name="${!system_name_var:-}"

      if [[ -n "$remote_url" ]]; then
        # If no explicit remote schema name, generate one
        if [[ -z "$remote_name" ]]; then
          remote_name="${system_name}_schema"
        fi

        # Construct full URL if needed
        local full_url=""
        if [[ "$remote_url" =~ ^https?:// ]]; then
          full_url="$remote_url"
        else
          # Construct URL from pattern like "api.app1" or just "api-service"
          full_url="http://${remote_url}.${base_domain}/graphql"
        fi

        # Output config for this remote schema
        echo "name: $remote_name"
        echo "url: $full_url"
        echo "headers:"
        echo "  X-App-Name: $system_name"
        echo "---"
      fi
    done
  fi
}

# Generate SQL for creating app-specific schemas
hasura::generate_schema_sql() {
  local schemas=()
  local schema

  # Get all schemas
  while IFS= read -r schema; do
    [[ -n "$schema" ]] && schemas+=("$schema")
  done < <(hasura::generate_app_schemas)

  # Generate SQL
  if [ ${#schemas[@]} -gt 0 ]; then
    for schema in "${schemas[@]}"; do
      cat <<EOF
-- Create schema for $schema
CREATE SCHEMA IF NOT EXISTS "$schema";

-- Grant permissions to postgres user
GRANT ALL ON SCHEMA "$schema" TO postgres;
GRANT ALL ON ALL TABLES IN SCHEMA "$schema" TO postgres;
GRANT ALL ON ALL SEQUENCES IN SCHEMA "$schema" TO postgres;
GRANT ALL ON ALL FUNCTIONS IN SCHEMA "$schema" TO postgres;

-- Set default privileges for future objects
ALTER DEFAULT PRIVILEGES IN SCHEMA "$schema"
  GRANT ALL ON TABLES TO postgres;
ALTER DEFAULT PRIVILEGES IN SCHEMA "$schema"
  GRANT ALL ON SEQUENCES TO postgres;
ALTER DEFAULT PRIVILEGES IN SCHEMA "$schema"
  GRANT ALL ON FUNCTIONS TO postgres;

EOF
    done
  fi
}

# Check if an app needs per-app auth tables
hasura::app_needs_isolated_auth() {
  local app_index="$1"
  local isolate_auth_var="FRONTEND_APP_${app_index}_ISOLATE_AUTH"
  local isolate_auth="${!isolate_auth_var:-false}"

  # Check if this app wants isolated auth tables
  [[ "$isolate_auth" == "true" ]]
}

# Generate auth table configuration for apps
hasura::generate_auth_config() {
  local app_count="${FRONTEND_APP_COUNT:-0}"

  # For each app, check if it needs isolated auth
  if [[ "$app_count" -gt 0 ]]; then
    for ((i = 1; i <= app_count; i++)); do
      if hasura::app_needs_isolated_auth "$i"; then
        local table_prefix_var="FRONTEND_APP_${i}_TABLE_PREFIX"
        local table_prefix="${!table_prefix_var:-}"
        local schema_name="${table_prefix%_}"

        if [[ -n "$schema_name" ]]; then
          cat <<EOF
-- Create isolated auth tables for $schema_name app
CREATE TABLE IF NOT EXISTS "${schema_name}".users AS SELECT * FROM auth.users WHERE false;
CREATE TABLE IF NOT EXISTS "${schema_name}".user_providers AS SELECT * FROM auth.user_providers WHERE false;
CREATE TABLE IF NOT EXISTS "${schema_name}".user_security_keys AS SELECT * FROM auth.user_security_keys WHERE false;
CREATE TABLE IF NOT EXISTS "${schema_name}".refresh_tokens AS SELECT * FROM auth.refresh_tokens WHERE false;
CREATE TABLE IF NOT EXISTS "${schema_name}".provider_requests AS SELECT * FROM auth.provider_requests WHERE false;
CREATE TABLE IF NOT EXISTS "${schema_name}".roles AS SELECT * FROM auth.roles WHERE false;
CREATE TABLE IF NOT EXISTS "${schema_name}".user_roles AS SELECT * FROM auth.user_roles WHERE false;

EOF
        fi
      fi
    done
  fi
}

# Generate Hasura metadata JSON for multi-app setup
hasura::generate_metadata_json() {
  local project_name="${PROJECT_NAME:-app}"
  local base_domain="${BASE_DOMAIN:-localhost}"

  cat <<EOF
{
  "version": 3,
  "sources": [
    {
      "name": "default",
      "kind": "postgres",
      "tables": [],
      "configuration": {
        "connection_info": {
          "database_url": {
            "from_env": "HASURA_GRAPHQL_DATABASE_URL"
          },
          "isolation_level": "read-committed",
          "pool_settings": {
            "connection_lifetime": 600,
            "retries": 1,
            "idle_timeout": 180,
            "max_connections": 50
          },
          "use_prepared_statements": true
        }
      }
    }
  ],
  "remote_schemas": [
EOF

  # Add remote schemas
  local first=true
  while IFS= read -r line; do
    if [[ "$line" == "---" ]]; then
      continue
    fi
    if [[ "$line" =~ ^name: ]]; then
      if [[ "$first" != "true" ]]; then
        echo ","
      fi
      echo "    {"
      echo "      \"$line\","
      first=false
    elif [[ "$line" =~ ^url: ]]; then
      echo "      \"$line\","
    elif [[ "$line" == "headers:" ]]; then
      echo "      \"headers\": {"
    elif [[ "$line" =~ ^[[:space:]]+X-App-Name: ]]; then
      local header_value="${line#*: }"
      echo "        \"X-App-Name\": \"$header_value\""
      echo "      }"
      echo "    }"
    fi
  done < <(hasura::generate_remote_schemas)

  cat <<EOF
  ]
}
EOF
}

# Export functions
export -f hasura::generate_app_schemas
export -f hasura::generate_remote_schemas
export -f hasura::generate_schema_sql
export -f hasura::app_needs_isolated_auth
export -f hasura::generate_auth_config
export -f hasura::generate_metadata_json
