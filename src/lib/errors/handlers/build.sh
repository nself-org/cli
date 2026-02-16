#!/usr/bin/env bash


# build.sh - Build error detection and fixes

# Analyze build errors
analyze_build_failure() {

set -euo pipefail

  local error_output="$1"

  echo ""
  log_info "Analyzing build errors..."

  # Check for Go module errors
  if echo "$error_output" | grep -q "missing go.sum entry\|go get\|go mod download"; then
    handle_go_module_error "$error_output"
    return
  fi

  # Check for Node.js errors
  if echo "$error_output" | grep -q "npm ERR!\|npm error"; then
    handle_node_error "$error_output"
    return
  fi

  # Check for Dockerfile errors
  if echo "$error_output" | grep -q "failed to solve"; then
    handle_dockerfile_error "$error_output"
    return
  fi

  # Check for disk space
  if echo "$error_output" | grep -q "no space left on device"; then
    handle_disk_space_error
    return
  fi

  # Generic build error
  log_error "Build failed with unknown error"
  echo "$error_output" | tail -20
}

# Handle Go module errors
handle_go_module_error() {
  local error_output="$1"

  log_error "Go module dependencies are missing"

  # Extract affected services
  local services=$(echo "$error_output" | grep -oE "${PROJECT_NAME:-nself}-[a-z0-9-]+" | sort -u)

  echo ""
  log_info "Affected services:"
  for service in $services; do
    echo "  • $service"
  done

  echo ""
  log_info "This happens when Go services are missing dependencies"
  log_info "Solutions:"
  echo ""
  echo "  1) Auto-fix: Run 'go mod tidy' and update go.mod/go.sum"
  echo "  2) Manual: Run 'go mod tidy' in each service directory"
  echo "  3) Skip: Remove Go services from docker-compose.yml"
  echo "  4) Cancel"

  read -p "Choose option [1-4]: " -n 1 -r
  echo ""

  case $REPLY in
    1)
      fix_go_modules "$services"
      ;;
    2)
      show_go_manual_fix "$services"
      ;;
    3)
      disable_go_services "$services"
      ;;
    4)
      log_info "Cancelled"
      return 1
      ;;
    *)
      log_error "Invalid option"
      return 1
      ;;
  esac
}

# Fix Go modules automatically
fix_go_modules() {
  local services="$1"

  log_info "Attempting to fix Go module dependencies..."

  for service in $services; do
    # Find the service directory
    local service_dir=""

    # Extract service base name (${PROJECT_NAME}-go-go1 -> go1)
    local base_service=$(echo "$service" | sed "s/^${PROJECT_NAME:-nself}-[^-]*-//")

    # Check common locations
    if [[ -d "services/go/$base_service" ]]; then
      service_dir="services/go/$base_service"
    elif [[ -d "services/$base_service" ]]; then
      service_dir="services/$base_service"
    elif [[ -d "$service" ]]; then
      service_dir="$service"
    else
      # Try to extract from docker-compose.yml
      service_dir=$(grep -A5 "$service:" docker-compose.yml | grep "build:" | sed 's/.*build: //' | tr -d '"' | tr -d "'")
    fi

    if [[ -n "$service_dir" ]] && [[ -d "$service_dir" ]]; then
      log_info "Fixing $service in $service_dir..."

      # Check if go is installed
      if ! command -v go &>/dev/null; then
        log_warning "Go is not installed. Cannot auto-fix Go modules."
        log_info "Install Go from https://golang.org/dl/ and try again"
        return 1
      fi

      # If no go.mod exists, initialize with proper module name
      if [[ ! -f "$service_dir/go.mod" ]]; then
        log_info "Initializing go.mod for $base_service..."
        (cd "$service_dir" && go mod init "$base_service" 2>/dev/null || true)
      fi

      # Analyze main.go for imports and add them
      if [[ -f "$service_dir/main.go" ]]; then
        local imports=$(grep -E '^\s*"github\.com/[^"]+"|^\s*"golang\.org/x/[^"]+"' "$service_dir/main.go" | sed 's/.*"\(.*\)".*/\1/' | sort -u)
        if [[ -n "$imports" ]]; then
          log_info "Detected imports, adding dependencies..."
          (
            cd "$service_dir"
            for import in $imports; do
              go get "$import" 2>/dev/null || true
            done
          )
        fi
      fi

      # Tidy and download dependencies
      (cd "$service_dir" && go mod tidy && go mod download)
      log_success "Fixed $service dependencies with 'go mod tidy'"
    else
      log_warning "Could not find directory for $service"
    fi
  done

  log_info "Go modules fixed. Try building again."
}

# Show manual fix instructions
show_go_manual_fix() {
  local services="$1"

  echo ""
  log_info "Manual fix instructions:"
  echo ""
  echo "Run these commands for each Go service:"
  echo ""

  for service in $services; do
    echo "  # For $service:"
    echo "  cd services/$service  # or wherever the service is"
    echo "  go mod init $service"
    echo "  go get github.com/gorilla/mux"
    echo "  go mod tidy"
    echo ""
  done

  echo "Then run 'nself build' again."
}

# Disable Go services
disable_go_services() {
  local services="$1"

  log_info "Disabling Go services in docker-compose.yml..."

  # Backup docker-compose.yml using _backup/timestamp convention
  timestamp=$(date +%Y%m%d_%H%M%S)
  backup_dir="_backup/${timestamp}"
  mkdir -p "$backup_dir"
  cp docker-compose.yml "$backup_dir/docker-compose.yml"

  # Comment out the services
  for service in $services; do
    log_info "Disabling $service..."
    # This is a simplified approach - might need more sophisticated YAML editing
    sed -i.bak "/$service:/,/^[^ ]/{s/^/#/}" docker-compose.yml
  done

  log_success "Go services disabled"
  log_info "Backup saved to $backup_dir/docker-compose.yml"
  log_info "You can re-enable them later by uncommenting in docker-compose.yml"
}

# Handle Node.js errors
handle_node_error() {
  local error_output="$1"

  log_error "Node.js build error detected"

  if echo "$error_output" | grep -q "EACCES\|permission denied"; then
    log_error "Permission issue with npm"
    log_info "Try running: sudo chown -R $(whoami) ~/.npm"
  elif echo "$error_output" | grep -q "Cannot find module"; then
    log_error "Missing Node.js dependencies"
    log_info "Solutions:"
    echo "  1) Delete node_modules and package-lock.json, then rebuild"
    echo "  2) Clear npm cache: npm cache clean --force"
  else
    log_error "Generic Node.js error"
    echo "$error_output" | grep "npm ERR!" | head -5
  fi

  read -p "Try to fix automatically? [y/N]: " -n 1 -r
  echo ""

  if [[ $REPLY =~ ^[Yy]$ ]]; then
    fix_node_issues
  fi
}

# Fix Node.js issues
fix_node_issues() {
  log_info "Attempting to fix Node.js issues..."

  # Find package.json files
  local package_files=$(find . -name "package.json" -not -path "*/node_modules/*" | head -5)

  for package_file in $package_files; do
    local dir=$(dirname "$package_file")
    log_info "Cleaning $dir..."

    # Remove node_modules and lock file
    rm -rf "$dir/node_modules"
    rm -f "$dir/package-lock.json"
  done

  # Clear npm cache
  npm cache clean --force 2>/dev/null || true

  log_success "Node.js cleanup complete"
  log_info "Try building again"
}

# Handle Dockerfile errors
handle_dockerfile_error() {
  local error_output="$1"

  log_error "Dockerfile build error"

  # Extract the failing step
  local failing_step=$(echo "$error_output" | grep "ERROR \[" | head -1)

  if [[ -n "$failing_step" ]]; then
    log_info "Failed at: $failing_step"
  fi

  log_info "Common Dockerfile issues:"
  echo "  • Missing files referenced in COPY commands"
  echo "  • Network issues downloading packages"
  echo "  • Invalid Dockerfile syntax"
  echo ""

  log_info "Try:"
  echo "  1) Check that all files referenced in Dockerfiles exist"
  echo "  2) Run: docker system prune -a (warning: removes all unused images)"
  echo "  3) Check your internet connection"
}

# Handle disk space errors
handle_disk_space_error() {
  log_error "No disk space available"

  echo ""
  log_info "Docker disk usage:"
  docker system df

  echo ""
  log_info "Options:"
  echo "  1) Clean Docker resources (safe)"
  echo "  2) Aggressive cleanup (removes all unused data)"
  echo "  3) Check system disk space"
  echo "  4) Cancel"

  read -p "Choose option [1-4]: " -n 1 -r
  echo ""

  case $REPLY in
    1)
      clean_docker_safe
      ;;
    2)
      clean_docker_aggressive
      ;;
    3)
      df -h
      ;;
    4)
      return 1
      ;;
  esac
}

# Safe Docker cleanup
clean_docker_safe() {
  log_info "Performing safe Docker cleanup..."

  docker container prune -f
  docker image prune -f
  docker network prune -f

  log_success "Cleanup complete"
  docker system df
}

# Aggressive Docker cleanup
clean_docker_aggressive() {
  log_warning "This will remove ALL unused Docker data!"
  read -p "Are you sure? [y/N]: " -n 1 -r
  echo ""

  if [[ $REPLY =~ ^[Yy]$ ]]; then
    log_info "Performing aggressive cleanup..."

    docker system prune -a -f --volumes

    log_success "Aggressive cleanup complete"
    docker system df
  fi
}

export -f analyze_build_failure
export -f handle_go_module_error
export -f fix_go_modules
export -f show_go_manual_fix
export -f disable_go_services
export -f handle_node_error
export -f fix_node_issues
export -f handle_dockerfile_error
export -f handle_disk_space_error
export -f clean_docker_safe
export -f clean_docker_aggressive
