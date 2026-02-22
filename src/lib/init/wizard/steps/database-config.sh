#!/usr/bin/env bash

# database-config.sh - Wizard step for database configuration
# POSIX-compliant, no Bash 4+ features

# Configure database settings
wizard_database_config() {

set -euo pipefail

  local config_array_name="$1"
  local project_name="${2:-myproject}"

  clear
  show_wizard_step 2 10 "Database Configuration"

  echo "🗄 PostgreSQL Database"
  echo ""

  # Database Name
  local postgres_db
  prompt_input "Database name" "$project_name" postgres_db "^[a-z][a-z0-9_]*$"
  eval "$config_array_name+=('POSTGRES_DB=$postgres_db')"

  echo ""

  # Database User
  local postgres_user
  prompt_input "Database user" "postgres" postgres_user "^[a-z][a-z0-9_]*$"
  eval "$config_array_name+=('POSTGRES_USER=$postgres_user')"

  echo ""

  # Database Password
  local postgres_password
  if confirm_action "Use auto-generated secure password for database?"; then
    postgres_password=$(generate_password 24)
    echo "Generated password: $postgres_password"
    echo "(This will be saved in .env file)"
  else
    prompt_password "Database password" postgres_password
  fi
  eval "$config_array_name+=('POSTGRES_PASSWORD=$postgres_password')"

  echo ""

  # Port Configuration
  echo "Database port (default: 5432):"
  echo "  Change if you have PostgreSQL already running"
  local postgres_port
  prompt_input "Port" "5432" postgres_port "^[0-9]+$"
  eval "$config_array_name+=('POSTGRES_PORT=$postgres_port')"

  echo ""

  # Advanced Options
  if confirm_action "Configure advanced database options?"; then
    echo ""

    # Connection Pool
    local max_connections
    prompt_input "Max connections" "100" max_connections "^[0-9]+$"
    eval "$config_array_name+=('POSTGRES_MAX_CONNECTIONS=$max_connections')"

    echo ""

    # Shared Buffers
    echo "Shared buffers (PostgreSQL memory):"
    local buffer_options=(
      "256MB - Small projects"
      "512MB - Medium projects"
      "1GB - Large projects"
      "2GB - Enterprise projects"
      "Custom"
    )
    local selected_buffer
    select_option "Select shared buffer size" buffer_options selected_buffer

    local shared_buffers
    case $selected_buffer in
      0) shared_buffers="256MB" ;;
      1) shared_buffers="512MB" ;;
      2) shared_buffers="1GB" ;;
      3) shared_buffers="2GB" ;;
      4)
        prompt_input "Custom size (e.g., 4GB)" "1GB" shared_buffers
        ;;
    esac
    eval "$config_array_name+=('POSTGRES_SHARED_BUFFERS=$shared_buffers')"

    echo ""

    # Extensions
    echo "PostgreSQL extensions to enable:"
    local extensions=""

    if confirm_action "Enable UUID support (uuid-ossp)?"; then
      extensions="${extensions}uuid-ossp,"
    fi

    if confirm_action "Enable crypto functions (pgcrypto)?"; then
      extensions="${extensions}pgcrypto,"
    fi

    if confirm_action "Enable vector search (pgvector)?"; then
      extensions="${extensions}pgvector,"
    fi

    if confirm_action "Enable PostGIS for geospatial?"; then
      extensions="${extensions}postgis,"
    fi

    if confirm_action "Enable full-text search (pg_trgm)?"; then
      extensions="${extensions}pg_trgm,"
    fi

    # Remove trailing comma
    extensions="${extensions%,}"

    if [[ -n "$extensions" ]]; then
      eval "$config_array_name+=('POSTGRES_EXTENSIONS=$extensions')"
    fi
  fi

  echo ""

  # Backup Configuration
  if confirm_action "Enable automatic database backups?"; then
    eval "$config_array_name+=('POSTGRES_BACKUP_ENABLED=true')"

    echo ""
    echo "Backup schedule:"
    local backup_options=(
      "Daily at midnight"
      "Every 6 hours"
      "Every 12 hours"
      "Weekly on Sunday"
      "Custom cron expression"
    )
    local selected_backup
    select_option "Select backup schedule" backup_options selected_backup

    local backup_schedule
    case $selected_backup in
      0) backup_schedule="0 0 * * *" ;;
      1) backup_schedule="0 */6 * * *" ;;
      2) backup_schedule="0 */12 * * *" ;;
      3) backup_schedule="0 0 * * 0" ;;
      4)
        echo ""
        prompt_input "Cron expression" "0 0 * * *" backup_schedule
        ;;
    esac
    eval "$config_array_name+=('POSTGRES_BACKUP_SCHEDULE=\"$backup_schedule\"')"

    echo ""
    local retention_days
    prompt_input "Backup retention (days)" "7" retention_days "^[0-9]+$"
    eval "$config_array_name+=('POSTGRES_BACKUP_RETENTION_DAYS=$retention_days')"
  else
    eval "$config_array_name+=('POSTGRES_BACKUP_ENABLED=false')"
  fi

  return 0
}

# Generate secure password
generate_password() {
  local length="${1:-16}"
  if command -v openssl >/dev/null 2>&1; then
    # Request enough bytes to produce at least $length chars after filtering.
    # Base64 of N bytes = ~4N/3 chars; extra 16 ensures we have enough after
    # removing =, +, / and newlines (which openssl inserts every 64 chars).
    local bytes=$(( (length * 4 / 3) + 16 ))
    openssl rand -base64 "$bytes" | tr -d "=+/\n" | head -c "$length"
  else
    # Fallback to /dev/urandom
    LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c "$length"
  fi
}

# Export functions
export -f wizard_database_config
export -f generate_password
