#!/usr/bin/env bash


SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

source "${SCRIPT_DIR}/../utils/display.sh"
source "${SCRIPT_DIR}/../utils/output-formatter.sh"

apply_auto_fix() {
  local fix_command="$1"
  local env_file="${2:-.env.local}"

  IFS=':' read -ra FIX_PARTS <<<"$fix_command"
  local fix_type="${FIX_PARTS[0]}"

  case "$fix_type" in
    set_default_project_name)
      local default_name="nself-project-$(date +%s)"
      format_info "Setting default PROJECT_NAME: $default_name"
      if grep -q "^PROJECT_NAME=" "$env_file"; then
        sed -i.bak "s/^PROJECT_NAME=.*/PROJECT_NAME=$default_name/" "$env_file"
      else
        echo "PROJECT_NAME=$default_name" >>"$env_file"
      fi
      ;;

    fix_project_name_spaces)
      local old_name="${FIX_PARTS[1]}"
      local new_name=$(echo "$old_name" | tr ' ' '-')
      format_info "Replacing spaces with hyphens: $old_name → $new_name"
      sed -i.bak "s/^PROJECT_NAME=.*/PROJECT_NAME=$new_name/" "$env_file"
      ;;

    fix_project_name_chars)
      local old_name="${FIX_PARTS[1]}"
      local new_name=$(echo "$old_name" | sed 's/[^a-zA-Z0-9-_]/-/g')
      format_info "Removing invalid characters: $old_name → $new_name"
      sed -i.bak "s/^PROJECT_NAME=.*/PROJECT_NAME=$new_name/" "$env_file"
      ;;

    truncate_project_name)
      local old_name="${FIX_PARTS[1]}"
      local new_name="${old_name:0:50}"
      format_info "Truncating long project name: ${#old_name} → 50 chars"
      sed -i.bak "s/^PROJECT_NAME=.*/PROJECT_NAME=$new_name/" "$env_file"
      ;;

    fix_project_name_start)
      local old_name="${FIX_PARTS[1]}"
      local new_name="project-$old_name"
      format_info "Fixing numeric start: $old_name → $new_name"
      sed -i.bak "s/^PROJECT_NAME=.*/PROJECT_NAME=$new_name/" "$env_file"
      ;;

    generate_password)
      local var_name="${FIX_PARTS[1]}"
      local min_length="${FIX_PARTS[2]:-16}"
      local new_password=$(openssl rand -base64 $((min_length * 2)) | tr -d '/+=' | head -c "$min_length")
      format_info "Generating secure password for $var_name"
      if grep -q "^$var_name=" "$env_file"; then
        sed -i.bak "s/^$var_name=.*/$var_name=$new_password/" "$env_file"
      else
        echo "$var_name=$new_password" >>"$env_file"
      fi
      ;;

    extend_password)
      local var_name="${FIX_PARTS[1]}"
      local old_password="${FIX_PARTS[2]}"
      local min_length="${FIX_PARTS[3]}"
      local extra_length=$((min_length - ${#old_password}))
      local extension=$(openssl rand -base64 $((extra_length * 2)) | tr -d '/+=' | head -c "$extra_length")
      local new_password="${old_password}${extension}"
      format_info "Extending password for $var_name to $min_length characters"
      sed -i.bak "s/^$var_name=.*/$var_name=$new_password/" "$env_file"
      ;;

    escape_password_quotes)
      local var_name="${FIX_PARTS[1]}"
      local old_password="${FIX_PARTS[2]}"
      local new_password=$(echo "$old_password" | sed "s/'/\\\\'/g" | sed 's/"/\\\\"/g')
      format_info "Escaping quotes in $var_name"
      sed -i.bak "s/^$var_name=.*/$var_name=$new_password/" "$env_file"
      ;;

    replace_weak_password)
      local var_name="${FIX_PARTS[1]}"
      local new_password=$(openssl rand -base64 24 | tr -d '/+=' | head -c 20)
      format_info "Replacing weak password for $var_name"
      sed -i.bak "s/^$var_name=.*/$var_name=$new_password/" "$env_file"
      ;;

    generate_jwt_key)
      local new_key=$(openssl rand -base64 48 | tr -d '/+\n=' | head -c 32)
      format_info "Generating JWT key (32 chars)"
      if grep -q "^HASURA_JWT_KEY=" "$env_file"; then
        sed -i.bak "s/^HASURA_JWT_KEY=.*/HASURA_JWT_KEY=$new_key/" "$env_file"
      else
        echo "HASURA_JWT_KEY=$new_key" >>"$env_file"
      fi
      ;;

    extend_jwt_key)
      local old_key="${FIX_PARTS[1]}"
      local extra_length=$((32 - ${#old_key}))
      local extension=$(openssl rand -base64 $((extra_length * 2)) | tr -d '/+=' | head -c "$extra_length")
      local new_key="${old_key}${extension}"
      format_info "Extending JWT key to 32 characters"
      sed -i.bak "s/^HASURA_JWT_KEY=.*/HASURA_JWT_KEY=$new_key/" "$env_file"
      ;;

    fix_boolean)
      local var_name="${FIX_PARTS[1]}"
      local old_value="${FIX_PARTS[2]}"
      local new_value="false"
      [[ "$old_value" =~ ^[Tt1Yy] ]] && new_value="true"
      format_info "Fixing boolean $var_name: $old_value → $new_value"
      sed -i.bak "s/^$var_name=.*/$var_name=$new_value/" "$env_file"
      ;;

    normalize_boolean)
      local var_name="${FIX_PARTS[1]}"
      local old_value="${FIX_PARTS[2]}"
      local new_value=$(echo "$old_value" | tr '[:upper:]' '[:lower:]')
      format_info "Normalizing boolean $var_name: $old_value → $new_value"
      sed -i.bak "s/^$var_name=.*/$var_name=$new_value/" "$env_file"
      ;;

    fix_service_commas)
      local var_name="${FIX_PARTS[1]}"
      local old_value="${FIX_PARTS[2]}"
      local new_value=$(echo "$old_value" | sed 's/^,*//' | sed 's/,*$//' | sed 's/,,*/,/g')
      format_info "Fixing commas in $var_name"
      sed -i.bak "s/^$var_name=.*/$var_name=$new_value/" "$env_file"
      ;;

    fix_service_empty)
      local var_name="${FIX_PARTS[1]}"
      local old_value="${FIX_PARTS[2]}"
      local new_value=$(echo "$old_value" | sed 's/,,*/,/g' | sed 's/^,*//' | sed 's/,*$//')
      format_info "Removing empty values from $var_name"
      sed -i.bak "s/^$var_name=.*/$var_name=$new_value/" "$env_file"
      ;;

    remove_service_spaces)
      local var_name="${FIX_PARTS[1]}"
      local old_value="${FIX_PARTS[2]}"
      local new_value=$(echo "$old_value" | tr -d ' ')
      format_info "Removing spaces from $var_name"
      sed -i.bak "s/^$var_name=.*/$var_name=$new_value/" "$env_file"
      ;;

    fix_service_hyphen)
      local var_name="${FIX_PARTS[1]}"
      local service="${FIX_PARTS[2]}"
      local current_value=$(grep "^$var_name=" "$env_file" | cut -d= -f2-)
      local new_service=$(echo "$service" | tr '-' '_')
      local new_value=$(echo "$current_value" | sed "s/$service/$new_service/g")
      format_info "Replacing hyphens with underscores: $service → $new_service"
      sed -i.bak "s/^$var_name=.*/$var_name=$new_value/" "$env_file"
      ;;

    fix_numeric_service)
      local var_name="${FIX_PARTS[1]}"
      local service="${FIX_PARTS[2]}"
      local service_type="${FIX_PARTS[3]}"
      local current_value=$(grep "^$var_name=" "$env_file" | cut -d= -f2-)
      local new_service="${service_type}_${service}"
      local new_value=$(echo "$current_value" | sed "s/$service/$new_service/g")
      format_info "Fixing numeric service name: $service → $new_service"
      sed -i.bak "s/^$var_name=.*/$var_name=$new_value/" "$env_file"
      ;;

    fix_service_chars)
      local var_name="${FIX_PARTS[1]}"
      local service="${FIX_PARTS[2]}"
      local current_value=$(grep "^$var_name=" "$env_file" | cut -d= -f2-)
      local new_service=$(echo "$service" | sed 's/[^a-zA-Z0-9_]/_/g')
      local new_value=$(echo "$current_value" | sed "s/$service/$new_service/g")
      format_info "Fixing invalid characters: $service → $new_service"
      sed -i.bak "s/^$var_name=.*/$var_name=$new_value/" "$env_file"
      ;;

    remove_duplicate_service)
      local var_name="${FIX_PARTS[1]}"
      local service="${FIX_PARTS[2]}"
      local current_value=$(grep "^$var_name=" "$env_file" | cut -d= -f2-)
      local new_value=$(echo "$current_value" | tr ',' '\n' | awk '!seen[$0]++' | tr '\n' ',' | sed 's/,$//')
      format_info "Removing duplicate: $service"
      sed -i.bak "s/^$var_name=.*/$var_name=$new_value/" "$env_file"
      ;;

    fix_port_conflict)
      local port="${FIX_PARTS[1]}"
      format_warning "Port conflict on $port - manual intervention required" \
        "Change one of the conflicting service ports"
      ;;

    suggest_port_change)
      local port="${FIX_PARTS[1]}"
      local new_port=$((port + 1000))
      format_warning "Port $port is in use" \
        "Consider using port $new_port instead"
      ;;

    start_docker)
      format_info "Attempting to start Docker..."
      if [[ "$OSTYPE" == "darwin"* ]]; then
        open -a Docker
        sleep 5
      else
        sudo systemctl start docker 2>/dev/null || sudo service docker start 2>/dev/null
      fi
      ;;

    suggest_docker_memory)
      format_warning "Low Docker memory" \
        "Increase Docker memory allocation to at least 4GB in Docker Desktop settings"
      ;;

    suggest_disk_cleanup)
      format_info "Running Docker cleanup..."
      docker system prune -f --volumes 2>/dev/null
      ;;

    install_dependencies)
      shift
      local deps="${FIX_PARTS[@]:1}"
      format_warning "Missing dependencies: $deps" \
        "Install these manually or they will run in Docker containers only"
      ;;

    validate_extension)
      local ext="${FIX_PARTS[1]}"
      format_warning "Unknown Postgres extension: $ext" \
        "This extension may not be available in the Postgres image"
      ;;

    *)
      format_warning "Unknown fix type: $fix_type"
      ;;
  esac
}

apply_all_fixes() {
  local env_file="${1:-.env.local}"
  shift
  local fixes=("$@")

  if [[ ${#fixes[@]} -eq 0 ]]; then
    format_success "No fixes needed!"
    return 0
  fi

  format_section "Applying Auto-Fixes" 60
  printf "${BLUE}ℹ️${RESET} Found ${BOLD}%d${RESET} issues that can be automatically fixed\n" "${#fixes[@]}"
  echo

  local fix_count=0
  local total=${#fixes[@]}
  local applied_fixes=()

  for fix in "${fixes[@]}"; do
    ((fix_count++))

    # Extract fix type for display
    IFS=':' read -ra FIX_PARTS <<<"$fix"
    local fix_type="${FIX_PARTS[0]}"
    local fix_desc=""

    case "$fix_type" in
      set_default_project_name)
        fix_desc="Generate default project name"
        ;;
      fix_project_name_*)
        fix_desc="Fix project name issues"
        ;;
      generate_password)
        fix_desc="Generate secure password"
        ;;
      extend_password | extend_jwt_key)
        fix_desc="Extend short credential"
        ;;
      fix_boolean | normalize_boolean)
        fix_desc="Fix boolean value"
        ;;
      fix_service_* | remove_service_*)
        fix_desc="Fix service configuration"
        ;;
      *)
        fix_desc="Apply configuration fix"
        ;;
    esac

    printf "${BLUE}[%d/%d]${RESET} %s\n" "$fix_count" "$total" "$fix_desc"
    apply_auto_fix "$fix" "$env_file"
    applied_fixes+=("$fix_desc")

    # Show progress
    show_progress $fix_count $total "Progress"
  done

  echo
  format_success "Successfully applied ${fix_count} fixes!"

  # Show summary of what was fixed
  if [[ ${#applied_fixes[@]} -gt 0 ]]; then
    echo
    printf "${GREEN}✅ Fixes Applied:${RESET}\n"
    for desc in "${applied_fixes[@]}"; do
      printf "   ${GREEN}•${RESET} %s\n" "$desc"
    done
  fi

  rm -f "${env_file}.bak" 2>/dev/null

  return 0
}

export -f apply_auto_fix
export -f apply_all_fixes
