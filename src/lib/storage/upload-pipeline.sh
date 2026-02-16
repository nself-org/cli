#!/usr/bin/env bash
# upload-pipeline.sh - Comprehensive file upload pipeline with multipart, thumbnails, virus scanning
# Part of nself storage management system


# Source required utilities
UPLOAD_PIPELINE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

# Check if display.sh was already sourced from parent
if [[ "${DISPLAY_SOURCED:-0}" != "1" ]]; then
  source "${UPLOAD_PIPELINE_DIR}/../utils/display.sh"
fi
# Validation.sh is optional
[[ -f "${UPLOAD_PIPELINE_DIR}/../utils/validation.sh" ]] && source "${UPLOAD_PIPELINE_DIR}/../utils/validation.sh" || true
# Platform compatibility utilities
[[ -f "${UPLOAD_PIPELINE_DIR}/../utils/platform-compat.sh" ]] && source "${UPLOAD_PIPELINE_DIR}/../utils/platform-compat.sh" || true

# Compatibility aliases for output functions
output_info() { log_info "$@"; }
output_success() { log_success "$@"; }
output_error() { log_error "$@"; }
output_warning() { log_warning "$@"; }

# Upload pipeline configuration defaults
readonly DEFAULT_CHUNK_SIZE=$((5 * 1024 * 1024))           # 5MB chunks
readonly DEFAULT_MAX_FILE_SIZE=$((5 * 1024 * 1024 * 1024)) # 5GB
readonly DEFAULT_THUMBNAIL_SIZES="150x150,300x300,600x600"
readonly DEFAULT_IMAGE_FORMATS="avif,webp,jpg"
readonly SUPPORTED_IMAGE_TYPES="image/jpeg image/png image/gif image/webp image/svg+xml"
readonly SUPPORTED_VIDEO_TYPES="video/mp4 video/webm video/ogg"
readonly SUPPORTED_DOCUMENT_TYPES="application/pdf application/msword application/vnd.openxmlformats-officedocument.wordprocessingml.document"

# Feature flags
ENABLE_MULTIPART="${UPLOAD_ENABLE_MULTIPART:-true}"
ENABLE_THUMBNAILS="${UPLOAD_ENABLE_THUMBNAILS:-false}"
ENABLE_VIRUS_SCAN="${UPLOAD_ENABLE_VIRUS_SCAN:-false}"
ENABLE_COMPRESSION="${UPLOAD_ENABLE_COMPRESSION:-true}"
ENABLE_PROGRESSIVE="${UPLOAD_ENABLE_PROGRESSIVE:-true}"

# Storage backend configuration
STORAGE_BACKEND="${STORAGE_BACKEND:-minio}"
MINIO_ENDPOINT="${MINIO_ENDPOINT:-http://minio:9000}"
MINIO_ACCESS_KEY="${MINIO_ACCESS_KEY:-minioadmin}"
MINIO_SECRET_KEY="${MINIO_SECRET_KEY:-minioadmin}"
MINIO_BUCKET="${MINIO_BUCKET:-uploads}"

# Upload pipeline state
declare -a UPLOAD_PARTS=()
UPLOAD_ID=""
CURRENT_PART=0

#######################################
# Initialize upload pipeline
# Globals:
#   None
# Arguments:
#   None
# Returns:
#   0 on success, 1 on error
#######################################
init_upload_pipeline() {
  output_info "Initializing upload pipeline..."

  # Check required tools
  local missing_tools=()

  if [[ "${ENABLE_THUMBNAILS}" == "true" ]]; then
    if ! command -v convert >/dev/null 2>&1; then
      missing_tools+=("imagemagick (for thumbnails)")
    fi
    if ! command -v ffmpeg >/dev/null 2>&1; then
      missing_tools+=("ffmpeg (for video thumbnails)")
    fi
  fi

  if [[ "${ENABLE_VIRUS_SCAN}" == "true" ]]; then
    if ! command -v clamscan >/dev/null 2>&1; then
      missing_tools+=("clamav (for virus scanning)")
    fi
  fi

  if [[ "${ENABLE_COMPRESSION}" == "true" ]]; then
    if ! command -v gzip >/dev/null 2>&1; then
      missing_tools+=("gzip (for compression)")
    fi
  fi

  if [[ ${#missing_tools[@]} -gt 0 ]]; then
    output_warning "Optional tools not available: ${missing_tools[*]}"
    output_info "Some features will be disabled"
  fi

  # Initialize MinIO client
  if ! init_minio_client; then
    output_error "Failed to initialize MinIO client"
    return 1
  fi

  output_success "Upload pipeline initialized"
  return 0
}

#######################################
# Initialize MinIO client configuration
# Globals:
#   MINIO_ENDPOINT, MINIO_ACCESS_KEY, MINIO_SECRET_KEY
# Arguments:
#   None
# Returns:
#   0 on success, 1 on error
#######################################
init_minio_client() {
  if ! command -v mc >/dev/null 2>&1; then
    output_info "Installing MinIO client..."

    local os_type
    os_type="$(uname -s | tr '[:upper:]' '[:lower:]')"

    case "${os_type}" in
      linux)
        curl -sSL https://dl.min.io/client/mc/release/linux-amd64/mc -o /tmp/mc
        ;;
      darwin)
        curl -sSL https://dl.min.io/client/mc/release/darwin-amd64/mc -o /tmp/mc
        ;;
      *)
        output_error "Unsupported OS: ${os_type}"
        return 1
        ;;
    esac

    chmod +x /tmp/mc
    sudo mv /tmp/mc /usr/local/bin/mc 2>/dev/null || mv /tmp/mc "${HOME}/.local/bin/mc"
  fi

  # Configure MinIO alias
  mc alias set nself "${MINIO_ENDPOINT}" "${MINIO_ACCESS_KEY}" "${MINIO_SECRET_KEY}" >/dev/null 2>&1 || true

  # Create bucket if it doesn't exist
  if ! mc ls "nself/${MINIO_BUCKET}" >/dev/null 2>&1; then
    mc mb "nself/${MINIO_BUCKET}" >/dev/null 2>&1 || true
  fi

  return 0
}

#######################################
# Upload file with full pipeline processing
# Globals:
#   All upload configuration variables
# Arguments:
#   $1 - File path
#   $2 - Destination path (optional)
#   $3 - Options (comma-separated: thumbnails,virus-scan,compression)
# Returns:
#   0 on success, 1 on error
#######################################
upload_file() {
  local file_path="$1"
  local dest_path="${2:-}"
  local options="${3:-}"

  # Validate file exists
  if [[ ! -f "${file_path}" ]]; then
    output_error "File not found: ${file_path}"
    return 1
  fi

  # Parse options
  if [[ -n "${options}" ]]; then
    [[ "${options}" =~ thumbnails ]] && ENABLE_THUMBNAILS=true
    [[ "${options}" =~ virus-scan ]] && ENABLE_VIRUS_SCAN=true
    [[ "${options}" =~ compression ]] && ENABLE_COMPRESSION=true
  fi

  local file_name
  local raw_filename
  raw_filename="$(basename "${file_path}")"
  file_name="$(sanitize_filename "${raw_filename}")"
  local file_size
  file_size="$(stat -f%z "${file_path}" 2>/dev/null || stat -c%s "${file_path}" 2>/dev/null)"
  local mime_type
  mime_type="$(file --mime-type -b "${file_path}")"

  # Set destination path if not provided
  if [[ -z "${dest_path}" ]]; then
    local timestamp
    timestamp="$(date +%Y/%m/%d)"
    local file_hash
    file_hash="$(md5sum "${file_path}" 2>/dev/null | cut -d' ' -f1 || md5 "${file_path}" 2>/dev/null | awk '{print $NF}')"
    # Sanitize the hash prefix (should already be safe, but be defensive)
    local safe_hash
    safe_hash="$(printf '%s' "${file_hash:0:8}" | tr -cd '0-9a-f')"
    dest_path="${timestamp}/${safe_hash}/${file_name}"
  fi

  output_info "Uploading: ${file_name} ($(numfmt --to=iec-i --suffix=B "${file_size}" 2>/dev/null || echo "${file_size} bytes"))"
  output_info "MIME type: ${mime_type}"
  output_info "Destination: ${dest_path}"

  # Virus scan
  if [[ "${ENABLE_VIRUS_SCAN}" == "true" ]]; then
    if ! scan_file_for_viruses "${file_path}"; then
      output_error "Virus detected! Upload aborted."
      return 1
    fi
  fi

  # Process based on file type
  local processed_file="${file_path}"

  # Compression for large files
  if [[ "${ENABLE_COMPRESSION}" == "true" ]] && [[ "${file_size}" -gt $((10 * 1024 * 1024)) ]]; then
    if should_compress_file "${mime_type}"; then
      processed_file="$(compress_file "${file_path}")"
      dest_path="${dest_path}.gz"
    fi
  fi

  # Multipart upload for large files
  if [[ "${ENABLE_MULTIPART}" == "true" ]] && [[ "${file_size}" -gt $((100 * 1024 * 1024)) ]]; then
    if ! multipart_upload "${processed_file}" "${dest_path}"; then
      output_error "Multipart upload failed"
      return 1
    fi
  else
    # Regular upload
    if ! single_upload "${processed_file}" "${dest_path}"; then
      output_error "Upload failed"
      return 1
    fi
  fi

  # Generate thumbnails for images and videos
  if [[ "${ENABLE_THUMBNAILS}" == "true" ]]; then
    if is_image_file "${mime_type}" || is_video_file "${mime_type}"; then
      generate_thumbnails "${file_path}" "${dest_path}"
    fi
  fi

  # Get file URL
  local file_url
  file_url="$(get_file_url "${dest_path}")"

  # Output metadata
  output_success "Upload complete!"
  printf "\nFile Details:\n"
  printf "  URL: %s\n" "${file_url}"
  printf "  Path: %s\n" "${dest_path}"
  printf "  Size: %s\n" "$(numfmt --to=iec-i --suffix=B "${file_size}" 2>/dev/null || echo "${file_size} bytes")"
  printf "  Type: %s\n" "${mime_type}"

  # Clean up temporary files
  if [[ "${processed_file}" != "${file_path}" ]]; then
    rm -f "${processed_file}"
  fi

  return 0
}

#######################################
# Single file upload
# Arguments:
#   $1 - File path
#   $2 - Destination path
# Returns:
#   0 on success, 1 on error
#######################################
single_upload() {
  local file_path="$1"
  local dest_path="$2"

  if mc cp --progress "${file_path}" "nself/${MINIO_BUCKET}/${dest_path}" 2>&1; then
    return 0
  else
    return 1
  fi
}

#######################################
# Multipart upload for large files
# Arguments:
#   $1 - File path
#   $2 - Destination path
# Returns:
#   0 on success, 1 on error
#######################################
multipart_upload() {
  local file_path="$1"
  local dest_path="$2"

  output_info "Starting multipart upload..."

  # MinIO client handles multipart automatically for large files
  if mc cp --progress "${file_path}" "nself/${MINIO_BUCKET}/${dest_path}" 2>&1; then
    output_success "Multipart upload complete"
    return 0
  else
    output_error "Multipart upload failed"
    return 1
  fi
}

#######################################
# Scan file for viruses using ClamAV
# Arguments:
#   $1 - File path
# Returns:
#   0 if clean, 1 if infected or error
#######################################
scan_file_for_viruses() {
  local file_path="$1"

  if ! command -v clamscan >/dev/null 2>&1; then
    output_warning "ClamAV not installed, skipping virus scan"
    return 0
  fi

  output_info "Scanning for viruses..."

  if clamscan --no-summary "${file_path}" >/dev/null 2>&1; then
    output_success "File is clean"
    return 0
  else
    return 1
  fi
}

#######################################
# Generate thumbnails for images and videos
# Arguments:
#   $1 - Source file path
#   $2 - Destination path
# Returns:
#   0 on success
#######################################
generate_thumbnails() {
  local file_path="$1"
  local dest_path="$2"
  local mime_type
  mime_type="$(file --mime-type -b "${file_path}")"

  output_info "Generating thumbnails..."

  # Parse thumbnail sizes
  IFS=',' read -ra sizes <<<"${DEFAULT_THUMBNAIL_SIZES}"
  IFS=',' read -ra formats <<<"${DEFAULT_IMAGE_FORMATS}"

  local base_dest="${dest_path%.*}"
  local dest_dir
  dest_dir="$(dirname "${dest_path}")/thumbnails"
  # Sanitize the destination directory to prevent traversal
  dest_dir="$(sanitize_filename "$(basename "${dest_dir}")")"

  for size in "${sizes[@]}"; do
    for format in "${formats[@]}"; do
      # Sanitize size parameter (should be "150x150" etc, but be defensive)
      local safe_size
      safe_size="$(printf '%s' "${size}" | tr -cd '0-9x')"
      # Sanitize format extension
      local safe_format
      safe_format="$(sanitize_filename "${format}")"
      local thumb_path="${dest_dir}/${safe_size}.${safe_format}"

      if is_video_file "${mime_type}"; then
        # Video thumbnail (extract first frame)
        if command -v ffmpeg >/dev/null 2>&1; then
          local temp_frame="/tmp/frame_$$.jpg"
          ffmpeg -i "${file_path}" -vframes 1 -q:v 2 "${temp_frame}" -y >/dev/null 2>&1

          if convert "${temp_frame}" -resize "${size}^" -gravity center -extent "${size}" "${format}:/tmp/thumb_$$.${format}" 2>/dev/null; then
            mc cp "/tmp/thumb_$$.${format}" "nself/${MINIO_BUCKET}/${thumb_path}" >/dev/null 2>&1
            rm -f "/tmp/thumb_$$.${format}"
          fi
          rm -f "${temp_frame}"
        fi
      elif is_image_file "${mime_type}"; then
        # Image thumbnail
        if command -v convert >/dev/null 2>&1; then
          if convert "${file_path}" -resize "${size}^" -gravity center -extent "${size}" "${format}:/tmp/thumb_$$.${format}" 2>/dev/null; then
            mc cp "/tmp/thumb_$$.${format}" "nself/${MINIO_BUCKET}/${thumb_path}" >/dev/null 2>&1
            rm -f "/tmp/thumb_$$.${format}"
          fi
        fi
      fi
    done
  done

  output_success "Thumbnails generated"
  return 0
}

#######################################
# Compress file
# Arguments:
#   $1 - File path
# Returns:
#   Compressed file path
#######################################
compress_file() {
  local file_path="$1"
  local raw_filename
  raw_filename="$(basename "${file_path}")"
  local safe_filename
  safe_filename="$(sanitize_filename "${raw_filename}")"
  local compressed_path="/tmp/${safe_filename}.gz"

  output_info "Compressing file..."

  if gzip -c "${file_path}" >"${compressed_path}"; then
    local original_size
    original_size="$(stat -f%z "${file_path}" 2>/dev/null || stat -c%s "${file_path}" 2>/dev/null)"
    local compressed_size
    compressed_size="$(stat -f%z "${compressed_path}" 2>/dev/null || stat -c%s "${compressed_path}" 2>/dev/null)"
    local ratio=$((100 - (compressed_size * 100 / original_size)))

    output_success "Compressed ${ratio}% ($(numfmt --to=iec-i --suffix=B "${compressed_size}" 2>/dev/null || echo "${compressed_size} bytes"))"
    printf "%s" "${compressed_path}"
  else
    printf "%s" "${file_path}"
  fi
}

#######################################
# Check if file should be compressed
# Arguments:
#   $1 - MIME type
# Returns:
#   0 if should compress, 1 otherwise
#######################################
should_compress_file() {
  local mime_type="$1"

  # Don't compress already compressed formats
  case "${mime_type}" in
    image/jpeg | image/png | image/gif | image/webp)
      return 1
      ;;
    video/*)
      return 1
      ;;
    application/zip | application/gzip | application/x-gzip)
      return 1
      ;;
    *)
      return 0
      ;;
  esac
}

#######################################
# Check if file is an image
# Arguments:
#   $1 - MIME type
# Returns:
#   0 if image, 1 otherwise
#######################################
is_image_file() {
  local mime_type="$1"
  [[ "${SUPPORTED_IMAGE_TYPES}" =~ ${mime_type} ]]
}

#######################################
# Check if file is a video
# Arguments:
#   $1 - MIME type
# Returns:
#   0 if video, 1 otherwise
#######################################
is_video_file() {
  local mime_type="$1"
  [[ "${SUPPORTED_VIDEO_TYPES}" =~ ${mime_type} ]]
}

#######################################
# Get public URL for uploaded file
# Arguments:
#   $1 - File path in bucket
# Returns:
#   Public URL
#######################################
get_file_url() {
  local file_path="$1"
  local storage_url="${STORAGE_PUBLIC_URL:-http://storage.localhost}"

  printf "%s/%s/%s" "${storage_url}" "${MINIO_BUCKET}" "${file_path}"
}

#######################################
# List uploaded files
# Arguments:
#   $1 - Prefix (optional)
# Returns:
#   0 on success
#######################################
list_uploads() {
  local prefix="${1:-}"

  if [[ -n "${prefix}" ]]; then
    mc ls "nself/${MINIO_BUCKET}/${prefix}"
  else
    mc ls "nself/${MINIO_BUCKET}"
  fi
}

#######################################
# Delete uploaded file
# Arguments:
#   $1 - File path
# Returns:
#   0 on success
#######################################
delete_upload() {
  local file_path="$1"

  output_info "Deleting: ${file_path}"

  if mc rm "nself/${MINIO_BUCKET}/${file_path}" 2>/dev/null; then
    output_success "File deleted"
    return 0
  else
    output_error "Failed to delete file"
    return 1
  fi
}

#######################################
# Get upload pipeline status
# Returns:
#   Status information
#######################################
get_pipeline_status() {
  printf "Upload Pipeline Status\n"
  printf "======================\n\n"

  printf "Backend: %s\n" "${STORAGE_BACKEND}"
  printf "Endpoint: %s\n" "${MINIO_ENDPOINT}"
  printf "Bucket: %s\n" "${MINIO_BUCKET}"
  printf "\nFeatures:\n"
  printf "  Multipart Upload: %s\n" "${ENABLE_MULTIPART}"
  printf "  Thumbnails: %s\n" "${ENABLE_THUMBNAILS}"
  printf "  Virus Scan: %s\n" "${ENABLE_VIRUS_SCAN}"
  printf "  Compression: %s\n" "${ENABLE_COMPRESSION}"
  printf "\nAvailable Tools:\n"

  command -v mc >/dev/null 2>&1 && printf "  MinIO Client: ✓\n" || printf "  MinIO Client: ✗\n"
  command -v convert >/dev/null 2>&1 && printf "  ImageMagick: ✓\n" || printf "  ImageMagick: ✗\n"
  command -v ffmpeg >/dev/null 2>&1 && printf "  FFmpeg: ✓\n" || printf "  FFmpeg: ✗\n"
  command -v clamscan >/dev/null 2>&1 && printf "  ClamAV: ✓\n" || printf "  ClamAV: ✗\n"

  printf "\n"
}

# Export functions
export -f init_upload_pipeline
export -f upload_file
export -f list_uploads
export -f delete_upload
export -f get_pipeline_status
