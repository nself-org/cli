#!/usr/bin/env bash


AUTO_FIXER_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

source "${AUTO_FIXER_DIR}/../utils/display.sh"
source "${AUTO_FIXER_DIR}/../utils/output-formatter.sh"

# Source platform compatibility for safe_sed_inline
source "${AUTO_FIXER_DIR}/../utils/platform-compat.sh" 2>/dev/null || {
  # Fallback definition
  safe_sed_inline() {
    local file="$1"
    shift
    if [[ "$OSTYPE" == "darwin"* ]]; then
      if [[ "$OSTYPE" == "darwin"* ]]; then
        sed -i '' "$@" "$file"
      else
        sed -i "$@" "$file"
      fi
    else
      sed -i "$@" "$file"
    fi
  }
}

# Apply a single auto-fix
apply_auto_fix() {
  local fix_command="$1"
  local env_file="${2:-.env.local}"

  # Parse fix command
  IFS=':' read -r -a fix_parts <<<"$fix_command"
  local fix_type="${fix_parts[0]}"

  case "$fix_type" in
    # Empty file fixes
    create_minimal_config)
      format_info "Creating minimal configuration..."
      cat >"$env_file" <<'EOF'
# nself Minimal Configuration
PROJECT_NAME=myproject

# Uncomment to enable optional services:
# REDIS_ENABLED=true
# DASHBOARD_ENABLED=true
# NESTJS_ENABLED=true
# NESTJS_SERVICES=api
EOF
      format_success "Created minimal configuration file"
      ;;

    # Whitespace fixes
    trim_whitespace)
      local key="${fix_parts[2]}"
      format_info "Trimming whitespace from $key..."
      # Get current value and trim it
      local current_value=$(grep "^$key=" "$env_file" | cut -d'=' -f2-)
      local trimmed_value=$(echo "$current_value" | xargs)
      safe_sed_inline "$env_file" "s|^$key=.*|$key=$trimmed_value|"
      format_success "Trimmed whitespace from $key"
      ;;

    replace_tabs)
      format_info "Replacing tabs with spaces..."
      safe_sed_inline "$env_file" $'s/\t/    /g'
      format_success "Replaced tabs with spaces"
      ;;

    # Quote fixes
    fix_quote_mismatch)
      local key="${fix_parts[2]}"
      local quote_type="${fix_parts[3]}"
      format_info "Fixing $quote_type quote mismatch in $key..."
      local current_value=$(grep "^$key=" "$env_file" | cut -d'=' -f2-)

      if [[ "$quote_type" == "single" ]]; then
        # Remove unmatched single quotes
        local fixed_value=$(echo "$current_value" | sed "s/'//g")
      else
        # Remove unmatched double quotes
        local fixed_value=$(echo "$current_value" | sed 's/"//g')
      fi

      safe_sed_inline "$env_file" "s|^$key=.*|$key=$fixed_value|"
      format_success "Fixed quote mismatch in $key"
      ;;

    fix_mixed_quotes)
      local key="${fix_parts[2]}"
      format_info "Fixing mixed quotes in $key..."
      local current_value=$(grep "^$key=" "$env_file" | cut -d'=' -f2-)
      # Remove all quotes
      local fixed_value=$(echo "$current_value" | sed "s/['\"]//g")
      safe_sed_inline "$env_file" "s|^$key=.*|$key=$fixed_value|"
      format_success "Fixed mixed quotes in $key"
      ;;

    # Duplicate fixes
    remove_duplicate)
      local key="${fix_parts[2]}"
      format_info "Removing duplicate entries for $key..."
      # Keep only the first occurrence
      awk -v key="$key" '
                !seen && $0 ~ "^"key"=" { seen=1; print; next }
                $0 !~ "^"key"=" { print }
            ' "$env_file" >"$env_file.tmp"
      mv "$env_file.tmp" "$env_file"
      format_success "Removed duplicate entries for $key"
      ;;

    # Port fixes
    fix_port_number)
      local var_name="${fix_parts[1]}"
      local bad_port="${fix_parts[2]}"
      format_info "Fixing invalid port number for $var_name..."
      # Extract numeric part if possible
      local numeric_port=$(echo "$bad_port" | grep -o '[0-9]*' | head -1)
      if [[ -n "$numeric_port" ]] && [[ $numeric_port -ge 1 ]] && [[ $numeric_port -le 65535 ]]; then
        safe_sed_inline "$env_file" "s|^$var_name=.*|$var_name=$numeric_port|"
      else
        # Use a sensible default based on service
        case "$var_name" in
          POSTGRES_PORT) local new_port=5432 ;;
          REDIS_PORT) local new_port=6379 ;;
          HASURA_PORT) local new_port=8080 ;;
          AUTH_PORT) local new_port=4000 ;;
          *) local new_port=3000 ;;
        esac
        safe_sed_inline "$env_file" "s|^$var_name=.*|$var_name=$new_port|"
      fi
      format_success "Fixed port number for $var_name"
      ;;

    fix_port_range)
      local var_name="${fix_parts[1]}"
      local bad_port="${fix_parts[2]}"
      format_info "Fixing out-of-range port for $var_name..."

      if [[ $bad_port -gt 65535 ]]; then
        # Try to use last 4 digits if valid
        local new_port=$((bad_port % 10000))
        [[ $new_port -lt 1024 ]] && new_port=$((new_port + 1024))
      elif [[ $bad_port -lt 1 ]]; then
        # Use service default
        case "$var_name" in
          POSTGRES_PORT) local new_port=5432 ;;
          REDIS_PORT) local new_port=6379 ;;
          HASURA_PORT) local new_port=8080 ;;
          *) local new_port=3000 ;;
        esac
      fi

      safe_sed_inline "$env_file" "s|^$var_name=.*|$var_name=$new_port|"
      format_success "Fixed port range for $var_name to $new_port"
      ;;

    suggest_alternative_port)
      local var_name="${fix_parts[1]}"
      local current_port="${fix_parts[2]}"
      format_info "Finding alternative port for $var_name..."

      # Try ports in sequence until we find a free one
      local new_port=$((current_port + 1))
      while lsof -Pi :$new_port -sTCP:LISTEN -t >/dev/null 2>&1; do
        new_port=$((new_port + 1))
        [[ $new_port -gt 65535 ]] && new_port=1024
      done

      safe_sed_inline "$env_file" "s|^$var_name=.*|$var_name=$new_port|"
      format_success "Changed $var_name from $current_port to $new_port"
      ;;

    # Inline comment fixes
    remove_inline_comment)
      local key="${fix_parts[2]}"
      format_info "Removing inline comment from $key..."
      # Remove everything after # (but not if # is in quotes)
      safe_sed_inline "$env_file" "/^$key=/s/\([^#]*\)#.*/\1/"
      # Remove trailing whitespace
      safe_sed_inline "$env_file" "/^$key=/s/[[:space:]]*$//"
      format_success "Removed inline comment from $key"
      ;;

    # Password fixes
    escape_password_special)
      local var_name="${fix_parts[1]}"
      local password="${fix_parts[2]}"
      format_info "Escaping special characters in $var_name..."
      # Replace problematic characters with safe alternatives
      local safe_password=$(echo "$password" | tr -d '\"'\''`$!&*(){}[];><|\\ ' | tr ' ' '_')
      safe_sed_inline "$env_file" "s|^$var_name=.*|$var_name=$safe_password|"
      format_success "Escaped special characters in $var_name"
      ;;

    remove_password_spaces)
      local var_name="${fix_parts[1]}"
      local password="${fix_parts[2]}"
      format_info "Removing spaces from $var_name..."
      local fixed_password=$(echo "$password" | tr -d ' ')
      safe_sed_inline "$env_file" "s|^$var_name=.*|$var_name=$fixed_password|"
      format_success "Removed spaces from $var_name"
      ;;

    generate_password)
      local var_name="${fix_parts[1]}"
      local min_length="${fix_parts[2]:-20}"
      format_info "Generating secure password for $var_name..."
      local new_password=$(openssl rand -base64 32 | tr -d '/+=\n' | head -c "$min_length")

      if grep -q "^$var_name=" "$env_file"; then
        safe_sed_inline "$env_file" "s|^$var_name=.*|$var_name=$new_password|"
      else
        echo "$var_name=$new_password" >>"$env_file"
      fi
      format_success "Generated secure password for $var_name"
      ;;

    extend_password)
      local var_name="${fix_parts[1]}"
      local current="${fix_parts[2]}"
      local min_length="${fix_parts[3]}"
      format_info "Extending password for $var_name..."
      local extension=$(openssl rand -base64 20 | tr -d '/+=\n' | head -c $((min_length - ${#current})))
      local new_password="${current}${extension}"
      safe_sed_inline "$env_file" "s|^$var_name=.*|$var_name=$new_password|"
      format_success "Extended password for $var_name"
      ;;

    replace_weak_password)
      local var_name="${fix_parts[1]}"
      format_info "Replacing weak password for $var_name..."
      local new_password=$(openssl rand -base64 24 | tr -d '/+=\n' | head -c 20)
      safe_sed_inline "$env_file" "s|^$var_name=.*|$var_name=$new_password|"
      format_success "Replaced weak password for $var_name"
      ;;

    # IP address fixes
    fix_ip_address)
      local var_name="${fix_parts[1]}"
      local bad_ip="${fix_parts[2]}"
      format_info "Fixing invalid IP address for $var_name..."

      # Default to localhost for most cases
      local new_ip="localhost"

      # Check if it looks like an attempt at an IP
      if [[ "$bad_ip" =~ [0-9]+\.[0-9]+\.[0-9]+\.[0-9]+ ]]; then
        # Try to fix common mistakes
        IFS='.' read -r -a octets <<<"$bad_ip"
        local fixed_octets=()
        for octet in "${octets[@]}"; do
          [[ $octet -gt 255 ]] && octet=255
          fixed_octets+=("$octet")
        done
        new_ip="${fixed_octets[0]}.${fixed_octets[1]}.${fixed_octets[2]}.${fixed_octets[3]}"
      fi

      safe_sed_inline "$env_file" "s|^$var_name=.*|$var_name=$new_ip|"
      format_success "Fixed IP address for $var_name to $new_ip"
      ;;

    # Memory format fixes
    fix_memory_format)
      local var_name="${fix_parts[1]}"
      local bad_memory="${fix_parts[2]}"
      format_info "Fixing memory format for $var_name..."

      # Extract numeric part
      local numeric=$(echo "$bad_memory" | grep -o '[0-9]*' | head -1)
      if [[ -z "$numeric" ]]; then
        numeric="512"
      fi

      # Determine unit based on size
      if [[ $numeric -lt 1024 ]]; then
        local fixed_memory="${numeric}M"
      else
        local gb=$((numeric / 1024))
        local fixed_memory="${gb}G"
      fi

      safe_sed_inline "$env_file" "s|^$var_name=.*|$var_name=$fixed_memory|"
      format_success "Fixed memory format for $var_name to $fixed_memory"
      ;;

    # Docker name fixes
    fix_docker_name)
      local var_name="${fix_parts[1]}"
      local bad_name="${fix_parts[2]}"
      format_info "Fixing Docker naming for $var_name..."

      # Convert to lowercase and replace invalid chars
      local fixed_name=$(echo "$bad_name" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9._-]/-/g')
      # Ensure it starts with alphanumeric
      [[ "$fixed_name" =~ ^[^a-z0-9] ]] && fixed_name="n${fixed_name}"

      safe_sed_inline "$env_file" "s|^$var_name=.*|$var_name=$fixed_name|"
      format_success "Fixed Docker naming for $var_name to $fixed_name"
      ;;

    truncate_docker_name)
      local var_name="${fix_parts[1]}"
      local long_name="${fix_parts[2]}"
      format_info "Truncating Docker name for $var_name..."

      local truncated="${long_name:0:63}"
      safe_sed_inline "$env_file" "s|^$var_name=.*|$var_name=$truncated|"
      format_success "Truncated $var_name to 63 characters"
      ;;

    # Project name fixes (from original)
    set_default_project_name)
      format_info "Setting default project name..."
      if grep -q "^PROJECT_NAME=" "$env_file"; then
        safe_sed_inline "$env_file" "s|^PROJECT_NAME=.*|PROJECT_NAME=myproject|"
      else
        echo "PROJECT_NAME=myproject" >>"$env_file"
      fi
      format_success "Set PROJECT_NAME to 'myproject'"
      ;;

    fix_project_name_spaces)
      local name="${fix_parts[1]}"
      format_info "Fixing spaces in project name..."
      local fixed_name=$(echo "$name" | tr ' ' '-' | tr '[:upper:]' '[:lower:]')
      safe_sed_inline "$env_file" "s|^PROJECT_NAME=.*|PROJECT_NAME=$fixed_name|"
      format_success "Fixed PROJECT_NAME to '$fixed_name'"
      ;;

    fix_project_name_chars)
      local name="${fix_parts[1]}"
      format_info "Fixing invalid characters in project name..."
      local fixed_name=$(echo "$name" | sed 's/[^a-zA-Z0-9-_]/-/g' | tr '[:upper:]' '[:lower:]')
      safe_sed_inline "$env_file" "s|^PROJECT_NAME=.*|PROJECT_NAME=$fixed_name|"
      format_success "Fixed PROJECT_NAME to '$fixed_name'"
      ;;

    fix_project_name_start)
      local name="${fix_parts[1]}"
      format_info "Fixing project name starting with number..."
      local fixed_name="project-$name"
      safe_sed_inline "$env_file" "s|^PROJECT_NAME=.*|PROJECT_NAME=$fixed_name|"
      format_success "Fixed PROJECT_NAME to '$fixed_name'"
      ;;

    truncate_project_name)
      local name="${fix_parts[1]}"
      format_info "Truncating project name..."
      local truncated="${name:0:50}"
      safe_sed_inline "$env_file" "s|^PROJECT_NAME=.*|PROJECT_NAME=$truncated|"
      format_success "Truncated PROJECT_NAME to '$truncated'"
      ;;

    # Boolean fixes
    fix_boolean)
      local var_name="${fix_parts[1]}"
      local value="${fix_parts[2]}"
      format_info "Fixing boolean value for $var_name..."

      # Try to interpret intent
      local lower_value=$(echo "$value" | tr '[:upper:]' '[:lower:]')
      case "$lower_value" in
        1 | yes | y | on | enabled | enable) local fixed_value="true" ;;
        0 | no | n | off | disabled | disable) local fixed_value="false" ;;
        *) local fixed_value="false" ;; # Default to false for safety
      esac

      safe_sed_inline "$env_file" "s|^$var_name=.*|$var_name=$fixed_value|"
      format_success "Fixed $var_name to '$fixed_value'"
      ;;

    normalize_boolean)
      local var_name="${fix_parts[1]}"
      local value="${fix_parts[2]}"
      format_info "Normalizing boolean for $var_name..."
      local normalized=$(echo "$value" | tr '[:upper:]' '[:lower:]')
      safe_sed_inline "$env_file" "s|^$var_name=.*|$var_name=$normalized|"
      format_success "Normalized $var_name to '$normalized'"
      ;;

    # Service list fixes
    fix_service_commas)
      local var_name="${fix_parts[1]}"
      local services="${fix_parts[2]}"
      format_info "Fixing service list commas for $var_name..."
      # Remove leading/trailing commas
      local fixed=$(echo "$services" | sed 's/^,*//' | sed 's/,*$//')
      safe_sed_inline "$env_file" "s|^$var_name=.*|$var_name=$fixed|"
      format_success "Fixed service list commas for $var_name"
      ;;

    fix_service_empty)
      local var_name="${fix_parts[1]}"
      local services="${fix_parts[2]}"
      format_info "Removing empty service entries for $var_name..."
      # Remove consecutive commas
      local fixed=$(echo "$services" | sed 's/,,*/,/g' | sed 's/^,*//' | sed 's/,*$//')
      safe_sed_inline "$env_file" "s|^$var_name=.*|$var_name=$fixed|"
      format_success "Removed empty service entries for $var_name"
      ;;

    remove_service_spaces)
      local var_name="${fix_parts[1]}"
      local services="${fix_parts[2]}"
      format_info "Removing spaces from service list for $var_name..."
      local fixed=$(echo "$services" | tr -d ' ')
      safe_sed_inline "$env_file" "s|^$var_name=.*|$var_name=$fixed|"
      format_success "Removed spaces from $var_name"
      ;;

    fix_service_hyphen)
      local var_name="${fix_parts[1]}"
      local service="${fix_parts[2]}"
      format_info "Replacing hyphens with underscores in $var_name..."
      local current=$(grep "^$var_name=" "$env_file" | cut -d'=' -f2-)
      local fixed=$(echo "$current" | sed "s/$service/$(echo $service | tr '-' '_')/g")
      safe_sed_inline "$env_file" "s|^$var_name=.*|$var_name=$fixed|"
      format_success "Fixed service name hyphens in $var_name"
      ;;

    # JWT fixes
    generate_jwt_key)
      format_info "Generating JWT key..."
      local jwt_key=$(openssl rand -base64 48 | tr -d '/+=\n' | head -c 32)
      if grep -q "^HASURA_JWT_KEY=" "$env_file"; then
        safe_sed_inline "$env_file" "s|^HASURA_JWT_KEY=.*|HASURA_JWT_KEY=$jwt_key|"
      else
        echo "HASURA_JWT_KEY=$jwt_key" >>"$env_file"
      fi
      format_success "Generated secure JWT key"
      ;;

    extend_jwt_key)
      local current="${fix_parts[1]}"
      format_info "Extending JWT key..."
      local extension=$(openssl rand -base64 20 | tr -d '/+=\n' | head -c $((32 - ${#current})))
      local new_key="${current}${extension}"
      safe_sed_inline "$env_file" "s|^HASURA_JWT_KEY=.*|HASURA_JWT_KEY=$new_key|"
      format_success "Extended JWT key to 32+ characters"
      ;;

    *)
      format_warning "Unknown fix type: $fix_type"
      return 1
      ;;
  esac

  return 0
}

# Apply all fixes
apply_all_fixes() {
  local env_file="${1:-.env.local}"
  shift
  local fixes=("$@")

  if [[ ${#fixes[@]} -eq 0 ]]; then
    format_info "No fixes to apply"
    return 0
  fi

  format_section "Applying Auto-Fixes" 40
  echo "Found ${#fixes[@]} issues to fix"
  echo ""

  # Create backup
  cp "$env_file" "${env_file}.backup"
  format_info "Created backup: ${env_file}.backup"

  local applied=0
  local failed=0

  for fix in "${fixes[@]}"; do
    if apply_auto_fix "$fix" "$env_file"; then
      applied=$((applied + 1))
    else
      failed=$((failed + 1))
    fi
  done

  echo ""
  format_section "Auto-Fix Summary" 40
  echo "${GREEN}✓${RESET} Applied: $applied fixes"
  [[ $failed -gt 0 ]] && echo "${RED}✗${RESET} Failed: $failed fixes"

  # Clean up backup files
  rm -f "${env_file}.bak"

  return $([[ $failed -eq 0 ]] && echo 0 || echo 1)
}

# Interactive fix mode
interactive_fixes() {
  local env_file="${1:-.env.local}"
  shift
  local fixes=("$@")

  if [[ ${#fixes[@]} -eq 0 ]]; then
    format_info "No fixes available"
    return 0
  fi

  format_section "Interactive Auto-Fix" 40
  echo "Found ${#fixes[@]} potential fixes"
  echo ""

  # Create backup
  cp "$env_file" "${env_file}.backup"
  format_info "Created backup: ${env_file}.backup"

  for fix in "${fixes[@]}"; do
    # Parse fix to show description
    IFS=':' read -r -a fix_parts <<<"$fix"
    local fix_type="${fix_parts[0]}"

    printf "${YELLOW}→${RESET} Fix available: ${BOLD}%s${RESET}\n" "$fix_type"
    read -p "Apply this fix? [y/N]: " -n 1 -r
    echo

    if [[ $REPLY =~ ^[Yy]$ ]]; then
      apply_auto_fix "$fix" "$env_file"
    else
      format_info "Skipped: $fix_type"
    fi
    echo ""
  done

  # Clean up backup files
  rm -f "${env_file}.bak"

  format_success "Interactive fixes complete"
  return 0
}

# Export functions
export -f apply_auto_fix
export -f apply_all_fixes
export -f interactive_fixes
