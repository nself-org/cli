#!/usr/bin/env bash

# compression.sh - Smart compression with performance optimization
# Part of nself v0.9.8 - Performance Optimization
# POSIX-compliant, no Bash 4+ features

# Detect best available compression tool
get_best_compressor() {

set -euo pipefail

  # Priority: zstd (fastest) > pigz (parallel gzip) > gzip (standard)
  if command -v zstd >/dev/null 2>&1; then
    echo "zstd"
  elif command -v pigz >/dev/null 2>&1; then
    echo "pigz"
  else
    echo "gzip"
  fi
}

# Get compressor extension
get_compressor_ext() {
  local compressor="${1:-$(get_best_compressor)}"

  case "$compressor" in
    zstd) echo ".zst" ;;
    pigz|gzip) echo ".gz" ;;
    *) echo ".gz" ;;
  esac
}

# Compress file with progress
compress_file() {
  local input="$1"
  local output="${2:-${input}$(get_compressor_ext)}"
  local compressor="${3:-$(get_best_compressor)}"
  local level="${4:-3}"  # Default compression level (balanced)

  if [[ ! -f "$input" ]]; then
    echo "ERROR: Input file not found: $input" >&2
    return 1
  fi

  # Get file size for progress estimation
  local file_size=$(wc -c < "$input" 2>/dev/null || echo "0")
  local readable_size=$(numfmt --to=iec-i --suffix=B "$file_size" 2>/dev/null || echo "${file_size}B")

  printf "Compressing %s (%s)..." "$(basename "$input")" "$readable_size"

  case "$compressor" in
    zstd)
      # zstd is 2-3x faster than gzip with similar compression
      if zstd -${level} -q "$input" -o "$output" 2>/dev/null; then
        local compressed_size=$(wc -c < "$output" 2>/dev/null || echo "0")
        local ratio=$(( (file_size - compressed_size) * 100 / file_size ))
        printf " done (%d%% smaller)\n" "$ratio"
        return 0
      else
        printf " failed\n"
        return 1
      fi
      ;;
    pigz)
      # pigz uses multiple cores for faster compression
      if pigz -${level} -c "$input" > "$output" 2>/dev/null; then
        local compressed_size=$(wc -c < "$output" 2>/dev/null || echo "0")
        local ratio=$(( (file_size - compressed_size) * 100 / file_size ))
        printf " done (%d%% smaller)\n" "$ratio"
        return 0
      else
        printf " failed\n"
        return 1
      fi
      ;;
    gzip)
      # Standard gzip (single-threaded)
      if gzip -${level} -c "$input" > "$output" 2>/dev/null; then
        local compressed_size=$(wc -c < "$output" 2>/dev/null || echo "0")
        local ratio=$(( (file_size - compressed_size) * 100 / file_size ))
        printf " done (%d%% smaller)\n" "$ratio"
        return 0
      else
        printf " failed\n"
        return 1
      fi
      ;;
    *)
      echo "ERROR: Unknown compressor: $compressor" >&2
      return 1
      ;;
  esac
}

# Decompress file with auto-detection
decompress_file() {
  local input="$1"
  local output="${2:-}"

  if [[ ! -f "$input" ]]; then
    echo "ERROR: Input file not found: $input" >&2
    return 1
  fi

  # Auto-detect compression format
  local ext="${input##*.}"
  local compressor=""

  case "$ext" in
    zst)
      compressor="zstd"
      [[ -z "$output" ]] && output="${input%.zst}"
      ;;
    gz)
      # Try pigz first, fallback to gzip
      if command -v pigz >/dev/null 2>&1; then
        compressor="pigz"
      else
        compressor="gzip"
      fi
      [[ -z "$output" ]] && output="${input%.gz}"
      ;;
    *)
      echo "ERROR: Unknown compression format: $ext" >&2
      return 1
      ;;
  esac

  printf "Decompressing %s..." "$(basename "$input")"

  case "$compressor" in
    zstd)
      if zstd -d -q "$input" -o "$output" 2>/dev/null; then
        printf " done\n"
        return 0
      else
        printf " failed\n"
        return 1
      fi
      ;;
    pigz)
      if pigz -d -c "$input" > "$output" 2>/dev/null; then
        printf " done\n"
        return 0
      else
        printf " failed\n"
        return 1
      fi
      ;;
    gzip)
      if gunzip -c "$input" > "$output" 2>/dev/null; then
        printf " done\n"
        return 0
      else
        printf " failed\n"
        return 1
      fi
      ;;
    *)
      echo "ERROR: Unknown compressor: $compressor" >&2
      return 1
      ;;
  esac
}

# Parallel compression for multiple files
parallel_compress() {
  local files=("$@")
  local compressor=$(get_best_compressor)
  local max_jobs=4

  printf "Compressing %d files in parallel (using %s)...\n" "${#files[@]}" "$compressor"

  local pids=()
  local job_count=0

  for file in "${files[@]}"; do
    # Wait if we've reached max parallel jobs
    if [[ $job_count -ge $max_jobs ]]; then
      wait -n 2>/dev/null || wait
      job_count=$(( job_count - 1 ))
    fi

    # Start compression in background
    (compress_file "$file" "${file}$(get_compressor_ext)" "$compressor" 3) &
    pids+=($!)
    job_count=$(( job_count + 1 ))
  done

  # Wait for all remaining jobs
  for pid in "${pids[@]}"; do
    wait "$pid" 2>/dev/null || true
  done

  printf "All compressions complete\n"
}

# Get compression info
compression_info() {
  printf "Available compression tools:\n"

  if command -v zstd >/dev/null 2>&1; then
    local version=$(zstd --version 2>&1 | head -1)
    printf "  ✓ zstd    - %s (fastest, recommended)\n" "$version"
  else
    printf "  ✗ zstd    - Not installed (install for 2-3x faster compression)\n"
  fi

  if command -v pigz >/dev/null 2>&1; then
    local version=$(pigz --version 2>&1 | head -1 | awk '{print $2}')
    printf "  ✓ pigz    - v%s (parallel gzip)\n" "$version"
  else
    printf "  ✗ pigz    - Not installed (install for faster gzip)\n"
  fi

  if command -v gzip >/dev/null 2>&1; then
    printf "  ✓ gzip    - Available (standard)\n"
  fi

  printf "\nCurrent compressor: %s\n" "$(get_best_compressor)"
}

# Export functions
export -f get_best_compressor
export -f get_compressor_ext
export -f compress_file
export -f decompress_file
export -f parallel_compress
export -f compression_info
