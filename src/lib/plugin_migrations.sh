#!/usr/bin/env bash
# plugin_migrations.sh — Plugin schema migration helpers for the nself CLI.
#
# Functions:
#   plugin_migration_current_version <plugin> <db_url>
#   plugin_migration_run_up          <plugin> <db_url> <migrations_dir>
#   plugin_migration_run_down        <plugin> <db_url> <migrations_dir> <target_version>
#
# Migration files must be named:
#   <NNN>_description.up.sql
#   <NNN>_description.down.sql
#
# Bash 3.2 compatible — no echo -e, no declare -A, no ${var,,}.

set -euo pipefail

# ---------------------------------------------------------------------------
# plugin_migration_current_version
#   Returns the highest version applied in the 'up' direction for <plugin>.
#   Outputs 0 if no migrations have been applied yet.
# ---------------------------------------------------------------------------
plugin_migration_current_version() {
  local plugin="$1"
  local db_url="$2"

  local version
  version=$(psql "$db_url" -t -A -c "
    SELECT COALESCE(MAX(version), 0)
    FROM np_migration_versions
    WHERE plugin_name = '${plugin}'
      AND direction = 'up';
  " 2>/dev/null) || version=0

  printf "%s\n" "$version"
}

# ---------------------------------------------------------------------------
# plugin_migration_run_up
#   Apply all pending up-migrations for <plugin> in ascending order.
#   Records each applied version in np_migration_versions.
# ---------------------------------------------------------------------------
plugin_migration_run_up() {
  local plugin="$1"
  local db_url="$2"
  local migrations_dir="$3"

  if [ ! -d "$migrations_dir" ]; then
    printf "[plugin_migrations] No migrations dir: %s\n" "$migrations_dir" >&2
    return 0
  fi

  local current_version
  current_version=$(plugin_migration_current_version "$plugin" "$db_url")

  local applied=0

  # Iterate migration files in numeric order (Bash 3.2 compatible glob sort)
  for f in "$migrations_dir"/*.up.sql; do
    [ -f "$f" ] || continue

    # Extract version number from filename (e.g., 003_add_column.up.sql -> 3)
    local filename
    filename=$(basename "$f")
    local version_str
    version_str=$(printf "%s" "$filename" | sed 's/^0*\([0-9][0-9]*\)_.*/\1/')
    local version
    version=$(printf "%d" "$version_str" 2>/dev/null) || continue

    if [ "$version" -le "$current_version" ]; then
      continue
    fi

    printf "[plugin_migrations] Applying %s migration v%d: %s\n" "$plugin" "$version" "$filename"

    # Apply the SQL file
    if psql "$db_url" -f "$f" >/dev/null 2>&1; then
      # Record the applied version
      psql "$db_url" -c "
        INSERT INTO np_migration_versions (plugin_name, version, direction)
        VALUES ('${plugin}', ${version}, 'up')
        ON CONFLICT DO NOTHING;
      " >/dev/null 2>&1

      applied=$((applied + 1))
      printf "[plugin_migrations] Applied v%d successfully\n" "$version"
    else
      printf "[plugin_migrations] ERROR: Failed to apply v%d for plugin %s\n" "$version" "$plugin" >&2
      return 1
    fi
  done

  if [ "$applied" -eq 0 ]; then
    printf "[plugin_migrations] %s: schema is up to date (v%d)\n" "$plugin" "$current_version"
  else
    printf "[plugin_migrations] %s: applied %d migration(s)\n" "$plugin" "$applied"
  fi
}

# ---------------------------------------------------------------------------
# plugin_migration_run_down
#   Roll back to <target_version> by running down-migrations in descending order.
#   If no down-migration file exists for a version, logs a WARNING and continues.
# ---------------------------------------------------------------------------
plugin_migration_run_down() {
  local plugin="$1"
  local db_url="$2"
  local migrations_dir="$3"
  local target_version="${4:-0}"

  if [ ! -d "$migrations_dir" ]; then
    printf "[plugin_migrations] No migrations dir: %s\n" "$migrations_dir" >&2
    return 1
  fi

  local current_version
  current_version=$(plugin_migration_current_version "$plugin" "$db_url")

  if [ "$current_version" -le "$target_version" ]; then
    printf "[plugin_migrations] %s: already at v%d, nothing to roll back\n" \
      "$plugin" "$current_version"
    return 0
  fi

  # Build a descending list of versions to roll back
  local version=$current_version
  while [ "$version" -gt "$target_version" ]; do
    # Look for the down migration file for this version
    local down_file=""
    for f in "$migrations_dir"/*.down.sql; do
      [ -f "$f" ] || continue
      local fn
      fn=$(basename "$f")
      local v_str
      v_str=$(printf "%s" "$fn" | sed 's/^0*\([0-9][0-9]*\)_.*/\1/')
      local v
      v=$(printf "%d" "$v_str" 2>/dev/null) || continue
      if [ "$v" -eq "$version" ]; then
        down_file="$f"
        break
      fi
    done

    if [ -z "$down_file" ]; then
      printf "[plugin_migrations] WARNING: rolled back to v%d but no down-migration for schema v%d — DB schema is ahead of binary. Proceed with caution.\n" \
        "$target_version" "$version" >&2
    else
      printf "[plugin_migrations] Rolling back %s v%d: %s\n" "$plugin" "$version" "$(basename "$down_file")"

      if psql "$db_url" -f "$down_file" >/dev/null 2>&1; then
        psql "$db_url" -c "
          INSERT INTO np_migration_versions (plugin_name, version, direction)
          VALUES ('${plugin}', ${version}, 'down')
          ON CONFLICT DO NOTHING;
        " >/dev/null 2>&1

        printf "[plugin_migrations] Rolled back v%d\n" "$version"
      else
        printf "[plugin_migrations] ERROR: Failed to roll back v%d for plugin %s\n" "$version" "$plugin" >&2
        return 1
      fi
    fi

    version=$((version - 1))
  done

  printf "[plugin_migrations] %s: rolled back to v%d\n" "$plugin" "$target_version"
}

# ---------------------------------------------------------------------------
# plugin_migration_check_version
#   Compare the binary's embedded schema version against the DB version.
#   Logs appropriate messages. Does NOT abort on schema-ahead condition.
# ---------------------------------------------------------------------------
plugin_migration_check_version() {
  local plugin="$1"
  local db_url="$2"
  local binary_version="$3"
  local migrations_dir="${4:-}"

  local db_version
  db_version=$(plugin_migration_current_version "$plugin" "$db_url")

  if [ "$binary_version" -gt "$db_version" ]; then
    printf "[plugin_migrations] %s: schema upgrade needed (DB v%d → binary v%d)\n" \
      "$plugin" "$db_version" "$binary_version"
    if [ -n "$migrations_dir" ]; then
      plugin_migration_run_up "$plugin" "$db_url" "$migrations_dir"
    fi
  elif [ "$binary_version" -lt "$db_version" ]; then
    printf "[plugin_migrations] WARNING: %s binary v%d is behind DB schema v%d. Rolled-back binary detected. Proceed with caution.\n" \
      "$plugin" "$binary_version" "$db_version" >&2
  else
    printf "[plugin_migrations] %s: schema v%d is current\n" "$plugin" "$binary_version"
  fi
}

export -f plugin_migration_current_version
export -f plugin_migration_run_up
export -f plugin_migration_run_down
export -f plugin_migration_check_version
