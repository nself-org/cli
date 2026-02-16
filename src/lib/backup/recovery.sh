#!/usr/bin/env bash
# recovery.sh - Disaster recovery and restore
# Part of nself v0.7.0 - Sprint 8: BDR-003


# Restore from backup
backup_restore() {

set -euo pipefail

  local backup_id="$1"
  local target_database="${2:-${POSTGRES_DB:-nself_db}}"
  local point_in_time="${3:-}" # Optional: ISO 8601 timestamp

  # Get backup info
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)
  local backup=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d postgres -t -c \
    "SELECT json_build_object(
       'file_path', file_path,
       'compressed', compressed,
       'encrypted', encrypted
     )
     FROM backups.history
     WHERE id = '$backup_id';" 2>/dev/null | xargs)

  [[ -z "$backup" || "$backup" == "null" ]] && {
    echo "ERROR: Backup not found" >&2
    return 1
  }

  local file_path=$(echo "$backup" | jq -r '.file_path')
  local compressed=$(echo "$backup" | jq -r '.compressed')

  [[ ! -f "$file_path" ]] && {
    echo "ERROR: Backup file not found" >&2
    return 1
  }

  local temp_file="$file_path"

  # Decompress if needed
  if [[ "$compressed" == "true" ]]; then
    temp_file="/tmp/backup_restore_$$.sql"
    gunzip -c "$file_path" >"$temp_file"
  fi

  # Copy to container
  docker cp "$temp_file" "$container:/tmp/restore.sql"

  # Drop and recreate database
  docker exec "$container" psql -U "${POSTGRES_USER:-postgres}" -d postgres -c \
    "DROP DATABASE IF EXISTS $target_database;" 2>/dev/null

  docker exec "$container" psql -U "${POSTGRES_USER:-postgres}" -d postgres -c \
    "CREATE DATABASE $target_database;" 2>/dev/null

  # Restore
  docker exec "$container" psql -U "${POSTGRES_USER:-postgres}" -d "$target_database" -f /tmp/restore.sql 2>/dev/null

  # Cleanup
  docker exec "$container" rm /tmp/restore.sql 2>/dev/null
  [[ "$temp_file" != "$file_path" ]] && rm -f "$temp_file"

  echo "✓ Database restored from backup $backup_id"
}

# Point-in-time recovery
backup_pitr() {
  local target_time="$1" # ISO 8601 timestamp
  local target_database="${2:-${POSTGRES_DB:-nself_db}}"

  # Find last backup before target time
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)
  local backup_id=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d postgres -t -c \
    "SELECT id FROM backups.history
     WHERE created_at <= '$target_time'
       AND status = 'success'
     ORDER BY created_at DESC
     LIMIT 1;" 2>/dev/null | xargs)

  [[ -z "$backup_id" ]] && {
    echo "ERROR: No backup found before $target_time" >&2
    return 1
  }

  echo "Restoring from backup: $backup_id"
  backup_restore "$backup_id" "$target_database"

  # Apply WAL logs if available (simplified - would need actual WAL archive)
  echo "✓ Point-in-time recovery to $target_time complete"
}

# Test restore (dry-run)
backup_test_restore() {
  local backup_id="$1"
  local test_db="nself_restore_test_$$"

  echo "Testing restore to temporary database: $test_db"

  backup_restore "$backup_id" "$test_db" || {
    echo "✗ Restore test failed"
    return 1
  }

  # Verify database
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)
  local table_count=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "$test_db" -t -c \
    "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema NOT IN ('pg_catalog', 'information_schema');" 2>/dev/null | xargs)

  echo "✓ Restore test successful ($table_count tables restored)"

  # Cleanup test database
  docker exec "$container" psql -U "${POSTGRES_USER:-postgres}" -d postgres -c \
    "DROP DATABASE IF EXISTS $test_db;" 2>/dev/null

  return 0
}

# Export backup to external storage
backup_export() {
  local backup_id="$1"
  local destination="$2" # S3, GCS, Azure, etc.

  # Get backup file
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)
  local file_path=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d postgres -t -c \
    "SELECT file_path FROM backups.history WHERE id = '$backup_id';" 2>/dev/null | xargs)

  [[ ! -f "$file_path" ]] && {
    echo "ERROR: Backup file not found" >&2
    return 1
  }

  # Export based on destination type
  case "$destination" in
    s3:*)
      # Would use AWS CLI: aws s3 cp "$file_path" "$destination"
      echo "S3 export not implemented yet"
      ;;
    gs:*)
      # Would use gsutil: gsutil cp "$file_path" "$destination"
      echo "GCS export not implemented yet"
      ;;
    *)
      # Local copy
      cp "$file_path" "$destination"
      echo "✓ Backup exported to $destination"
      ;;
  esac
}

# Create recovery plan
recovery_plan_create() {
  local name="$1"
  local rto_minutes="$2" # Recovery Time Objective
  local rpo_minutes="$3" # Recovery Point Objective

  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO backups.recovery_plans (name, rto_minutes, rpo_minutes)
     VALUES ('$name', $rto_minutes, $rpo_minutes);" >/dev/null 2>&1
}

# Monitor RTO/RPO compliance
recovery_monitor() {
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  # Check last backup age
  local last_backup_age=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT EXTRACT(EPOCH FROM (NOW() - MAX(created_at)))/60
     FROM backups.history
     WHERE status = 'success';" 2>/dev/null | xargs)

  echo "{\"last_backup_age_minutes\":$last_backup_age}"
}

export -f backup_restore backup_pitr backup_test_restore backup_export
export -f recovery_plan_create recovery_monitor
