#!/usr/bin/env bash
# automated.sh - Automated backup system
# Part of nself v0.7.0 - Sprint 8: BDR-001


# Backup types
readonly BACKUP_TYPE_FULL="full"

set -euo pipefail

readonly BACKUP_TYPE_INCREMENTAL="incremental"
readonly BACKUP_TYPE_DIFFERENTIAL="differential"

# Initialize backup system
backup_init() {
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)
  [[ -z "$container" ]] && {
    echo "ERROR: PostgreSQL not found" >&2
    return 1
  }

  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE SCHEMA IF NOT EXISTS backups;

CREATE TABLE IF NOT EXISTS backups.schedules (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL UNIQUE,
  backup_type TEXT NOT NULL,
  frequency TEXT NOT NULL, -- hourly, daily, weekly, monthly
  retention_days INTEGER NOT NULL DEFAULT 30,
  include_secrets BOOLEAN DEFAULT FALSE,
  compress BOOLEAN DEFAULT TRUE,
  encrypt BOOLEAN DEFAULT TRUE,
  enabled BOOLEAN DEFAULT TRUE,
  last_run TIMESTAMPTZ,
  next_run TIMESTAMPTZ,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS backups.history (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  schedule_id UUID REFERENCES backups.schedules(id) ON DELETE SET NULL,
  backup_type TEXT NOT NULL,
  file_path TEXT NOT NULL,
  file_size_bytes BIGINT,
  compressed BOOLEAN DEFAULT FALSE,
  encrypted BOOLEAN DEFAULT FALSE,
  checksum TEXT,
  duration_seconds INTEGER,
  status TEXT NOT NULL, -- success, failed, partial
  error_message TEXT,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS backups.metadata (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  backup_id UUID NOT NULL REFERENCES backups.history(id) ON DELETE CASCADE,
  database_name TEXT,
  tables_count INTEGER,
  rows_count BIGINT,
  schema_version TEXT,
  backup_method TEXT,
  metadata JSONB DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_schedules_enabled ON backups.schedules(enabled) WHERE enabled = TRUE;
CREATE INDEX IF NOT EXISTS idx_schedules_next_run ON backups.schedules(next_run);
CREATE INDEX IF NOT EXISTS idx_history_created ON backups.history(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_history_status ON backups.history(status);
CREATE INDEX IF NOT EXISTS idx_metadata_backup ON backups.metadata(backup_id);
EOSQL

  # Create backup directory
  mkdir -p /var/backups/nself
  return 0
}

# Create backup
backup_create() {
  local backup_type="${1:-$BACKUP_TYPE_FULL}"
  local compress="${2:-true}"
  local encrypt="${3:-false}"
  local include_secrets="${4:-false}"

  local timestamp=$(date +%Y%m%d_%H%M%S)
  local backup_dir="/var/backups/nself"
  local backup_file="nself_backup_${timestamp}.sql"

  local start_time=$(date +%s)
  local status="success"
  local error_message=""

  # PostgreSQL backup
  local pg_container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)
  if [[ -n "$pg_container" ]]; then
    docker exec "$pg_container" pg_dump \
      -U "${POSTGRES_USER:-postgres}" \
      -d "${POSTGRES_DB:-nself_db}" \
      -F p \
      -f "/tmp/$backup_file" 2>/dev/null || {
      status="failed"
      error_message="PostgreSQL backup failed"
    }

    # Copy from container to host
    docker cp "$pg_container:/tmp/$backup_file" "$backup_dir/$backup_file" 2>/dev/null
    docker exec "$pg_container" rm "/tmp/$backup_file" 2>/dev/null
  fi

  # Compress if requested
  if [[ "$compress" == "true" ]] && [[ -f "$backup_dir/$backup_file" ]]; then
    gzip "$backup_dir/$backup_file"
    backup_file="${backup_file}.gz"
  fi

  # Calculate checksum
  local checksum=""
  if [[ -f "$backup_dir/$backup_file" ]]; then
    checksum=$(sha256sum "$backup_dir/$backup_file" | cut -d' ' -f1)
  fi

  local end_time=$(date +%s)
  local duration=$((end_time - start_time))
  local file_size=$(stat -f%z "$backup_dir/$backup_file" 2>/dev/null || stat -c%s "$backup_dir/$backup_file" 2>/dev/null || echo 0)

  # Record backup
  local db_container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)
  local backup_id=$(docker exec -i "$db_container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "INSERT INTO backups.history (backup_type, file_path, file_size_bytes, compressed, encrypted, checksum, duration_seconds, status, error_message)
     VALUES (
       '$backup_type',
       '$backup_dir/$backup_file',
       $file_size,
       $compress,
       $encrypt,
       '$checksum',
       $duration,
       '$status',
       $([ -n "$error_message" ] && echo "'$error_message'" || echo "NULL")
     )
     RETURNING id;" 2>/dev/null | xargs)

  echo "{\"backup_id\":\"$backup_id\",\"file\":\"$backup_dir/$backup_file\",\"size\":$file_size,\"status\":\"$status\"}"
}

# Schedule backup
backup_schedule_create() {
  local name="$1"
  local backup_type="$2"
  local frequency="$3" # hourly, daily, weekly, monthly
  local retention_days="${4:-30}"

  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  # Calculate next run time
  local next_run="NOW()"
  case "$frequency" in
    hourly) next_run="NOW() + INTERVAL '1 hour'" ;;
    daily) next_run="NOW() + INTERVAL '1 day'" ;;
    weekly) next_run="NOW() + INTERVAL '1 week'" ;;
    monthly) next_run="NOW() + INTERVAL '1 month'" ;;
  esac

  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO backups.schedules (name, backup_type, frequency, retention_days, next_run)
     VALUES ('$name', '$backup_type', '$frequency', $retention_days, $next_run)
     ON CONFLICT (name) DO UPDATE SET
       backup_type = EXCLUDED.backup_type,
       frequency = EXCLUDED.frequency,
       retention_days = EXCLUDED.retention_days,
       next_run = EXCLUDED.next_run,
       enabled = TRUE;" >/dev/null 2>&1
}

# Run scheduled backups
backup_run_scheduled() {
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  # Get due schedules
  local schedules=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(s) FROM (
       SELECT id, name, backup_type, frequency, include_secrets
       FROM backups.schedules
       WHERE enabled = TRUE
         AND next_run <= NOW()
     ) s;" 2>/dev/null | xargs)

  [[ -z "$schedules" || "$schedules" == "null" ]] && return 0

  # Run each scheduled backup
  echo "$schedules" | jq -c '.[]' | while read -r schedule; do
    local schedule_id=$(echo "$schedule" | jq -r '.id')
    local name=$(echo "$schedule" | jq -r '.name')
    local backup_type=$(echo "$schedule" | jq -r '.backup_type')
    local include_secrets=$(echo "$schedule" | jq -r '.include_secrets')
    local frequency=$(echo "$schedule" | jq -r '.frequency')

    echo "Running scheduled backup: $name"
    backup_create "$backup_type" "true" "false" "$include_secrets"

    # Update schedule
    local next_run="NOW()"
    case "$frequency" in
      hourly) next_run="NOW() + INTERVAL '1 hour'" ;;
      daily) next_run="NOW() + INTERVAL '1 day'" ;;
      weekly) next_run="NOW() + INTERVAL '1 week'" ;;
      monthly) next_run="NOW() + INTERVAL '1 month'" ;;
    esac

    docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
      "UPDATE backups.schedules
       SET last_run = NOW(),
           next_run = $next_run
       WHERE id = '$schedule_id';" >/dev/null 2>&1
  done
}

# List backups
backup_list() {
  local limit="${1:-50}"
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  local backups=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(b) FROM (
       SELECT
         id,
         backup_type,
         file_path,
         file_size_bytes,
         compressed,
         encrypted,
         status,
         created_at
       FROM backups.history
       ORDER BY created_at DESC
       LIMIT $limit
     ) b;" 2>/dev/null | xargs)

  [[ -z "$backups" || "$backups" == "null" ]] && echo "[]" || echo "$backups"
}

# Get backup info
backup_get() {
  local backup_id="$1"
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  local backup=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_build_object(
       'id', h.id,
       'backup_type', h.backup_type,
       'file_path', h.file_path,
       'file_size_bytes', h.file_size_bytes,
       'compressed', h.compressed,
       'encrypted', h.encrypted,
       'checksum', h.checksum,
       'duration_seconds', h.duration_seconds,
       'status', h.status,
       'created_at', h.created_at,
       'metadata', m.metadata
     )
     FROM backups.history h
     LEFT JOIN backups.metadata m ON h.id = m.backup_id
     WHERE h.id = '$backup_id';" 2>/dev/null | xargs)

  echo "$backup"
}

# Verify backup
backup_verify() {
  local backup_id="$1"
  local backup=$(backup_get "$backup_id")

  [[ -z "$backup" || "$backup" == "null" ]] && {
    echo "Backup not found"
    return 1
  }

  local file_path=$(echo "$backup" | jq -r '.file_path')
  local stored_checksum=$(echo "$backup" | jq -r '.checksum')

  # Check if file exists
  [[ ! -f "$file_path" ]] && {
    echo "Backup file not found"
    return 1
  }

  # Verify checksum
  local current_checksum=$(sha256sum "$file_path" | cut -d' ' -f1)

  if [[ "$stored_checksum" == "$current_checksum" ]]; then
    echo "✓ Backup verified"
    return 0
  else
    echo "✗ Backup corrupted (checksum mismatch)"
    return 1
  fi
}

export -f backup_init backup_create backup_schedule_create backup_run_scheduled
export -f backup_list backup_get backup_verify
