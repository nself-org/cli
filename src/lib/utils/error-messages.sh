#!/usr/bin/env bash

# error-messages.sh - Comprehensive error message library for nself
# Provides helpful, actionable error messages with clear solutions
# Cross-platform compatible (Bash 3.2+)

# Prevent double-sourcing
[[ "${ERROR_MESSAGES_SOURCED:-}" == "1" ]] && return 0

set -euo pipefail

export ERROR_MESSAGES_SOURCED=1

# Source dependencies (namespaced to avoid clobbering caller's SCRIPT_DIR)
_ERROR_MSGS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${_ERROR_MSGS_DIR}/cli-output.sh" 2>/dev/null || true
source "${_ERROR_MSGS_DIR}/platform-compat.sh" 2>/dev/null || true

# =============================================================================
# ERROR MESSAGE TEMPLATES - Top 10 Common Scenarios
# =============================================================================

# 1. Port Conflict Error
show_port_conflict_error() {
  local port="$1"
  local service="${2:-unknown}"
  local conflicting_process="${3:-}"

  cli_error "Container '${service}' failed to start"
  printf "\n"

  cli_info "Reason: Port ${port} is already in use"
  if [[ -n "$conflicting_process" ]]; then
    cli_indent "Currently used by: ${conflicting_process}" 1
  fi
  printf "\n"

  cli_section "Possible solutions:"

  # Solution 1: Kill the conflicting process
  cli_list_numbered 1 "Stop the service using port ${port}:"

  # Platform-specific commands
  if is_macos; then
    cli_indent "lsof -ti:${port} | xargs kill -9" 2
  else
    cli_indent "sudo kill \$(lsof -ti:${port})" 2
  fi
  printf "\n"

  # Solution 2: Change port in .env
  cli_list_numbered 2 "Change ${service} port in .env.local:"
  local new_port=$((port + 1000))
  local env_var=$(echo "${service}" | tr '[:lower:]' '[:upper:]')_PORT
  cli_indent "${env_var}=${new_port}" 2
  cli_indent "Then run: nself build && nself start" 2
  printf "\n"

  # Solution 3: Use nself doctor --fix
  cli_list_numbered 3 "Use automatic fix:"
  cli_indent "nself doctor --fix" 2
  printf "\n"

  cli_info "Run 'nself doctor' for more diagnostics"
  printf "\n"
}

# 2. Container Startup Failure
show_container_failed_error() {
  local container="$1"
  local reason="${2:-unknown error}"
  local logs="${3:-}"

  cli_error "Container '${container}' failed to start"
  printf "\n"

  cli_info "Reason: ${reason}"
  printf "\n"

  if [[ -n "$logs" ]]; then
    cli_section "Recent logs:"
    printf "%b%s%b\n" "${CLI_DIM}" "${logs}" "${CLI_RESET}"
    printf "\n"
  fi

  cli_section "Possible solutions:"

  cli_list_numbered 1 "Check container logs for detailed error:"
  cli_indent "nself logs ${container}" 2
  printf "\n"

  cli_list_numbered 2 "Restart the specific service:"
  cli_indent "nself restart ${container}" 2
  printf "\n"

  cli_list_numbered 3 "Check if dependencies are running:"
  cli_indent "nself status" 2
  printf "\n"

  cli_list_numbered 4 "Rebuild and restart all services:"
  cli_indent "nself stop && nself build && nself start" 2
  printf "\n"

  cli_info "Run 'nself doctor' for health diagnostics"
  printf "\n"
}

# 3. Missing Configuration Error
show_config_missing_error() {
  local config_file="${1:-.env}"
  local missing_vars="${2:-}"

  cli_error "Required configuration missing"
  printf "\n"

  cli_info "File: ${config_file} not found or incomplete"
  printf "\n"

  if [[ -n "$missing_vars" ]]; then
    cli_section "Missing variables:"
    local IFS=' '
    for var in $missing_vars; do
      cli_list_item "${var}"
    done
    printf "\n"
  fi

  cli_section "Possible solutions:"

  cli_list_numbered 1 "Initialize configuration:"
  cli_indent "nself init" 2
  printf "\n"

  cli_list_numbered 2 "Copy from example:"
  cli_indent "cp .env.example ${config_file}" 2
  cli_indent "# Edit ${config_file} with your values" 2
  printf "\n"

  cli_list_numbered 3 "Validate and fix configuration:"
  cli_indent "nself config validate --fix" 2
  printf "\n"

  cli_info "Documentation: .wiki/configuration/ENVIRONMENT-VARIABLES.md"
  printf "\n"
}

# 4. Permission Denied Error
show_permission_error() {
  local path="$1"
  local operation="${2:-access}"

  cli_error "Permission denied"
  printf "\n"

  cli_info "Cannot ${operation}: ${path}"
  printf "\n"

  cli_section "Possible solutions:"

  cli_list_numbered 1 "Fix file ownership:"
  if is_macos || is_linux; then
    cli_indent "sudo chown -R \$(whoami) ${path}" 2
  fi
  printf "\n"

  cli_list_numbered 2 "Fix file permissions:"
  if is_macos || is_linux; then
    cli_indent "chmod -R 755 ${path}" 2
  fi
  printf "\n"

  if [[ "$path" =~ docker ]] || [[ "$path" =~ /var/run ]]; then
    cli_list_numbered 3 "Add user to docker group:"
    if is_linux; then
      cli_indent "sudo usermod -aG docker \$USER" 2
      cli_indent "# Then log out and back in" 2
    elif is_macos; then
      cli_indent "# Docker Desktop handles permissions automatically" 2
      cli_indent "# Restart Docker Desktop if issues persist" 2
    fi
    printf "\n"
  fi

  cli_list_numbered 4 "Check Docker is running:"
  cli_indent "docker info" 2
  printf "\n"

  cli_info "Note: Some operations may require sudo privileges"
  printf "\n"
}

# 5. Network/Connectivity Error
show_network_error() {
  local service="$1"
  local url="${2:-}"
  local error_msg="${3:-connection failed}"

  cli_error "Network connection failed"
  printf "\n"

  cli_info "Service: ${service}"
  [[ -n "$url" ]] && cli_info "URL: ${url}"
  cli_info "Error: ${error_msg}"
  printf "\n"

  cli_section "Possible solutions:"

  cli_list_numbered 1 "Check internet connectivity:"
  cli_indent "ping -c 3 google.com" 2
  printf "\n"

  cli_list_numbered 2 "Check DNS resolution:"
  cli_indent "nslookup ${service}" 2
  printf "\n"

  cli_list_numbered 3 "Check Docker networking:"
  cli_indent "docker network ls" 2
  cli_indent "docker network inspect nself_default" 2
  printf "\n"

  cli_list_numbered 4 "Restart Docker networking:"
  cli_indent "nself stop" 2
  cli_indent "docker network prune -f" 2
  cli_indent "nself start" 2
  printf "\n"

  if [[ -n "$url" ]]; then
    cli_list_numbered 5 "Test service directly:"
    cli_indent "curl -v ${url}" 2
    printf "\n"
  fi

  cli_warning "VPN or proxy settings may interfere with connectivity"
  printf "\n"
}

# 6. Docker Not Running Error
show_docker_not_running_error() {
  local platform="${1:-$(uname -s)}"

  cli_error "Docker is not running"
  printf "\n"

  cli_section "Start Docker:"

  case "${platform}" in
    Darwin)
      cli_list_item "Open Docker Desktop application"
      cli_list_item "Or run: open -a Docker"
      printf "\n"
      cli_info "Wait 10-15 seconds for Docker to initialize"
      printf "\n"
      cli_warning "If Docker Desktop is not installed:"
      cli_indent "brew install --cask docker" 2
      cli_indent "Or download from: https://docker.com/products/docker-desktop" 2
      ;;
    Linux)
      cli_list_item "Start Docker service:"
      cli_indent "sudo systemctl start docker" 2
      printf "\n"
      cli_list_item "Enable on boot:"
      cli_indent "sudo systemctl enable docker" 2
      printf "\n"
      cli_warning "If Docker is not installed:"
      cli_indent "curl -fsSL https://get.docker.com | sh" 2
      cli_indent "sudo usermod -aG docker \$USER" 2
      ;;
    *)
      cli_list_item "Start Docker Desktop from your applications"
      ;;
  esac
  printf "\n"

  cli_section "Verify Docker is running:"
  cli_indent "docker info" 2
  cli_indent "docker ps" 2
  printf "\n"
}

# 7. Insufficient Resources Error
show_resource_error() {
  local resource="${1:-memory}"
  local available="${2:-unknown}"
  local required="${3:-unknown}"

  cli_error "Insufficient ${resource}"
  printf "\n"

  cli_info "Available: ${available}"
  cli_info "Required: ${required}"
  printf "\n"

  cli_section "Possible solutions:"

  case "$resource" in
    memory)
      cli_list_numbered 1 "Close unnecessary applications"
      printf "\n"

      cli_list_numbered 2 "Increase Docker memory allocation:"
      if is_macos; then
        cli_indent "Docker Desktop → Settings → Resources → Memory" 2
        cli_indent "Recommended: 4GB minimum, 8GB optimal" 2
      elif is_linux; then
        cli_indent "Add swap space or increase physical RAM" 2
      fi
      printf "\n"

      cli_list_numbered 3 "Disable optional services:"
      cli_indent "# In .env.local:" 2
      cli_indent "MONITORING_ENABLED=false" 2
      cli_indent "MLFLOW_ENABLED=false" 2
      ;;
    disk)
      cli_list_numbered 1 "Clean Docker resources:"
      cli_indent "docker system prune -a --volumes" 2
      printf "\n"

      cli_list_numbered 2 "Remove old builds:"
      cli_indent "nself clean" 2
      printf "\n"

      cli_list_numbered 3 "Check disk usage:"
      cli_indent "df -h ." 2
      cli_indent "du -sh * | sort -h" 2
      ;;
  esac
  printf "\n"

  cli_info "Run 'nself doctor' to check system resources"
  printf "\n"
}

# 8. Database Connection Error
show_database_error() {
  local db_type="${1:-PostgreSQL}"
  local error="${2:-connection refused}"

  cli_error "Database connection failed"
  printf "\n"

  cli_info "Database: ${db_type}"
  cli_info "Error: ${error}"
  printf "\n"

  cli_section "Possible solutions:"

  cli_list_numbered 1 "Check if database container is running:"
  cli_indent "docker ps | grep postgres" 2
  printf "\n"

  cli_list_numbered 2 "Check database logs:"
  cli_indent "nself logs postgres" 2
  printf "\n"

  cli_list_numbered 3 "Verify connection settings:"
  cli_indent "# Check in .env:" 2
  cli_indent "POSTGRES_PORT=5432" 2
  cli_indent "POSTGRES_PASSWORD=<your-password>" 2
  printf "\n"

  cli_list_numbered 4 "Restart database:"
  cli_indent "nself restart postgres" 2
  printf "\n"

  cli_list_numbered 5 "Check for port conflicts:"
  cli_indent "lsof -i :5432" 2
  printf "\n"

  cli_info "Database URL format: postgresql://user:password@localhost:5432/database"
  printf "\n"
}

# 9. Build Failure Error
show_build_error() {
  local service="${1:-unknown}"
  local stage="${2:-}"
  local error="${3:-}"

  cli_error "Build failed for '${service}'"
  printf "\n"

  [[ -n "$stage" ]] && cli_info "Stage: ${stage}"
  [[ -n "$error" ]] && cli_info "Error: ${error}"
  printf "\n"

  cli_section "Possible solutions:"

  cli_list_numbered 1 "Check Dockerfile syntax:"
  cli_indent "cat services/${service}/Dockerfile" 2
  printf "\n"

  cli_list_numbered 2 "Clean build cache and rebuild:"
  cli_indent "docker builder prune -f" 2
  cli_indent "nself build --no-cache" 2
  printf "\n"

  cli_list_numbered 3 "Check for sufficient disk space:"
  cli_indent "df -h" 2
  printf "\n"

  cli_list_numbered 4 "Rebuild specific service:"
  cli_indent "docker-compose build ${service}" 2
  printf "\n"

  cli_list_numbered 5 "Check Docker daemon logs:"
  if is_macos; then
    cli_indent "# Docker Desktop → Troubleshoot → View logs" 2
  elif is_linux; then
    cli_indent "journalctl -u docker.service" 2
  fi
  printf "\n"

  cli_info "Build logs: docker-compose logs --tail=50 ${service}"
  printf "\n"
}

# 10. Service Health Check Failure
show_health_check_error() {
  local service="$1"
  local health_status="${2:-unhealthy}"

  cli_error "Service '${service}' is ${health_status}"
  printf "\n"

  cli_section "Possible solutions:"

  cli_list_numbered 1 "Check service logs for errors:"
  cli_indent "nself logs ${service} --tail 50" 2
  printf "\n"

  cli_list_numbered 2 "Inspect container details:"
  cli_indent "docker inspect \${PROJECT_NAME}_${service}" 2
  printf "\n"

  cli_list_numbered 3 "Check service dependencies:"
  cli_indent "nself status" 2
  printf "\n"

  cli_list_numbered 4 "Restart the service:"
  cli_indent "nself restart ${service}" 2
  printf "\n"

  cli_list_numbered 5 "Check resource usage:"
  cli_indent "docker stats \${PROJECT_NAME}_${service}" 2
  printf "\n"

  cli_warning "Some services take 30-60 seconds to become healthy"
  printf "\n"
}

# =============================================================================
# HELPER FUNCTIONS
# =============================================================================

# Show generic error with custom message and solutions
show_generic_error() {
  local title="$1"
  local reason="$2"
  shift 2
  local solutions=("$@")

  cli_error "${title}"
  printf "\n"

  if [[ -n "$reason" ]]; then
    cli_info "Reason: ${reason}"
    printf "\n"
  fi

  if [[ ${#solutions[@]} -gt 0 ]]; then
    cli_section "Possible solutions:"
    local i=1
    for solution in "${solutions[@]}"; do
      cli_list_numbered "$i" "${solution}"
      i=$((i + 1))
    done
    printf "\n"
  fi

  cli_info "Run 'nself doctor' for comprehensive diagnostics"
  printf "\n"
}

# Show warning message with solutions
show_warning_message() {
  local message="$1"
  shift
  local suggestions=("$@")

  cli_warning "${message}"
  printf "\n"

  if [[ ${#suggestions[@]} -gt 0 ]]; then
    cli_section "Suggestions:"
    for suggestion in "${suggestions[@]}"; do
      cli_list_item "${suggestion}"
    done
    printf "\n"
  fi
}

# Quick error reference
show_error_reference() {
  cli_section "Common Error Solutions Quick Reference"
  printf "\n"

  cli_info "Port conflicts:"
  cli_indent "nself doctor --fix" 2
  printf "\n"

  cli_info "Container failures:"
  cli_indent "nself logs <service>" 2
  cli_indent "nself restart <service>" 2
  printf "\n"

  cli_info "Configuration issues:"
  cli_indent "nself config validate --fix" 2
  printf "\n"

  cli_info "Build problems:"
  cli_indent "docker builder prune -f" 2
  cli_indent "nself build --no-cache" 2
  printf "\n"

  cli_info "Complete health check:"
  cli_indent "nself doctor" 2
  printf "\n"

  cli_info "Full documentation: .wiki/troubleshooting/"
  printf "\n"
}

# =============================================================================
# EXPORT FUNCTIONS
# =============================================================================

export -f show_port_conflict_error
export -f show_container_failed_error
export -f show_config_missing_error
export -f show_permission_error
export -f show_network_error
export -f show_docker_not_running_error
export -f show_resource_error
export -f show_database_error
export -f show_build_error
export -f show_health_check_error
export -f show_generic_error
export -f show_warning_message
export -f show_error_reference
