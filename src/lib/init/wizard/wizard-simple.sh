#!/usr/bin/env bash

# wizard-simple.sh - Simplified wizard using smart defaults
# Only asks for essential information, everything else uses smart defaults

# Get script directory
WIZARD_SIMPLE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail


# Simple project setup - only asks for project name and domain
prompt_project_info_simple() {
  local config_array_name="${1:-config}"

  echo "Let's set up your project with smart defaults."
  echo ""

  # Project name - must be valid for Docker and DNS
  local project_name
  local default_name=$(basename "$PWD" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9-]/-/g' | sed 's/^-*//;s/-*$//' | sed 's/--*/-/g')
  # Ensure default is valid
  if ! echo "$default_name" | grep -q '^[a-z0-9][a-z0-9-]*[a-z0-9]$'; then
    default_name="myproject"
  fi
  prompt_input "Project name (lowercase, alphanumeric, hyphens only)" "$default_name" project_name "^[a-z0-9][a-z0-9-]*[a-z0-9]$"
  eval "$config_array_name+=('PROJECT_NAME=$project_name')"

  # Environment - use smart default
  eval "$config_array_name+=('ENV=\${NSELF_ENV:-dev}')"

  # Domain - use smart default based on environment
  local default_domain="localhost"
  if [[ "${NSELF_ENV:-dev}" == "prod" ]]; then
    default_domain="example.com"
  elif [[ "${NSELF_ENV:-dev}" == "staging" ]]; then
    default_domain="staging.example.com"
  fi

  local base_domain
  prompt_input "Base domain" "$default_domain" base_domain
  eval "$config_array_name+=('BASE_DOMAIN=$base_domain')"

  echo ""
  echo "✓ Basic configuration set. Using smart defaults for everything else."
}

# Simple service selection - just enable/disable optional services
prompt_services_simple() {
  local config_array_name="${1:-config}"

  echo "Which optional services do you want to enable?"
  echo "(Core services are always enabled)"
  echo ""

  # Core services are always enabled (no need to ask)
  eval "$config_array_name+=('# Core Services (always enabled)')"
  eval "$config_array_name+=('POSTGRES_ENABLED=true')"
  eval "$config_array_name+=('HASURA_ENABLED=true')"
  eval "$config_array_name+=('AUTH_ENABLED=true')"
  eval "$config_array_name+=('NGINX_ENABLED=true')"

  echo ""
  eval "$config_array_name+=('# Optional Services')"

  # Optional services - ask with defaults
  if confirm_action "Enable Admin Dashboard? (recommended)" "y"; then
    eval "$config_array_name+=('NSELF_ADMIN_ENABLED=true')"
  fi

  if confirm_action "Enable Storage (MinIO)?" "y"; then
    eval "$config_array_name+=('MINIO_ENABLED=true')"
  fi

  if confirm_action "Enable Redis cache?" "y"; then
    eval "$config_array_name+=('REDIS_ENABLED=true')"
  fi

  if confirm_action "Enable Search (Meilisearch)?" "n"; then
    eval "$config_array_name+=('MEILISEARCH_ENABLED=true')"
  fi

  if confirm_action "Enable Email (MailPit for dev)?" "y"; then
    eval "$config_array_name+=('MAILPIT_ENABLED=true')"
  fi

  if confirm_action "Enable ML tracking (MLflow)?" "n"; then
    eval "$config_array_name+=('MLFLOW_ENABLED=true')"
  fi

  if confirm_action "Enable Functions runtime?" "n"; then
    eval "$config_array_name+=('FUNCTIONS_ENABLED=true')"
  fi

  # Monitoring bundle
  echo ""
  if confirm_action "Enable full monitoring bundle? (10 services)" "n"; then
    eval "$config_array_name+=('MONITORING_ENABLED=true')"
  fi

  # Custom services - simplified
  echo ""
  if confirm_action "Add custom services?" "n"; then
    prompt_custom_services_simple "$config_array_name"
  fi

  # Frontend apps - simplified
  echo ""
  if confirm_action "Add frontend applications?" "y"; then
    prompt_frontend_apps_simple "$config_array_name"
  fi
}

# Simple custom service prompt
prompt_custom_services_simple() {
  local config_array_name="$1"
  local service_count=0

  echo ""
  echo "Add custom services (up to 10):"
  echo "Format: name:template:port"
  echo "Templates: express-js, fastapi, bullmq-js, grpc, go-api, python-api"
  echo ""

  while [[ $service_count -lt 10 ]]; do
    service_count=$((service_count + 1))

    local service_def
    prompt_input "Service $service_count (or press Enter to finish)" "" service_def

    if [[ -z "$service_def" ]]; then
      break
    fi

    # Parse and validate format
    if [[ "$service_def" =~ ^([a-z0-9-]+):([a-z-]+):([0-9]+)$ ]]; then
      eval "$config_array_name+=('CS_${service_count}=$service_def')"
    else
      echo "Invalid format. Use: name:template:port"
      service_count=$((service_count - 1))
    fi
  done
}

# Simple frontend app prompt
prompt_frontend_apps_simple() {
  local config_array_name="$1"
  local app_count=0

  echo ""
  echo "Add frontend applications (they run outside Docker):"
  echo ""

  while [[ $app_count -lt 5 ]]; do
    app_count=$((app_count + 1))

    local app_name
    prompt_input "App $app_count name (or press Enter to finish)" "" app_name

    if [[ -z "$app_name" ]]; then
      break
    fi

    local app_port=$((2999 + app_count))
    eval "$config_array_name+=('FRONTEND_APP_${app_count}_NAME=$app_name')"
    eval "$config_array_name+=('FRONTEND_APP_${app_count}_PORT=$app_port')"
    eval "$config_array_name+=('FRONTEND_APP_${app_count}_ROUTE=$app_name')"
  done
}

# Simple configuration review
review_configuration_simple() {
  local config_array_name="${1:-config}"

  echo "Configuration complete!"
  echo ""
  echo "Summary:"
  echo "--------"

  # Show only key settings
  eval "local config_items=(\"\${${config_array_name}[@]}\")"
  for item in "${config_items[@]}"; do
    case "$item" in
      PROJECT_NAME=* | BASE_DOMAIN=* | ENV=*)
        echo "  $item"
        ;;
      *_ENABLED=true)
        echo "  $item"
        ;;
      CS_* | FRONTEND_APP_*_NAME=*)
        echo "  $item"
        ;;
    esac
  done

  echo ""
  echo "All other settings will use smart defaults."
  echo "You can customize them later in .env if needed."
}

# Run simplified wizard
run_simple_wizard() {
  local output_file="${1:-.env}"

  # Set up trap for Ctrl+C
  trap 'echo ""; echo "Wizard cancelled"; exit 0' INT TERM

  # Configuration array
  local config=()

  clear
  echo "╔════════════════════════════════════════════════════════╗"
  echo "║ nself Quick Setup Wizard                               ║"
  echo "║ Smart defaults for faster setup                        ║"
  echo "╚════════════════════════════════════════════════════════╝"
  echo ""

  # Step 1: Project info
  prompt_project_info_simple config
  echo ""

  # Step 2: Services
  prompt_services_simple config
  echo ""

  # Step 3: Review
  review_configuration_simple config
  echo ""

  if confirm_action "Save configuration?" "y"; then
    # Write minimal config
    {
      echo "# nself Configuration - Using Smart Defaults"
      echo "# Generated: $(date)"
      echo "# Most settings use smart defaults from src/lib/config/smart-defaults.sh"
      echo ""
      echo "# Project Settings"
      for item in "${config[@]}"; do
        echo "$item"
      done
      echo ""
      echo "# Smart defaults are applied for:"
      echo "# - Database configuration (optimal settings based on resources)"
      echo "# - Service passwords (auto-generated secure values)"
      echo "# - Port assignments (conflict-free defaults)"
      echo "# - JWT secrets (cryptographically secure)"
      echo "# - SSL certificates (auto-generated)"
      echo ""
      echo "# To override any default, uncomment and set below:"
      echo "# POSTGRES_PASSWORD=custom-password"
      echo "# HASURA_GRAPHQL_ADMIN_SECRET=custom-secret"
      echo "# REDIS_PASSWORD=custom-redis-password"
    } >"$output_file"

    # Auto-generate strong secrets for enabled services
    if command -v auto_generate_secrets_for_env >/dev/null 2>&1; then
      auto_generate_secrets_for_env "$output_file"
    fi

    echo "✅ Configuration saved to $output_file"
    echo ""
    echo "Next steps:"
    echo "  1. Run: nself build"
    echo "  2. Run: nself start"
  else
    echo "Configuration not saved."
  fi

  return 0
}

# Helper to confirm with default
confirm_action() {
  local prompt="$1"
  local default="${2:-n}"
  local response

  if [[ "$default" == "y" ]]; then
    printf "%s" "$prompt [Y/n]: "
  else
    printf "%s" "$prompt [y/N]: "
  fi

  read -r response
  response=${response:-$default}

  # Convert to lowercase (Bash 3.2 compatible)
  response=$(echo "$response" | tr '[:upper:]' '[:lower:]')
  [[ "$response" == "y" ]] || [[ "$response" == "yes" ]]
}

# Export functions
export -f prompt_project_info_simple
export -f prompt_services_simple
export -f prompt_custom_services_simple
export -f prompt_frontend_apps_simple
export -f review_configuration_simple
export -f run_simple_wizard
export -f confirm_action
