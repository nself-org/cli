#!/usr/bin/env bash
# cache.sh - Redis caching layer
# Part of nself v0.7.0 - Sprint 6: RDS-004


# Default cache TTL (1 hour)
readonly CACHE_DEFAULT_TTL=3600

set -euo pipefail


# Cache set
redis_cache_set() {
  local key="$1"
  local value="$2"
  local ttl="${3:-$CACHE_DEFAULT_TTL}"
  local connection_name="${4:-main}"

  local redis_container=$(docker ps --filter 'name=redis' --format '{{.Names}}' | head -1)
  [[ -z "$redis_container" ]] && return 1

  local conn=$(redis_connection_get "$connection_name" 2>/dev/null)
  [[ -z "$conn" || "$conn" == "null" ]] && return 1

  local host=$(echo "$conn" | jq -r '.host')
  local port=$(echo "$conn" | jq -r '.port')
  local database=$(echo "$conn" | jq -r '.database')

  local cache_key="cache:$key"

  docker exec "$redis_container" redis-cli -h "$host" -p "$port" -n "$database" \
    SET "$cache_key" "$value" EX "$ttl" >/dev/null 2>&1
}

# Cache get
redis_cache_get() {
  local key="$1"
  local connection_name="${2:-main}"

  local redis_container=$(docker ps --filter 'name=redis' --format '{{.Names}}' | head -1)
  [[ -z "$redis_container" ]] && {
    echo "null"
    return 1
  }

  local conn=$(redis_connection_get "$connection_name" 2>/dev/null)
  [[ -z "$conn" || "$conn" == "null" ]] && {
    echo "null"
    return 1
  }

  local host=$(echo "$conn" | jq -r '.host')
  local port=$(echo "$conn" | jq -r '.port')
  local database=$(echo "$conn" | jq -r '.database')

  local cache_key="cache:$key"

  local value=$(docker exec "$redis_container" redis-cli -h "$host" -p "$port" -n "$database" \
    GET "$cache_key" 2>/dev/null)

  [[ -z "$value" || "$value" == "null" ]] && {
    echo "null"
    return 1
  }

  echo "$value"
}

# Cache delete
redis_cache_delete() {
  local key="$1"
  local connection_name="${2:-main}"

  local redis_container=$(docker ps --filter 'name=redis' --format '{{.Names}}' | head -1)
  [[ -z "$redis_container" ]] && return 1

  local conn=$(redis_connection_get "$connection_name" 2>/dev/null)
  [[ -z "$conn" || "$conn" == "null" ]] && return 1

  local host=$(echo "$conn" | jq -r '.host')
  local port=$(echo "$conn" | jq -r '.port')
  local database=$(echo "$conn" | jq -r '.database')

  local cache_key="cache:$key"

  docker exec "$redis_container" redis-cli -h "$host" -p "$port" -n "$database" \
    DEL "$cache_key" >/dev/null 2>&1
}

# Cache invalidate by pattern
redis_cache_invalidate() {
  local pattern="$1"
  local connection_name="${2:-main}"

  local redis_container=$(docker ps --filter 'name=redis' --format '{{.Names}}' | head -1)
  [[ -z "$redis_container" ]] && return 1

  local conn=$(redis_connection_get "$connection_name" 2>/dev/null)
  [[ -z "$conn" || "$conn" == "null" ]] && return 1

  local host=$(echo "$conn" | jq -r '.host')
  local port=$(echo "$conn" | jq -r '.port')
  local database=$(echo "$conn" | jq -r '.database')

  local cache_pattern="cache:$pattern"

  # Get all matching keys
  local keys=$(docker exec "$redis_container" redis-cli -h "$host" -p "$port" -n "$database" \
    KEYS "$cache_pattern" 2>/dev/null)

  [[ -z "$keys" ]] && return 0

  # Delete all matching keys
  while IFS= read -r key; do
    [[ -z "$key" ]] && continue
    docker exec "$redis_container" redis-cli -h "$host" -p "$port" -n "$database" \
      DEL "$key" >/dev/null 2>&1
  done <<<"$keys"
}

# Cache exists
redis_cache_exists() {
  local key="$1"
  local connection_name="${2:-main}"

  local redis_container=$(docker ps --filter 'name=redis' --format '{{.Names}}' | head -1)
  [[ -z "$redis_container" ]] && return 1

  local conn=$(redis_connection_get "$connection_name" 2>/dev/null)
  [[ -z "$conn" || "$conn" == "null" ]] && return 1

  local host=$(echo "$conn" | jq -r '.host')
  local port=$(echo "$conn" | jq -r '.port')
  local database=$(echo "$conn" | jq -r '.database')

  local cache_key="cache:$key"

  local exists=$(docker exec "$redis_container" redis-cli -h "$host" -p "$port" -n "$database" \
    EXISTS "$cache_key" 2>/dev/null)

  [[ "$exists" == "1" ]] && return 0 || return 1
}

# Cache TTL
redis_cache_ttl() {
  local key="$1"
  local connection_name="${2:-main}"

  local redis_container=$(docker ps --filter 'name=redis' --format '{{.Names}}' | head -1)
  [[ -z "$redis_container" ]] && {
    echo "0"
    return 1
  }

  local conn=$(redis_connection_get "$connection_name" 2>/dev/null)
  [[ -z "$conn" || "$conn" == "null" ]] && {
    echo "0"
    return 1
  }

  local host=$(echo "$conn" | jq -r '.host')
  local port=$(echo "$conn" | jq -r '.port')
  local database=$(echo "$conn" | jq -r '.database')

  local cache_key="cache:$key"

  local ttl=$(docker exec "$redis_container" redis-cli -h "$host" -p "$port" -n "$database" \
    TTL "$cache_key" 2>/dev/null || echo "0")

  echo "$ttl"
}

# Cache warming - preload data
redis_cache_warm() {
  local cache_config="$1" # JSON array of {key, query, ttl}
  local connection_name="${2:-main}"

  local count=0
  echo "$cache_config" | jq -c '.[]' | while read -r item; do
    local key=$(echo "$item" | jq -r '.key')
    local query=$(echo "$item" | jq -r '.query')
    local ttl=$(echo "$item" | jq -r '.ttl // 3600')

    # Execute query (assumes PostgreSQL)
    local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)
    if [[ -n "$container" ]]; then
      local result=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
        "$query" 2>/dev/null | xargs)

      if [[ -n "$result" ]]; then
        redis_cache_set "$key" "$result" "$ttl" "$connection_name"
        count=$((count + 1))
      fi
    fi
  done

  echo "Warmed $count cache entries"
}

# Cache statistics
redis_cache_stats() {
  local connection_name="${1:-main}"

  local redis_container=$(docker ps --filter 'name=redis' --format '{{.Names}}' | head -1)
  [[ -z "$redis_container" ]] && {
    echo "{}"
    return 1
  }

  local conn=$(redis_connection_get "$connection_name" 2>/dev/null)
  [[ -z "$conn" || "$conn" == "null" ]] && {
    echo "{}"
    return 1
  }

  local host=$(echo "$conn" | jq -r '.host')
  local port=$(echo "$conn" | jq -r '.port')
  local database=$(echo "$conn" | jq -r '.database')

  # Get cache keys count
  local cache_keys=$(docker exec "$redis_container" redis-cli -h "$host" -p "$port" -n "$database" \
    KEYS "cache:*" 2>/dev/null | wc -l)

  # Get Redis info
  local info=$(docker exec "$redis_container" redis-cli -h "$host" -p "$port" -n "$database" \
    INFO stats 2>/dev/null)

  local hits=$(echo "$info" | grep keyspace_hits | cut -d: -f2 | tr -d '\r')
  local misses=$(echo "$info" | grep keyspace_misses | cut -d: -f2 | tr -d '\r')
  local total=$((hits + misses))
  local hit_rate=0

  [[ $total -gt 0 ]] && hit_rate=$(echo "scale=2; $hits * 100 / $total" | bc)

  echo "{\"cache_keys\":$cache_keys,\"hits\":$hits,\"misses\":$misses,\"hit_rate\":$hit_rate}"
}

# Cache get or set (lazy loading)
redis_cache_get_or_set() {
  local key="$1"
  local generator_func="$2" # Function to call if cache miss
  local ttl="${3:-$CACHE_DEFAULT_TTL}"
  local connection_name="${4:-main}"

  # Try to get from cache
  local value=$(redis_cache_get "$key" "$connection_name" 2>/dev/null)

  if [[ "$value" != "null" ]]; then
    echo "$value"
    return 0
  fi

  # Cache miss - generate value
  value=$($generator_func)

  # Store in cache
  redis_cache_set "$key" "$value" "$ttl" "$connection_name"

  echo "$value"
}

# Cache with tags for group invalidation
redis_cache_tag_add() {
  local key="$1"
  local tags="$2" # Comma-separated
  local connection_name="${3:-main}"

  local redis_container=$(docker ps --filter 'name=redis' --format '{{.Names}}' | head -1)
  [[ -z "$redis_container" ]] && return 1

  local conn=$(redis_connection_get "$connection_name" 2>/dev/null)
  [[ -z "$conn" || "$conn" == "null" ]] && return 1

  local host=$(echo "$conn" | jq -r '.host')
  local port=$(echo "$conn" | jq -r '.port')
  local database=$(echo "$conn" | jq -r '.database')

  IFS=',' read -ra tag_array <<<"$tags"
  for tag in "${tag_array[@]}"; do
    [[ -z "$tag" ]] && continue
    local tag_key="cache:tag:$tag"
    docker exec "$redis_container" redis-cli -h "$host" -p "$port" -n "$database" \
      SADD "$tag_key" "$key" >/dev/null 2>&1
  done
}

# Invalidate cache by tag
redis_cache_invalidate_tag() {
  local tag="$1"
  local connection_name="${2:-main}"

  local redis_container=$(docker ps --filter 'name=redis' --format '{{.Names}}' | head -1)
  [[ -z "$redis_container" ]] && return 1

  local conn=$(redis_connection_get "$connection_name" 2>/dev/null)
  [[ -z "$conn" || "$conn" == "null" ]] && return 1

  local host=$(echo "$conn" | jq -r '.host')
  local port=$(echo "$conn" | jq -r '.port')
  local database=$(echo "$conn" | jq -r '.database')

  local tag_key="cache:tag:$tag"

  # Get all keys with this tag
  local keys=$(docker exec "$redis_container" redis-cli -h "$host" -p "$port" -n "$database" \
    SMEMBERS "$tag_key" 2>/dev/null)

  [[ -z "$keys" ]] && return 0

  # Delete all keys
  while IFS= read -r key; do
    [[ -z "$key" ]] && continue
    redis_cache_delete "$key" "$connection_name"
  done <<<"$keys"

  # Delete tag set
  docker exec "$redis_container" redis-cli -h "$host" -p "$port" -n "$database" \
    DEL "$tag_key" >/dev/null 2>&1
}

export -f redis_cache_set redis_cache_get redis_cache_delete redis_cache_invalidate
export -f redis_cache_exists redis_cache_ttl redis_cache_warm redis_cache_stats
export -f redis_cache_get_or_set redis_cache_tag_add redis_cache_invalidate_tag
