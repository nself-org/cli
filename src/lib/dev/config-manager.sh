#!/usr/bin/env bash
# config-manager.sh - Enhanced configuration management
# Part of nself v0.7.0 - Sprint 10: DEV-002


# Validate configuration
config_validate() {

set -euo pipefail

  local config_file="${1:-.env}"

  [[ ! -f "$config_file" ]] && {
    echo "ERROR: Config file not found: $config_file" >&2
    return 1
  }

  local errors="[]"

  # Check required variables
  local required_vars=("PROJECT_NAME" "ENV" "BASE_DOMAIN" "POSTGRES_DB" "POSTGRES_USER" "POSTGRES_PASSWORD")

  for var in "${required_vars[@]}"; do
    if ! grep -q "^${var}=" "$config_file" 2>/dev/null; then
      errors=$(echo "$errors" | jq --arg var "$var" '. += [{"type":"missing","variable":$var}]')
    fi
  done

  # Check for common mistakes
  if grep -q "localhost" "$config_file" 2>/dev/null && grep -q "ENV=prod" "$config_file" 2>/dev/null; then
    errors=$(echo "$errors" | jq '. += [{"type":"warning","message":"Using localhost in production"}]')
  fi

  if [[ "$(echo "$errors" | jq 'length')" -gt 0 ]]; then
    echo "✗ Configuration validation failed:"
    echo "$errors" | jq '.'
    return 1
  else
    echo "✓ Configuration valid"
    return 0
  fi
}

# Configuration templates
config_create_template() {
  local template_type="$1" # minimal, development, production
  local output_file="${2:-.env.template}"

  case "$template_type" in
    minimal)
      cat >"$output_file" <<'EOF'
# Minimal nself configuration
PROJECT_NAME=myapp
ENV=dev
BASE_DOMAIN=localhost

POSTGRES_DB=myapp_db
POSTGRES_USER=postgres
POSTGRES_PASSWORD=changeme
EOF
      ;;

    development)
      cat >"$output_file" <<'EOF'
# Development configuration
PROJECT_NAME=myapp
ENV=dev
BASE_DOMAIN=localhost

# Required Services
POSTGRES_DB=myapp_db
POSTGRES_USER=postgres
POSTGRES_PASSWORD=dev_password
HASURA_GRAPHQL_ADMIN_SECRET=dev_secret

# Optional Services
REDIS_ENABLED=true
MINIO_ENABLED=true
NSELF_ADMIN_ENABLED=true
MAILPIT_ENABLED=true

# Monitoring
MONITORING_ENABLED=true
EOF
      ;;

    production)
      cat >"$output_file" <<'EOF'
# Production configuration
PROJECT_NAME=myapp
ENV=prod
BASE_DOMAIN=example.com

# Required Services (use strong passwords!)
POSTGRES_DB=myapp_db
POSTGRES_USER=postgres
POSTGRES_PASSWORD=CHANGE_ME_STRONG_PASSWORD
HASURA_GRAPHQL_ADMIN_SECRET=CHANGE_ME_STRONG_SECRET

# SSL
SSL_ENABLED=true
DNS_PROVIDER=cloudflare
DNS_API_TOKEN=your_cloudflare_token

# Optional Services
REDIS_ENABLED=true
MINIO_ENABLED=true

# Monitoring
MONITORING_ENABLED=true
EOF
      ;;
  esac

  echo "✓ Configuration template created: $output_file"
}

# Configuration migration
config_migrate() {
  local old_version="$1"
  local new_version="$2"

  echo "Migrating configuration from v$old_version to v$new_version..."

  case "$old_version-$new_version" in
    "0.5.0-0.6.0")
      # Add new Redis variables if not present
      if ! grep -q "^REDIS_ENABLED=" .env 2>/dev/null; then
        echo "" >>.env
        echo "# Redis (added in v0.6.0)" >>.env
        echo "REDIS_ENABLED=false" >>.env
      fi
      ;;

    *)
      echo "No migration needed"
      ;;
  esac

  echo "✓ Configuration migrated"
}

# Show configuration diff
config_diff() {
  local file1="$1"
  local file2="$2"

  diff -u "$file1" "$file2" || true
}

# Export configuration as JSON
config_export_json() {
  local config_file="${1:-.env}"
  local output_file="${2:-config.json}"

  # Parse .env file to JSON
  local json="{}"

  while IFS='=' read -r key value; do
    # Skip comments and empty lines
    [[ "$key" =~ ^#.*$ ]] && continue
    [[ -z "$key" ]] && continue

    # Remove quotes from value
    value=$(echo "$value" | sed 's/^["'\'']\|["'\'']$//g')

    json=$(echo "$json" | jq --arg k "$key" --arg v "$value" '. + {($k): $v}')
  done <"$config_file"

  echo "$json" | jq '.' >"$output_file"
  echo "✓ Configuration exported to $output_file"
}

export -f config_validate config_create_template config_migrate config_diff config_export_json
