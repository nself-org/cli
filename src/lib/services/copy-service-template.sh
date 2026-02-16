#!/usr/bin/env bash

# copy-service-template.sh - Copy service templates instead of generating
# POSIX-compliant, no Bash 4+ features

# Get the root directory where templates are stored
get_templates_root() {

set -euo pipefail

  # Try to find the templates directory
  local script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  local templates_dir=""

  # Check various possible locations
  for dir in "$script_dir/../../templates/services" \
    "$NSELF_ROOT/src/templates/services" \
    "$NSELF_ROOT/templates/services" \
    "/usr/local/share/nself/src/templates/services"; do
    if [[ -d "$dir" ]]; then
      templates_dir="$dir"
      break
    fi
  done

  echo "$templates_dir"
}

# Copy service template to target directory
copy_service_template() {
  local service_name="$1"
  local service_type="$2"
  local target_dir="${3:-services/$service_name}"

  # Get template directory
  local templates_root=$(get_templates_root)
  if [[ -z "$templates_root" ]]; then
    echo "Error: Templates directory not found" >&2
    return 1
  fi

  # Map service type to template path
  local template_path=""
  case "$service_type" in
    express-js) template_path="$templates_root/js/express-js" ;;
    fastify-js) template_path="$templates_root/js/fastify-js" ;;
    nestjs-js | nest-js) template_path="$templates_root/js/nest-js" ;;
    bullmq-js) template_path="$templates_root/js/bullmq-js" ;;
    node-js) template_path="$templates_root/js/node-js" ;;
    fastapi) template_path="$templates_root/py/fastapi" ;;
    flask) template_path="$templates_root/py/flask" ;;
    django) template_path="$templates_root/py/django-rest" ;;
    gin) template_path="$templates_root/go/gin" ;;
    echo) template_path="$templates_root/go/echo" ;;
    fiber) template_path="$templates_root/go/fiber" ;;
    actix) template_path="$templates_root/rust/actix-web" ;;
    rails) template_path="$templates_root/ruby/rails" ;;
    spring) template_path="$templates_root/java/spring-boot" ;;
    custom) template_path="$templates_root/custom" ;;
    *)
      echo "Error: Unknown service type: $service_type" >&2
      return 1
      ;;
  esac

  # Check if template exists
  if [[ ! -d "$template_path" ]]; then
    echo "Error: Template not found: $template_path" >&2
    return 1
  fi

  # Create target directory
  mkdir -p "$target_dir"

  # Copy template files
  cp -r "$template_path"/* "$target_dir/" 2>/dev/null || {
    echo "Error: Failed to copy template files" >&2
    return 1
  }

  # Process template variables
  process_template_variables "$target_dir" "$service_name"

  return 0
}

# Process template variables in copied files
process_template_variables() {
  local target_dir="$1"
  local service_name="$2"
  local service_port="${3:-8000}"

  # Find all template files
  find "$target_dir" -type f \( -name "*.template" -o -name "*.tpl" \) | while read -r template_file; do
    local output_file="${template_file%.template}"
    output_file="${output_file%.tpl}"

    # Replace common placeholders
    sed -e "s/{{SERVICE_NAME}}/$service_name/g" \
      -e "s/{{PROJECT_NAME}}/${PROJECT_NAME:-nself}/g" \
      -e "s/{{BASE_DOMAIN}}/${BASE_DOMAIN:-localhost}/g" \
      -e "s/{{PORT}}/$service_port/g" \
      -e "s/{{ENV}}/${ENV:-dev}/g" \
      "$template_file" >"$output_file"

    rm "$template_file"
  done

  # Update package.json if it exists
  if [[ -f "$target_dir/package.json" ]]; then
    # Use temporary file for compatibility
    local temp_file=$(mktemp)
    sed "s/\"name\": \"[^\"]*\"/\"name\": \"$service_name\"/" "$target_dir/package.json" >"$temp_file"
    mv "$temp_file" "$target_dir/package.json"
  fi

  # Update go.mod if it exists
  if [[ -f "$target_dir/go.mod" ]]; then
    local temp_file=$(mktemp)
    sed "s|module .*|module $service_name|" "$target_dir/go.mod" >"$temp_file"
    mv "$temp_file" "$target_dir/go.mod"
  fi

  # Update Cargo.toml if it exists
  if [[ -f "$target_dir/Cargo.toml" ]]; then
    local temp_file=$(mktemp)
    sed "s/name = \"[^\"]*\"/name = \"$service_name\"/" "$target_dir/Cargo.toml" >"$temp_file"
    mv "$temp_file" "$target_dir/Cargo.toml"
  fi

  # Create .env file with service configuration
  cat >"$target_dir/.env" <<EOF
SERVICE_NAME=$service_name
PORT=$service_port
NODE_ENV=production
ENV=${ENV:-dev}
BASE_DOMAIN=${BASE_DOMAIN:-localhost}
EOF
}

# List available service templates
list_service_templates() {
  local templates_root=$(get_templates_root)

  if [[ -z "$templates_root" ]]; then
    echo "Error: Templates directory not found" >&2
    return 1
  fi

  echo "Available service templates:"
  echo ""

  # JavaScript/TypeScript
  if [[ -d "$templates_root/js" ]]; then
    echo "JavaScript/TypeScript:"
    for template in "$templates_root/js"/*; do
      if [[ -d "$template" ]]; then
        echo "  - $(basename "$template")"
      fi
    done
  fi

  # Python
  if [[ -d "$templates_root/py" ]]; then
    echo ""
    echo "Python:"
    for template in "$templates_root/py"/*; do
      if [[ -d "$template" ]]; then
        echo "  - $(basename "$template")"
      fi
    done
  fi

  # Go
  if [[ -d "$templates_root/go" ]]; then
    echo ""
    echo "Go:"
    for template in "$templates_root/go"/*; do
      if [[ -d "$template" ]]; then
        echo "  - $(basename "$template")"
      fi
    done
  fi

  # Other languages
  for lang_dir in "$templates_root"/*; do
    if [[ -d "$lang_dir" ]]; then
      local lang=$(basename "$lang_dir")
      case "$lang" in
        js | py | go) continue ;;
        *)
          echo ""
          echo "${lang^}:"
          for template in "$lang_dir"/*; do
            if [[ -d "$template" ]]; then
              echo "  - $(basename "$template")"
            fi
          done
          ;;
      esac
    fi
  done
}

# Build custom services from CUSTOM_SERVICE_N variables
build_custom_services_from_templates() {
  local services_built=0
  local services_failed=0

  # Check for custom services (up to 10)
  local i=1
  while [[ $i -le 10 ]]; do
    eval "local service_def=\${CUSTOM_SERVICE_${i}:-}"

    if [[ -n "$service_def" ]]; then
      # Parse service definition (name:type:port)
      local IFS=':'
      local parts=($service_def)
      local service_name="${parts[0]}"
      local service_type="${parts[1]}"
      local service_port="${parts[2]:-8000}"

      echo "Creating service from template: $service_name ($service_type)"

      if copy_service_template "$service_name" "$service_type" "services/$service_name"; then
        # Update port in the copied service
        if [[ -f "services/$service_name/.env" ]]; then
          echo "PORT=$service_port" >>"services/$service_name/.env"
        fi
        services_built=$((services_built + 1))
      else
        echo "Failed to create service: $service_name" >&2
        services_failed=$((services_failed + 1))
      fi
    fi

    i=$((i + 1))
  done

  if [[ $services_failed -gt 0 ]]; then
    echo "Created $services_built services, $services_failed failed" >&2
    return 1
  elif [[ $services_built -gt 0 ]]; then
    echo "Successfully created $services_built services from templates"
  fi

  return 0
}

# Export functions
export -f get_templates_root
export -f copy_service_template
export -f process_template_variables
export -f list_service_templates
export -f build_custom_services_from_templates
