#!/usr/bin/env bash

# service-generator-fixed.sh - Service generator that copies templates
# POSIX-compliant, no Bash 4+ features

# Source utilities - don't override parent SCRIPT_DIR
SERVICE_GEN_SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"

set -euo pipefail

source "$SERVICE_GEN_SCRIPT_DIR/../utils/display.sh" 2>/dev/null || true

# Get nself root dynamically
get_nself_root() {
  # Try multiple methods to find root
  if [[ -n "${NSELF_ROOT:-}" ]]; then
    echo "$NSELF_ROOT"
  elif [[ -f "$PWD/.nself-root" ]]; then
    cat "$PWD/.nself-root"
  elif [[ -f "$SERVICE_GEN_SCRIPT_DIR/../../../.nself-root" ]]; then
    cd "$SERVICE_GEN_SCRIPT_DIR/../../.." && pwd
  else
    # Fallback to finding via script location
    cd "$SERVICE_GEN_SCRIPT_DIR/../../.." && pwd
  fi
}

# Get templates directory
get_templates_dir() {
  local nself_root=$(get_nself_root)
  echo "$nself_root/src/templates/services"
}

# Source the template configuration
source "$SERVICE_GEN_SCRIPT_DIR/../config/service-templates.sh" 2>/dev/null || true

# Copy service template
copy_service_template() {
  local service_name="$1"
  local service_type="$2"
  local service_port="${3:-8000}"
  local target_dir="${4:-services/$service_name}"

  local templates_dir=$(get_templates_dir)

  # Get template path using the service-templates.sh mapping
  local template_path=""
  if command -v get_service_template >/dev/null 2>&1; then
    template_path=$(get_service_template "$service_type")
  fi

  # Fallback to direct path if function not available
  if [[ -z "$template_path" ]] || [[ ! -d "$template_path" ]]; then
    case "$service_type" in
      express-js) template_path="$templates_dir/js/express-js" ;;
      fastify-js) template_path="$templates_dir/js/fastify-js" ;;
      nestjs-js | nest-js) template_path="$templates_dir/js/nest-js" ;;
      bullmq-js) template_path="$templates_dir/js/bullmq-js" ;;
      node-js) template_path="$templates_dir/js/node-js" ;;
      fastapi) template_path="$templates_dir/py/fastapi" ;;
      flask) template_path="$templates_dir/py/flask" ;;
      django) template_path="$templates_dir/py/django-rest" ;;
      *)
        echo "Unknown service type: $service_type" >&2
        return 1
        ;;
    esac
  fi

  if [[ ! -d "$template_path" ]]; then
    echo "Template not found: $template_path" >&2
    return 1
  fi

  # Create target directory
  mkdir -p "$target_dir"

  # Copy template files
  cp -r "$template_path"/* "$target_dir/" 2>/dev/null || {
    echo "Failed to copy template files" >&2
    return 1
  }

  # Process template variables
  process_template_variables "$target_dir" "$service_name" "$service_port"

  return 0
}

# Process template variables
process_template_variables() {
  local target_dir="$1"
  local service_name="$2"
  local service_port="${3:-8000}"

  # Process .template files
  find "$target_dir" -type f \( -name "*.template" -o -name "*.tpl" \) 2>/dev/null | while read -r template_file; do
    local output_file="${template_file%.template}"
    output_file="${output_file%.tpl}"

    sed -e "s/{{SERVICE_NAME}}/$service_name/g" \
      -e "s/{{PROJECT_NAME}}/${PROJECT_NAME:-nself}/g" \
      -e "s/{{BASE_DOMAIN}}/${BASE_DOMAIN:-localhost}/g" \
      -e "s/{{PORT}}/$service_port/g" \
      -e "s/{{ENV}}/${ENV:-dev}/g" \
      "$template_file" >"$output_file"

    rm "$template_file"
  done

  # Update package.json if exists (handle both macOS and Linux sed)
  if [[ -f "$target_dir/package.json" ]]; then
    safe_sed_inline "$target_dir/package.json" "s/\"name\": \"[^\"]*\"/\"name\": \"$service_name\"/"
  fi

  # Update go.mod if exists
  if [[ -f "$target_dir/go.mod" ]]; then
    safe_sed_inline "$target_dir/go.mod" "s|module .*|module $service_name|"
  fi

  # Create .env file
  cat >"$target_dir/.env" <<EOF
SERVICE_NAME=$service_name
PORT=$service_port
NODE_ENV=${ENV:-development}
BASE_DOMAIN=${BASE_DOMAIN:-localhost}
EOF
}

# Platform-safe sed inline editing
safe_sed_inline() {
  local file="$1"
  local pattern="$2"

  if [[ "$OSTYPE" == "darwin"* ]]; then
    if [[ "$OSTYPE" == "darwin"* ]]; then
      sed -i '' "$pattern" "$file"
    else
      sed -i "$pattern" "$file"
    fi
  else
    sed -i "$pattern" "$file"
  fi
}

# Generate configured services
generate_configured_services() {
  local services_generated=0
  local services_failed=0

  # Check for Auth service
  if [[ "${AUTH_ENABLED:-false}" == "true" ]]; then
    if [[ "${AUTH_PROVIDER:-}" == "custom" ]]; then
      if [[ ! -d "services/auth" ]]; then
        echo "Creating auth service from template..."
        if copy_service_template "auth" "express-js" "4000"; then
          services_generated=$((services_generated + 1))
        else
          services_failed=$((services_failed + 1))
        fi
      fi
    fi
  fi

  # Check for Storage service
  if [[ "${STORAGE_ENABLED:-false}" == "true" ]]; then
    if [[ "${STORAGE_PROVIDER:-}" == "custom" ]]; then
      if [[ ! -d "services/storage" ]]; then
        echo "Creating storage service from template..."
        if copy_service_template "storage" "express-js" "5000"; then
          services_generated=$((services_generated + 1))
        else
          services_failed=$((services_failed + 1))
        fi
      fi
    fi
  fi

  # Check for Functions service
  if [[ "${FUNCTIONS_ENABLED:-false}" == "true" ]]; then
    if [[ ! -d "services/functions" ]]; then
      echo "Creating functions service from template..."
      if copy_service_template "functions" "express-js" "9000"; then
        services_generated=$((services_generated + 1))
      else
        services_failed=$((services_failed + 1))
      fi
    fi
  fi

  # Check for NestJS API
  if [[ "${NESTJS_ENABLED:-false}" == "true" ]]; then
    if [[ ! -d "services/api" ]]; then
      echo "Creating NestJS API from template..."
      if copy_service_template "api" "nest-js" "3333"; then
        services_generated=$((services_generated + 1))
      else
        services_failed=$((services_failed + 1))
      fi
    fi
  fi

  # Generate custom services from CUSTOM_SERVICE_N variables
  local i=1
  while [[ $i -le 10 ]]; do
    local service_var="CUSTOM_SERVICE_${i}"
    eval "local service_def=\${$service_var:-}"

    if [[ -n "$service_def" ]]; then
      # Parse service definition (name:type:port)
      IFS=':' read -r name type port <<<"$service_def"

      if [[ -n "$name" ]] && [[ -n "$type" ]]; then
        if [[ ! -d "services/$name" ]]; then
          echo "Creating $name service from $type template..."
          if copy_service_template "$name" "$type" "${port:-8000}"; then
            services_generated=$((services_generated + 1))
          else
            services_failed=$((services_failed + 1))
          fi
        fi
      fi
    fi

    i=$((i + 1))
  done

  if [[ $services_failed -gt 0 ]]; then
    return 1
  fi

  return 0
}

# Auto-fix missing services
auto_fix_missing_services() {
  if generate_configured_services; then
    echo "All required services are ready"
  else
    echo "Some services could not be created" >&2
    return 1
  fi

  return 0
}

# List available templates
list_available_templates() {
  local templates_dir=$(get_templates_dir)

  if [[ ! -d "$templates_dir" ]]; then
    echo "Templates directory not found" >&2
    return 1
  fi

  echo "Available service templates:"
  echo ""

  # List all template directories
  for lang_dir in "$templates_dir"/*; do
    if [[ -d "$lang_dir" ]]; then
      local lang=$(basename "$lang_dir")
      echo "${lang}:"
      for template in "$lang_dir"/*; do
        if [[ -d "$template" ]]; then
          echo "  - $(basename "$template")"
        fi
      done
      echo ""
    fi
  done
}

# Export functions
export -f get_nself_root
export -f get_templates_dir
export -f copy_service_template
export -f process_template_variables
export -f safe_sed_inline
export -f generate_configured_services
export -f auto_fix_missing_services
export -f list_available_templates
