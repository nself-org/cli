#!/usr/bin/env bash

# build-cache.sh - Smart caching system for nself build
# Part of nself v0.9.8 - Performance Optimization
# POSIX-compliant, no Bash 4+ features

# Cache directory
CACHE_DIR="${NSELF_CACHE_DIR:-.nself/cache}"

set -euo pipefail

CACHE_MANIFEST="$CACHE_DIR/build-manifest.txt"

# Initialize cache system
init_build_cache() {
  mkdir -p "$CACHE_DIR"

  if [[ ! -f "$CACHE_MANIFEST" ]]; then
    printf "# nself build cache manifest\n" > "$CACHE_MANIFEST"
    printf "# Format: filepath|checksum|timestamp\n" >> "$CACHE_MANIFEST"
  fi
}

# Calculate file checksum (fast)
get_file_checksum() {
  local file="$1"

  if [[ ! -f "$file" ]]; then
    echo "missing"
    return 1
  fi

  # Use fastest available checksum tool
  if command -v md5sum >/dev/null 2>&1; then
    md5sum "$file" 2>/dev/null | awk '{print $1}'
  elif command -v md5 >/dev/null 2>&1; then
    md5 -q "$file" 2>/dev/null
  else
    # Fallback: use file size + mtime
    local size=$(wc -c < "$file" 2>/dev/null || echo "0")
    local mtime=$(stat -c %Y "$file" 2>/dev/null || stat -f %m "$file" 2>/dev/null || echo "0")
    echo "${size}-${mtime}"
  fi
}

# Get cached checksum for file
get_cached_checksum() {
  local file="$1"

  if [[ ! -f "$CACHE_MANIFEST" ]]; then
    echo ""
    return 1
  fi

  grep "^${file}|" "$CACHE_MANIFEST" 2>/dev/null | cut -d'|' -f2
}

# Update cache entry
update_cache_entry() {
  local file="$1"
  local checksum="$2"
  local timestamp=$(date +%s)

  # Remove old entry
  if [[ -f "$CACHE_MANIFEST" ]]; then
    grep -v "^${file}|" "$CACHE_MANIFEST" > "${CACHE_MANIFEST}.tmp" 2>/dev/null || true
    mv "${CACHE_MANIFEST}.tmp" "$CACHE_MANIFEST"
  fi

  # Add new entry
  printf "%s|%s|%s\n" "$file" "$checksum" "$timestamp" >> "$CACHE_MANIFEST"
}

# Check if file changed since last build
file_changed() {
  local file="$1"

  local current_checksum=$(get_file_checksum "$file")
  local cached_checksum=$(get_cached_checksum "$file")

  if [[ -z "$cached_checksum" ]] || [[ "$current_checksum" != "$cached_checksum" ]]; then
    return 0  # Changed or new
  else
    return 1  # Not changed
  fi
}

# Check if any source files changed
check_sources_changed() {
  local changed=0

  # Check environment files
  for env_file in .env .env.dev .env.local; do
    if [[ -f "$env_file" ]] && file_changed "$env_file"; then
      changed=1
      break
    fi
  done

  # Check custom service definitions if they exist
  if [[ $changed -eq 0 ]]; then
    for i in {1..10}; do
      local cs_var="CS_${i}"
      local cs_val="${!cs_var:-}"
      if [[ -n "$cs_val" ]]; then
        # Parse CS definition to get service directory
        local service_name=$(echo "$cs_val" | cut -d':' -f1 | tr -d ' ')
        if [[ -d "services/$service_name" ]]; then
          # Check main service file
          if [[ -f "services/$service_name/package.json" ]] && file_changed "services/$service_name/package.json"; then
            changed=1
            break
          fi
        fi
      fi
    done
  fi

  return $changed
}

# Check if docker-compose.yml needs regeneration
needs_compose_rebuild() {
  # Always regenerate if file doesn't exist
  if [[ ! -f "docker-compose.yml" ]]; then
    return 0
  fi

  # Check if env files changed
  if check_sources_changed; then
    return 0
  fi

  # Check docker-compose.yml age vs cache manifest
  if [[ -f "$CACHE_MANIFEST" ]]; then
    local compose_mtime=$(stat -c %Y "docker-compose.yml" 2>/dev/null || stat -f %m "docker-compose.yml" 2>/dev/null || echo "0")
    local manifest_mtime=$(stat -c %Y "$CACHE_MANIFEST" 2>/dev/null || stat -f %m "$CACHE_MANIFEST" 2>/dev/null || echo "0")

    if [[ "$manifest_mtime" -gt "$compose_mtime" ]]; then
      return 0  # Sources changed after last compose generation
    fi
  fi

  return 1  # No rebuild needed
}

# Check if nginx config needs regeneration
needs_nginx_rebuild() {
  # Always regenerate if main config doesn't exist
  if [[ ! -f "nginx/nginx.conf" ]]; then
    return 0
  fi

  # Check if env files changed (domain/routing changes)
  if check_sources_changed; then
    return 0
  fi

  return 1  # No rebuild needed
}

# Check if SSL certs need regeneration
needs_ssl_rebuild() {
  local base_domain="${BASE_DOMAIN:-localhost}"

  # Check if certs exist
  if [[ ! -f "ssl/certificates/localhost/fullchain.pem" ]]; then
    return 0
  fi

  # Check if domain changed
  local cached_domain=$(get_cached_checksum "BASE_DOMAIN")
  if [[ "$cached_domain" != "$base_domain" ]]; then
    return 0
  fi

  # Check cert expiration (if older than 30 days, regenerate)
  local cert_age=$(find ssl/certificates/localhost/fullchain.pem -mtime +30 2>/dev/null | wc -l | tr -d ' ')
  if [[ "$cert_age" -gt 0 ]]; then
    return 0
  fi

  return 1  # No rebuild needed
}

# Mark component as cached
mark_cached() {
  local component="$1"
  local checksum="${2:-$(date +%s)}"

  update_cache_entry "$component" "$checksum"
}

# Clear cache
clear_build_cache() {
  if [[ -d "$CACHE_DIR" ]]; then
    rm -rf "$CACHE_DIR"
  fi
  init_build_cache
}

# Get cache statistics
get_cache_stats() {
  if [[ ! -f "$CACHE_MANIFEST" ]]; then
    printf "Cache: empty\n"
    return
  fi

  local entry_count=$(grep -v '^#' "$CACHE_MANIFEST" 2>/dev/null | wc -l | tr -d ' ')
  local cache_size=$(du -sh "$CACHE_DIR" 2>/dev/null | cut -f1)

  printf "Cache: %d entries, %s\n" "$entry_count" "${cache_size:-0K}"
}

# Export functions
export -f init_build_cache
export -f get_file_checksum
export -f get_cached_checksum
export -f update_cache_entry
export -f file_changed
export -f check_sources_changed
export -f needs_compose_rebuild
export -f needs_nginx_rebuild
export -f needs_ssl_rebuild
export -f mark_cached
export -f clear_build_cache
export -f get_cache_stats
