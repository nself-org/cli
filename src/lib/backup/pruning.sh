#!/usr/bin/env bash
# pruning.sh - Advanced backup pruning and retention policies
# Part of nself backup system


# Source required utilities
source "${SCRIPT_DIR:-$(dirname "${BASH_SOURCE[0]}")/..}/lib/utils/display.sh" 2>/dev/null || true

set -euo pipefail


# Retention policy constants
readonly RETENTION_POLICY_AGE="age"
readonly RETENTION_POLICY_COUNT="count"
readonly RETENTION_POLICY_SIZE="size"
readonly RETENTION_POLICY_GFS="gfs"
readonly RETENTION_POLICY_321="3-2-1"
readonly RETENTION_POLICY_SMART="smart"

# Default retention values
readonly DEFAULT_RETENTION_DAYS=30
readonly DEFAULT_RETENTION_COUNT=10
readonly DEFAULT_MAX_SIZE_GB=50
readonly DEFAULT_MIN_BACKUPS=3

# Get backup size in bytes
get_backup_size() {
  local file="$1"
  if [[ -f "$file" ]]; then
    stat -f%z "$file" 2>/dev/null || stat -c%s "$file" 2>/dev/null || echo 0
  else
    echo 0
  fi
}

# Get backup age in days
get_backup_age_days() {
  local file="$1"
  local now=$(date +%s)
  local mtime=$(stat -f %m "$file" 2>/dev/null || stat -c %Y "$file" 2>/dev/null || echo 0)
  echo $(((now - mtime) / 86400))
}

# Calculate total backup size in GB
calculate_total_backup_size() {
  local backup_dir="${1:-./backups}"
  local total_bytes=0

  if [[ -d "$backup_dir" ]]; then
    while IFS= read -r file; do
      if [[ -f "$file" ]]; then
        local size=$(get_backup_size "$file")
        total_bytes=$((total_bytes + size))
      fi
    done < <(find "$backup_dir" -type f -name "*.tar.gz" 2>/dev/null)
  fi

  # Convert to GB
  echo "scale=2; $total_bytes / 1073741824" | bc
}

# Age-based retention policy
prune_by_age() {
  local backup_dir="${1:-./backups}"
  local retention_days="${2:-$DEFAULT_RETENTION_DAYS}"
  local min_backups="${3:-$DEFAULT_MIN_BACKUPS}"

  log_info "Age-based pruning: Keeping backups newer than $retention_days days"
  log_info "Minimum backups to retain: $min_backups"
  echo ""

  local removed=0
  local kept=0
  local freed_bytes=0
  local total_backups=$(find "$backup_dir" -type f -name "*.tar.gz" 2>/dev/null | wc -l | tr -d ' ')

  # Find old backups
  while IFS= read -r backup; do
    if [[ -f "$backup" ]]; then
      local name=$(basename "$backup")
      local age=$(get_backup_age_days "$backup")

      # Keep minimum number of backups regardless of age
      if [[ $((total_backups - removed)) -le $min_backups ]]; then
        log_info "  ✓ Keeping: $name (minimum retention: $min_backups backups)"
        kept=$((kept + 1))
      elif [[ $age -gt $retention_days ]]; then
        local size=$(get_backup_size "$backup")
        log_info "  ✗ Removing: $name (age: ${age}d > ${retention_days}d)"
        rm -f "$backup"
        removed=$((removed + 1))
        freed_bytes=$((freed_bytes + size))
      else
        log_info "  ✓ Keeping: $name (age: ${age}d)"
        kept=$((kept + 1))
      fi
    fi
  done < <(find "$backup_dir" -type f -name "*.tar.gz" | sort)

  # Summary
  echo ""
  if [[ $removed -gt 0 ]]; then
    local freed_mb=$((freed_bytes / 1048576))
    log_success "Removed $removed backup(s), freed ${freed_mb}MB"
  else
    log_info "No backups to remove"
  fi
  log_info "Kept $kept backup(s)"
  echo ""
}

# Count-based retention policy
prune_by_count() {
  local backup_dir="${1:-./backups}"
  local max_count="${2:-$DEFAULT_RETENTION_COUNT}"

  log_info "Count-based pruning: Keeping last $max_count backups"
  echo ""

  local removed=0
  local kept=0
  local freed_bytes=0

  # Get all backups sorted by date (newest first)
  local backups=($(find "$backup_dir" -type f -name "*.tar.gz" -print0 2>/dev/null | xargs -0 ls -t 2>/dev/null))
  local total=${#backups[@]}

  for i in "${!backups[@]}"; do
    local backup="${backups[$i]}"
    local name=$(basename "$backup")

    if [[ $i -lt $max_count ]]; then
      log_info "  ✓ Keeping: $name (position: $((i + 1))/$max_count)"
      kept=$((kept + 1))
    else
      local size=$(get_backup_size "$backup")
      log_info "  ✗ Removing: $name (exceeds limit: $max_count)"
      rm -f "$backup"
      removed=$((removed + 1))
      freed_bytes=$((freed_bytes + size))
    fi
  done

  # Summary
  echo ""
  if [[ $removed -gt 0 ]]; then
    local freed_mb=$((freed_bytes / 1048576))
    log_success "Removed $removed backup(s), freed ${freed_mb}MB"
  else
    log_info "No backups to remove"
  fi
  log_info "Kept $kept backup(s)"
  echo ""
}

# Size-based retention policy
prune_by_size() {
  local backup_dir="${1:-./backups}"
  local max_size_gb="${2:-$DEFAULT_MAX_SIZE_GB}"
  local min_backups="${3:-$DEFAULT_MIN_BACKUPS}"

  log_info "Size-based pruning: Keeping total size under ${max_size_gb}GB"
  log_info "Minimum backups to retain: $min_backups"
  echo ""

  local max_size_bytes=$((max_size_gb * 1073741824))
  local current_size_bytes=0
  local removed=0
  local kept=0
  local freed_bytes=0

  # Get all backups sorted by date (newest first)
  local backups=($(find "$backup_dir" -type f -name "*.tar.gz" -print0 2>/dev/null | xargs -0 ls -t 2>/dev/null))
  local total=${#backups[@]}

  for i in "${!backups[@]}"; do
    local backup="${backups[$i]}"
    local name=$(basename "$backup")
    local size=$(get_backup_size "$backup")

    # Always keep minimum number of backups
    if [[ $kept -lt $min_backups ]]; then
      log_info "  ✓ Keeping: $name (minimum retention)"
      kept=$((kept + 1))
      current_size_bytes=$((current_size_bytes + size))
    # Keep if under size limit
    elif [[ $((current_size_bytes + size)) -le $max_size_bytes ]]; then
      local size_mb=$((size / 1048576))
      log_info "  ✓ Keeping: $name (${size_mb}MB)"
      kept=$((kept + 1))
      current_size_bytes=$((current_size_bytes + size))
    # Remove if over limit
    else
      local size_mb=$((size / 1048576))
      log_info "  ✗ Removing: $name (${size_mb}MB, exceeds limit)"
      rm -f "$backup"
      removed=$((removed + 1))
      freed_bytes=$((freed_bytes + size))
    fi
  done

  # Summary
  echo ""
  local current_size_gb=$(echo "scale=2; $current_size_bytes / 1073741824" | bc)
  if [[ $removed -gt 0 ]]; then
    local freed_gb=$(echo "scale=2; $freed_bytes / 1073741824" | bc)
    log_success "Removed $removed backup(s), freed ${freed_gb}GB"
  else
    log_info "No backups to remove"
  fi
  log_info "Kept $kept backup(s), total size: ${current_size_gb}GB"
  echo ""
}

# Grandfather-Father-Son (GFS) retention
prune_gfs() {
  local backup_dir="${1:-./backups}"
  local daily="${2:-7}"
  local weekly="${3:-4}"
  local monthly="${4:-12}"

  log_info "GFS Retention: $daily daily, $weekly weekly, $monthly monthly"
  echo ""

  local now=$(date +%s)
  local day_seconds=86400
  local week_seconds=604800
  local month_seconds=2592000

  local removed=0
  local kept_daily=0
  local kept_weekly=0
  local kept_monthly=0
  local freed_bytes=0

  # Track which backups to keep
  declare -a keep_backups=()

  # Get all backups sorted by date (newest first)
  local backups=($(find "$backup_dir" -type f -name "*.tar.gz" -print0 2>/dev/null | xargs -0 ls -t 2>/dev/null))

  # Categorize backups
  for backup in "${backups[@]}"; do
    local mtime=$(stat -f %m "$backup" 2>/dev/null || stat -c %Y "$backup" 2>/dev/null || echo 0)
    local age_seconds=$((now - mtime))
    local age_days=$((age_seconds / day_seconds))
    local age_weeks=$((age_seconds / week_seconds))
    local age_months=$((age_seconds / month_seconds))

    local name=$(basename "$backup")
    local keep=false
    local reason=""

    # Daily backups (last N days)
    if [[ $age_days -lt $daily ]] && [[ $kept_daily -lt $daily ]]; then
      keep=true
      reason="daily"
      kept_daily=$((kept_daily + 1))
    # Weekly backups
    elif [[ $age_weeks -lt $weekly ]] && [[ $((age_days % 7)) -eq 0 ]] && [[ $kept_weekly -lt $weekly ]]; then
      keep=true
      reason="weekly"
      kept_weekly=$((kept_weekly + 1))
    # Monthly backups
    elif [[ $age_months -lt $monthly ]] && [[ $((age_days % 30)) -eq 0 ]] && [[ $kept_monthly -lt $monthly ]]; then
      keep=true
      reason="monthly"
      kept_monthly=$((kept_monthly + 1))
    fi

    if [[ "$keep" == true ]]; then
      log_info "  ✓ Keeping: $name ($reason, age: ${age_days}d)"
      keep_backups+=("$backup")
    else
      local size=$(get_backup_size "$backup")
      log_info "  ✗ Removing: $name (age: ${age_days}d)"
      rm -f "$backup"
      removed=$((removed + 1))
      freed_bytes=$((freed_bytes + size))
    fi
  done

  # Summary
  echo ""
  local total_kept=$((kept_daily + kept_weekly + kept_monthly))
  if [[ $removed -gt 0 ]]; then
    local freed_mb=$((freed_bytes / 1048576))
    log_success "Removed $removed backup(s), freed ${freed_mb}MB"
  else
    log_info "No backups to remove"
  fi
  log_info "Kept $total_kept backup(s): $kept_daily daily, $kept_weekly weekly, $kept_monthly monthly"
  echo ""
}

# 3-2-1 backup rule enforcement
# 3 copies, 2 different media, 1 offsite
check_321_rule() {
  local backup_dir="${1:-./backups}"
  local cloud_provider="${BACKUP_CLOUD_PROVIDER:-}"

  log_info "Checking 3-2-1 backup rule compliance"
  echo ""

  # Count local backups
  local local_count=$(find "$backup_dir" -type f -name "*.tar.gz" 2>/dev/null | wc -l | tr -d ' ')

  # Check for cloud backups
  local cloud_count=0
  local has_cloud=false

  if [[ -n "$cloud_provider" ]]; then
    has_cloud=true
    log_info "Cloud provider configured: $cloud_provider"

    case "$cloud_provider" in
      s3)
        if command -v aws >/dev/null 2>&1 && [[ -n "${S3_BUCKET:-}" ]]; then
          cloud_count=$(aws s3 ls "s3://$S3_BUCKET/nself-backups/" 2>/dev/null | wc -l | tr -d ' ')
        fi
        ;;
      rclone)
        if command -v rclone >/dev/null 2>&1 && [[ -n "${RCLONE_REMOTE:-}" ]]; then
          cloud_count=$(rclone ls "${RCLONE_REMOTE}:${RCLONE_PATH:-nself-backups}" 2>/dev/null | wc -l | tr -d ' ')
        fi
        ;;
    esac
  fi

  # Assess compliance
  local total_copies=$((local_count > 0 ? 1 : 0))
  total_copies=$((total_copies + (cloud_count > 0 ? 1 : 0)))

  local different_media=$((local_count > 0 ? 1 : 0))
  different_media=$((different_media + (cloud_count > 0 ? 1 : 0)))

  local offsite=$((cloud_count > 0 ? 1 : 0))

  echo "3-2-1 Rule Status:"
  echo "  • 3 copies of data:"
  echo "    - Local backups: $local_count"
  echo "    - Cloud backups: $cloud_count"
  printf "    Status: "
  if [[ $total_copies -ge 2 ]]; then
    log_success "✓ (at least 2 locations)"
  else
    log_error "✗ (need backups in multiple locations)"
  fi

  echo "  • 2 different media types:"
  printf "    Status: "
  if [[ $different_media -ge 2 ]]; then
    log_success "✓ (local storage + cloud)"
  else
    log_warning "⚠ (consider adding cloud backup)"
  fi

  echo "  • 1 offsite copy:"
  printf "    Status: "
  if [[ $offsite -ge 1 ]]; then
    log_success "✓ (cloud backup configured)"
  else
    log_error "✗ (no offsite backup - run 'nself backup cloud setup')"
  fi

  echo ""
}

# Clean failed/partial backups
clean_failed_backups() {
  local backup_dir="${1:-./backups}"

  log_info "Cleaning failed and partial backups"
  echo ""

  local removed=0
  local freed_bytes=0

  # Find backups smaller than 1KB (likely failed)
  while IFS= read -r backup; do
    if [[ -f "$backup" ]]; then
      local size=$(get_backup_size "$backup")
      local name=$(basename "$backup")

      # Remove if smaller than 1KB
      if [[ $size -lt 1024 ]]; then
        log_info "  ✗ Removing failed backup: $name (${size}B)"
        rm -f "$backup"
        removed=$((removed + 1))
        freed_bytes=$((freed_bytes + size))
      fi
    fi
  done < <(find "$backup_dir" -type f -name "*.tar.gz" 2>/dev/null)

  # Find .partial files
  while IFS= read -r partial; do
    if [[ -f "$partial" ]]; then
      local size=$(get_backup_size "$partial")
      local name=$(basename "$partial")
      log_info "  ✗ Removing partial backup: $name"
      rm -f "$partial"
      removed=$((removed + 1))
      freed_bytes=$((freed_bytes + size))
    fi
  done < <(find "$backup_dir" -type f -name "*.partial" 2>/dev/null)

  # Summary
  echo ""
  if [[ $removed -gt 0 ]]; then
    local freed_kb=$((freed_bytes / 1024))
    log_success "Removed $removed failed/partial backup(s), freed ${freed_kb}KB"
  else
    log_info "No failed or partial backups found"
  fi
  echo ""
}

# Verify backup integrity
verify_backup_integrity() {
  local backup_file="$1"
  local verbose="${2:-false}"

  if [[ ! -f "$backup_file" ]]; then
    log_error "Backup file not found: $backup_file"
    return 1
  fi

  local name=$(basename "$backup_file")

  if [[ "$verbose" == "true" ]]; then
    log_info "Verifying backup: $name"
  fi

  # Check if file is readable
  if [[ ! -r "$backup_file" ]]; then
    log_error "  ✗ Cannot read file"
    return 1
  fi

  # Check file size
  local size=$(get_backup_size "$backup_file")
  if [[ $size -lt 1024 ]]; then
    log_error "  ✗ File too small (${size}B) - likely corrupted"
    return 1
  fi

  # Verify tar.gz integrity
  if [[ "$backup_file" == *.tar.gz ]]; then
    if ! tar -tzf "$backup_file" >/dev/null 2>&1; then
      log_error "  ✗ Archive corrupted (tar verification failed)"
      return 1
    fi
  fi

  # Calculate checksum if requested
  if [[ "$verbose" == "true" ]]; then
    local checksum=$(sha256sum "$backup_file" 2>/dev/null | cut -d' ' -f1 || shasum -a 256 "$backup_file" 2>/dev/null | cut -d' ' -f1)
    local size_mb=$((size / 1048576))
    log_success "  ✓ Valid ($size_mb MB, SHA256: ${checksum:0:16}...)"
  fi

  return 0
}

# Verify all backups
verify_all_backups() {
  local backup_dir="${1:-./backups}"

  log_info "Verifying all backups in $backup_dir"
  echo ""

  local valid=0
  local invalid=0
  local total=0

  while IFS= read -r backup; do
    if [[ -f "$backup" ]]; then
      total=$((total + 1))
      if verify_backup_integrity "$backup" true; then
        valid=$((valid + 1))
      else
        invalid=$((invalid + 1))
      fi
    fi
  done < <(find "$backup_dir" -type f -name "*.tar.gz" 2>/dev/null)

  # Summary
  echo ""
  if [[ $total -eq 0 ]]; then
    log_warning "No backups found"
  elif [[ $invalid -eq 0 ]]; then
    log_success "All $valid backup(s) verified successfully"
  else
    log_warning "Verified: $valid valid, $invalid corrupted backup(s)"
    log_info "Run 'nself backup clean' to remove corrupted backups"
  fi
  echo ""
}

# Export functions
export -f get_backup_size get_backup_age_days calculate_total_backup_size
export -f prune_by_age prune_by_count prune_by_size prune_gfs
export -f check_321_rule clean_failed_backups
export -f verify_backup_integrity verify_all_backups
