#!/usr/bin/env bash

# directory-setup.sh - Directory structure creation module
# POSIX-compliant, no Bash 4+ features

# Define standard project directory structure
get_project_directories() {

set -euo pipefail

  # Always needed directories
  cat <<EOF
nginx/conf.d
nginx/ssl
services
logs
postgres/init
ssl/certificates
.volumes/postgres
EOF

  # Conditional volume directories based on enabled services
  if [[ "${REDIS_ENABLED:-false}" == "true" ]]; then
    echo ".volumes/redis"
  fi

  if [[ "${MINIO_ENABLED:-false}" == "true" ]]; then
    echo ".volumes/minio"
  fi
}

# Create directory structure for the project
create_directory_structure() {
  local dirs_to_create=0
  local dirs_created=0
  local failed_dirs=""

  # Get list of directories
  local directories="$(get_project_directories)"

  # Check what needs to be created
  while IFS= read -r dir; do
    if [[ -n "$dir" ]] && [[ ! -d "$dir" ]]; then
      dirs_to_create=$((dirs_to_create + 1))
    fi
  done <<<"$directories"

  if [[ $dirs_to_create -eq 0 ]]; then
    return 0
  fi

  # Create directories
  while IFS= read -r dir; do
    if [[ -n "$dir" ]] && [[ ! -d "$dir" ]]; then
      if mkdir -p "$dir" 2>/dev/null; then
        dirs_created=$((dirs_created + 1))
      else
        failed_dirs="${failed_dirs}${dir} "
      fi
    fi
  done <<<"$directories"

  # Return status
  if [[ -n "$failed_dirs" ]]; then
    echo "Failed to create: $failed_dirs" >&2
    return 1
  fi

  return 0
}

# Check if directory structure exists
check_directory_structure() {
  local missing_count=0
  local directories="$(get_project_directories)"

  while IFS= read -r dir; do
    if [[ -n "$dir" ]] && [[ ! -d "$dir" ]]; then
      missing_count=$((missing_count + 1))
    fi
  done <<<"$directories"

  echo "$missing_count"
}

# Create project-specific directories
create_service_directories() {
  local services=""

  # Check which services are enabled
  if [[ "${HASURA_ENABLED:-false}" == "true" ]]; then
    services="${services} hasura/metadata hasura/migrations"
  fi

  if [[ "${AUTH_ENABLED:-false}" == "true" ]]; then
    services="${services} auth/config"
  fi

  if [[ "${STORAGE_ENABLED:-false}" == "true" ]]; then
    services="${services} storage/uploads storage/temp"
  fi

  if [[ "${FUNCTIONS_ENABLED:-false}" == "true" ]]; then
    services="${services} functions/src functions/dist"
  fi

  # NestJS should be a custom service, not a special folder
  # Remove this legacy pattern

  # Create service directories
  for dir in $services; do
    if [[ ! -d "$dir" ]]; then
      mkdir -p "$dir" 2>/dev/null
    fi
  done

  # NOW create functions package files after directories exist
  if [[ "${FUNCTIONS_ENABLED:-false}" == "true" ]]; then
    if [[ ! -f "functions/package.json" ]]; then
      cat >functions/package.json <<'EOF'
{
  "name": "functions",
  "version": "1.0.0",
  "description": "Serverless functions",
  "main": "index.js",
  "dependencies": {}
}
EOF
    fi

    if [[ ! -f "functions/package-lock.json" ]]; then
      cat >functions/package-lock.json <<'EOF'
{
  "name": "functions",
  "version": "1.0.0",
  "lockfileVersion": 2,
  "requires": true,
  "packages": {
    "": {
      "name": "functions",
      "version": "1.0.0",
      "dependencies": {}
    }
  },
  "dependencies": {}
}
EOF
    fi
  fi
}

# Create frontend app directories
create_frontend_directories() {
  # Frontend apps are external to nself - no directories needed
  # This function is kept for compatibility but does nothing
  return 0
}

# Set proper permissions for directories
set_directory_permissions() {
  local user_id="${USER_ID:-1000}"
  local group_id="${GROUP_ID:-1000}"

  # Set permissions for volume directories
  if command -v chown >/dev/null 2>&1; then
    # Try to set ownership (may fail without sudo)
    for dir in .volumes/postgres .volumes/redis .volumes/minio; do
      if [[ -d "$dir" ]]; then
        chown -R "${user_id}:${group_id}" "$dir" 2>/dev/null || true
      fi
    done
  fi

  # Ensure directories are writable
  for dir in logs .volumes services; do
    if [[ -d "$dir" ]]; then
      chmod -R 755 "$dir" 2>/dev/null || true
    fi
  done
}

# Clean up empty directories
cleanup_empty_directories() {
  # Find and remove empty directories in services
  if [[ -d "services" ]]; then
    find services -type d -empty -delete 2>/dev/null || true
  fi

  # Clean up empty volume directories
  for dir in .volumes/*; do
    if [[ -d "$dir" ]] && [[ -z "$(ls -A "$dir" 2>/dev/null)" ]]; then
      rmdir "$dir" 2>/dev/null || true
    fi
  done
}

# Main setup function
setup_project_directories() {
  local force="${1:-false}"

  # Create base directory structure
  if ! create_directory_structure; then
    return 1
  fi

  # Create service-specific directories
  create_service_directories

  # Create frontend directories
  create_frontend_directories

  # Set permissions
  set_directory_permissions

  # Cleanup if requested
  if [[ "$force" == "cleanup" ]]; then
    cleanup_empty_directories
  fi

  return 0
}

# Export functions
export -f get_project_directories
export -f create_directory_structure
export -f check_directory_structure
export -f create_service_directories
export -f create_frontend_directories
export -f set_directory_permissions
export -f cleanup_empty_directories
export -f setup_project_directories
