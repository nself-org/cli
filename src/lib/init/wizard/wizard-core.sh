#!/usr/bin/env bash

# wizard-core.sh - Core wizard orchestration using modules
# POSIX-compliant, no Bash 4+ features

# Get script directory
WIZARD_CORE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

STEPS_DIR="$WIZARD_CORE_DIR/steps"

# Source step modules
source_wizard_steps() {
  local steps_dir="${1:-$STEPS_DIR}"

  # Source each step module
  for module in "$steps_dir"/*.sh; do
    if [[ -f "$module" ]]; then
      source "$module"
    fi
  done
}

# Run the configuration wizard
run_modular_wizard() {
  local output_file="${1:-.env}"

  # Set up trap for Ctrl+C
  trap 'echo ""; echo ""; log_info "Wizard cancelled"; echo "Run nself init --wizard to try again."; echo ""; exit 0' INT TERM

  # Configuration array
  local config=()

  # Source all wizard steps
  source_wizard_steps

  # Welcome screen
  clear
  show_wizard_header "nself Configuration Wizard" "Setup Your Project Step by Step"

  echo "Welcome to nself! Let's configure your project."
  echo "This wizard will walk you through the essential settings."
  echo ""
  echo "📝 We'll configure:"
  echo "  • Project name and domain"
  echo "  • Database settings"
  echo "  • Core services"
  echo "  • Optional services"
  echo "  • Frontend applications"
  echo ""
  echo "(Press Ctrl+C anytime to exit)"
  echo ""
  press_any_key

  # Step 1: Core Settings
  wizard_core_settings config

  # Extract project name for use in other steps
  local project_name=""
  for item in "${config[@]}"; do
    if [[ "$item" == PROJECT_NAME=* ]]; then
      project_name="${item#PROJECT_NAME=}"
      break
    fi
  done

  # Step 2: Database Configuration
  wizard_database_config config "$project_name"

  # Step 3: Core Services
  wizard_core_services config

  # Step 4: Service Passwords
  wizard_service_passwords config

  # Step 5: Admin Dashboard
  wizard_admin_dashboard config

  # Step 6: Optional Services
  wizard_optional_services config

  # Step 7: Email & Search
  wizard_email_search config

  # Step 8: Custom Backend Services
  wizard_custom_services config

  # Step 9: Frontend Applications
  wizard_frontend_apps config

  # Step 10: Review and Generate
  wizard_review_generate config "$output_file"

  return 0
}

# Configure service passwords
wizard_service_passwords() {
  local config_array_name="$1"

  clear
  show_wizard_step 4 10 "Service Passwords"

  echo "🔑 Service Authentication"
  echo ""
  echo "Set secure passwords for services:"
  echo ""

  # Hasura Admin Secret
  local hasura_enabled=false
  for item in "${!config_array_name}[@]"; do
    eval "local cfg_item=\${${config_array_name}[$item]}"
    if [[ "$cfg_item" == "HASURA_ENABLED=true" ]]; then
      hasura_enabled=true
      break
    fi
  done

  if [[ "$hasura_enabled" == "true" ]]; then
    echo "Hasura Admin Secret:"
    local hasura_secret
    if confirm_action "Use auto-generated secure password?"; then
      hasura_secret=$(generate_password 32)
      echo "Generated: $hasura_secret"
    else
      prompt_password "Admin secret" hasura_secret
    fi
    eval "$config_array_name+=('HASURA_ADMIN_SECRET=$hasura_secret')"
    echo ""
  fi

  # JWT Secret
  echo "JWT Secret (for authentication tokens):"
  local jwt_secret
  if confirm_action "Use auto-generated secure secret?"; then
    jwt_secret=$(generate_password 64)
    echo "Generated: [hidden for security]"
  else
    prompt_password "JWT secret" jwt_secret
  fi
  eval "$config_array_name+=('JWT_SECRET=$jwt_secret')"

  echo ""

  # Storage/MinIO credentials
  local storage_enabled=false
  for item in "${!config_array_name}[@]"; do
    eval "local cfg_item=\${${config_array_name}[$item]}"
    if [[ "$cfg_item" == "STORAGE_ENABLED=true" ]]; then
      storage_enabled=true
      break
    fi
  done

  if [[ "$storage_enabled" == "true" ]]; then
    echo "Storage Service Credentials:"
    local storage_access_key storage_secret_key

    prompt_input "Access key" "minioadmin" storage_access_key
    if confirm_action "Use auto-generated secret key?"; then
      storage_secret_key=$(generate_password 40)
      echo "Generated: $storage_secret_key"
    else
      prompt_password "Secret key" storage_secret_key
    fi

    eval "$config_array_name+=('STORAGE_ACCESS_KEY=$storage_access_key')"
    eval "$config_array_name+=('STORAGE_SECRET_KEY=$storage_secret_key')"
    echo ""
  fi

  return 0
}

# Configure admin dashboard
wizard_admin_dashboard() {
  local config_array_name="$1"

  clear
  show_wizard_step 5 10 "Admin Dashboard"

  echo "🎛 Admin Dashboard"
  echo ""

  if confirm_action "Enable nself admin dashboard?"; then
    eval "$config_array_name+=('NSELF_ADMIN_ENABLED=true')"
    eval "$config_array_name+=('NSELF_ADMIN_PORT=3021')"

    echo ""
    echo "Dashboard authentication:"
    local admin_user admin_password

    prompt_input "Admin username" "admin" admin_user
    if confirm_action "Use auto-generated password?"; then
      admin_password=$(generate_password 16)
      echo "Generated password: $admin_password"
      echo "(Save this password - you'll need it to login)"
    else
      prompt_password "Admin password" admin_password
    fi

    eval "$config_array_name+=('NSELF_ADMIN_USER=$admin_user')"
    eval "$config_array_name+=('NSELF_ADMIN_PASSWORD=$admin_password')"

    echo ""
    echo "Dashboard features to enable:"

    if confirm_action "Enable real-time monitoring?"; then
      eval "$config_array_name+=('NSELF_ADMIN_MONITORING=true')"
    fi

    if confirm_action "Enable log viewer?"; then
      eval "$config_array_name+=('NSELF_ADMIN_LOGS=true')"
    fi

    if confirm_action "Enable database manager?"; then
      eval "$config_array_name+=('NSELF_ADMIN_DATABASE=true')"
    fi
  else
    eval "$config_array_name+=('NSELF_ADMIN_ENABLED=false')"
  fi

  return 0
}

# Configure custom backend services
wizard_custom_services() {
  local config_array_name="$1"

  clear
  show_wizard_step 8 10 "Custom Backend Services"

  echo "🔧 Custom Backend Services"
  echo ""

  if confirm_action "Add custom backend services?"; then
    local service_count=0
    local add_more=true

    while [[ "$add_more" == "true" ]] && [[ $service_count -lt 10 ]]; do
      service_count=$((service_count + 1))
      echo ""
      echo "Service #$service_count:"

      local service_name service_type service_port

      prompt_input "Service name" "service-$service_count" service_name "^[a-z][a-z0-9-_]*$"

      echo ""
      echo "Service type:"
      local type_options=(
        "express-js - Express.js REST API"
        "fastapi - Python FastAPI"
        "bullmq-js - BullMQ job processor"
        "grpc - gRPC service"
        "Custom Docker image"
      )
      local selected_type
      select_option "Select type" type_options selected_type

      case $selected_type in
        0) service_type="express-js" ;;
        1) service_type="fastapi" ;;
        2) service_type="bullmq-js" ;;
        3) service_type="grpc" ;;
        4)
          echo ""
          prompt_input "Docker image" "node:18" service_type
          ;;
      esac

      echo ""
      prompt_input "Service port" "$((8000 + service_count))" service_port "^[0-9]+$"

      eval "$config_array_name+=('CUSTOM_SERVICE_${service_count}=${service_name}:${service_type}:${service_port}')"

      echo ""
      if ! confirm_action "Add another service?"; then
        add_more=false
      fi
    done

    eval "$config_array_name+=('CUSTOM_SERVICES_COUNT=$service_count')"
  else
    eval "$config_array_name+=('CUSTOM_SERVICES_COUNT=0')"
  fi

  return 0
}

# Configure frontend applications
wizard_frontend_apps() {
  local config_array_name="$1"

  clear
  show_wizard_step 9 10 "Frontend Applications"

  echo "🎨 Frontend Applications"
  echo ""

  if confirm_action "Add frontend applications?"; then
    local app_count=0
    local add_more=true

    while [[ "$add_more" == "true" ]] && [[ $app_count -lt 10 ]]; do
      app_count=$((app_count + 1))
      echo ""
      echo "Frontend App #$app_count:"

      local app_name app_framework app_port

      prompt_input "App name" "app$app_count" app_name "^[a-z][a-z0-9-]*$"

      echo ""
      echo "Framework:"
      local framework_options=(
        "Next.js - React framework with SSR"
        "React - Create React App"
        "Vue.js - Progressive framework"
        "Angular - Enterprise framework"
        "Svelte - Compiled framework"
        "Static HTML - Plain HTML/CSS/JS"
      )
      local selected_framework
      select_option "Select framework" framework_options selected_framework

      case $selected_framework in
        0) app_framework="nextjs" ;;
        1) app_framework="react" ;;
        2) app_framework="vue" ;;
        3) app_framework="angular" ;;
        4) app_framework="svelte" ;;
        5) app_framework="static" ;;
      esac

      echo ""
      prompt_input "App port" "$((3000 + app_count - 1))" app_port "^[0-9]+$"

      eval "$config_array_name+=('FRONTEND_APP_${app_count}_NAME=$app_name')"
      eval "$config_array_name+=('FRONTEND_APP_${app_count}_FRAMEWORK=$app_framework')"
      eval "$config_array_name+=('FRONTEND_APP_${app_count}_PORT=$app_port')"
      # Frontend apps are external - no DIR needed

      echo ""
      if ! confirm_action "Add another frontend app?"; then
        add_more=false
      fi
    done

    eval "$config_array_name+=('FRONTEND_APP_COUNT=$app_count')"
  else
    eval "$config_array_name+=('FRONTEND_APP_COUNT=0')"
  fi

  return 0
}

# Review and generate configuration
wizard_review_generate() {
  local config_array_name="$1"
  local output_file="$2"

  clear
  show_wizard_step 10 10 "Review Configuration"

  echo "📋 Configuration Summary"
  echo ""
  echo "Review your configuration:"
  echo ""

  # Display configuration
  eval "local config_items=(\"\${${config_array_name}[@]}\")"
  for item in "${config_items[@]}"; do
    echo "  $item"
  done | head -20

  local total_items=${#config_items[@]}
  if [[ $total_items -gt 20 ]]; then
    echo "  ... and $((total_items - 20)) more settings"
  fi

  echo ""
  echo "Configuration will be saved to: $output_file"
  echo ""

  if confirm_action "Generate configuration?"; then
    # Create backup if file exists
    if [[ -f "$output_file" ]]; then
      local backup_file="${output_file}.backup.$(date +%Y%m%d_%H%M%S)"
      cp "$output_file" "$backup_file"
      echo "Backed up existing config to: $backup_file"
    fi

    # Write configuration
    {
      echo "# nself Configuration"
      echo "# Generated by wizard on $(date)"
      echo ""

      for item in "${config_items[@]}"; do
        echo "$item"
      done
    } >"$output_file"

    # Auto-generate strong secrets for enabled services
    if command -v auto_generate_secrets_for_env >/dev/null 2>&1; then
      auto_generate_secrets_for_env "$output_file"
    fi

    echo ""
    echo "✅ Configuration generated successfully!"
    echo ""
    echo "Next steps:"
    echo "  1. Review and edit $output_file if needed"
    echo "  2. Run: nself build"
    echo "  3. Run: nself start"
  else
    echo ""
    echo "Configuration not saved."
    echo "Run 'nself init --wizard' to try again."
  fi

  return 0
}

# Export functions
export -f source_wizard_steps
export -f run_modular_wizard
export -f wizard_service_passwords
export -f wizard_admin_dashboard
export -f wizard_custom_services
export -f wizard_frontend_apps
export -f wizard_review_generate
