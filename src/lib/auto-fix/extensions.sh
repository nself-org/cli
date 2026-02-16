#!/usr/bin/env bash

# extensions.sh - Auto-fix PostgreSQL extensions compatibility

# Source utilities
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

source "$SCRIPT_DIR/../utils/display.sh"

# Source platform compatibility for safe_sed_inline
source "$SCRIPT_DIR/../utils/platform-compat.sh" 2>/dev/null || {
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

# Check and fix PostgreSQL extensions
fix_postgres_extensions() {
  local extensions="${POSTGRES_EXTENSIONS:-uuid-ossp}"
  local needs_special_image=false
  local postgres_image="postgres:${POSTGRES_VERSION:-16-alpine}"

  # Check for extensions that need special images
  if [[ "$extensions" == *"timescaledb"* ]]; then
    needs_special_image=true
    postgres_image="timescale/timescaledb:latest-pg16"
    log_info "Detected TimescaleDB - using TimescaleDB image"
  elif [[ "$extensions" == *"postgis"* ]] && [[ "$extensions" == *"pgvector"* ]]; then
    needs_special_image=true
    postgres_image="ankane/pgvector:latest"
    log_info "Detected PostGIS + pgvector - using pgvector image with PostGIS"
  elif [[ "$extensions" == *"postgis"* ]]; then
    needs_special_image=true
    postgres_image="postgis/postgis:16-3.4"
    log_info "Detected PostGIS - using PostGIS image"
  elif [[ "$extensions" == *"pgvector"* ]]; then
    needs_special_image=true
    postgres_image="ankane/pgvector:latest"
    log_info "Detected pgvector - using pgvector image"
  fi

  # Update docker-compose.yml if needed
  if [[ "$needs_special_image" == "true" ]] && [[ -f "docker-compose.yml" ]]; then
    # Check current image
    current_image=$(grep -A2 "postgres:" docker-compose.yml | grep "image:" | awk '{print $2}')

    if [[ "$current_image" != "$postgres_image" ]]; then
      log_info "Updating PostgreSQL image to support requested extensions"
      safe_sed_inline docker-compose.yml "s|image: postgres:.*|image: $postgres_image|"
      log_success "PostgreSQL image updated to: $postgres_image"
      return 0
    fi
  fi

  return 0
}

# Validate extension combination
validate_extension_compatibility() {
  local extensions="${POSTGRES_EXTENSIONS:-}"

  # Check for incompatible combinations
  if [[ "$extensions" == *"timescaledb"* ]] && [[ "$extensions" == *"postgis"* ]]; then
    log_warning "TimescaleDB and PostGIS together may require custom image"
    log_info "Consider using timescale/timescaledb-ha:pg16-latest"
  fi

  return 0
}

# Export functions
export -f fix_postgres_extensions
export -f validate_extension_compatibility
