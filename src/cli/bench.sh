#!/usr/bin/env bash

# bench.sh - Performance benchmarking and load testing
# v0.4.6 - Part of the Scaling & Performance release

set -euo pipefail

# Source shared utilities
CLI_SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"
SCRIPT_DIR="$CLI_SCRIPT_DIR"
source "$CLI_SCRIPT_DIR/../lib/utils/env.sh"
source "$CLI_SCRIPT_DIR/../lib/utils/display.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/utils/header.sh"
source "$CLI_SCRIPT_DIR/../lib/hooks/pre-command.sh"
source "$CLI_SCRIPT_DIR/../lib/hooks/post-command.sh"

# Color fallbacks
: "${COLOR_GREEN:=\033[0;32m}"
: "${COLOR_YELLOW:=\033[0;33m}"
: "${COLOR_RED:=\033[0;31m}"
: "${COLOR_CYAN:=\033[0;36m}"
: "${COLOR_RESET:=\033[0m}"
: "${COLOR_DIM:=\033[2m}"
: "${COLOR_BOLD:=\033[1m}"

# Show help
show_bench_help() {
  cat <<'EOF'
nself bench - Performance benchmarking and load testing

Usage: nself bench <subcommand> [options]

Subcommands:
  run [target]          Run benchmark against target (API, database, etc.)
  baseline              Establish performance baseline
  compare [file]        Compare current performance against baseline
  stress [target]       Run stress test (high load)
  report                Generate benchmark report

Options:
  --requests N          Number of requests to send (default: 1000)
  --concurrency N       Concurrent connections (default: 10)
  --duration N          Test duration in seconds (default: 30)
  --rate N              Requests per second limit (0 = unlimited)
  --warmup N            Warmup period in seconds (default: 5)
  --output FILE         Save results to file
  --json                Output in JSON format
  -h, --help            Show this help message

Targets:
  api                   Hasura GraphQL API
  auth                  Authentication service
  db                    Database (PostgreSQL)
  functions             Serverless functions
  custom <url>          Custom endpoint

Examples:
  nself bench run api                 # Benchmark GraphQL API
  nself bench run api --requests 5000 # 5000 requests
  nself bench baseline                # Establish baseline
  nself bench compare baseline.json   # Compare to baseline
  nself bench stress api --duration 60 # 60 second stress test
  nself bench report --json           # JSON report
EOF
}

# Initialize benchmark environment
init_bench() {
  load_env_with_priority

  # Benchmark configuration
  BENCH_REQUESTS="${BENCH_REQUESTS:-1000}"
  BENCH_CONCURRENCY="${BENCH_CONCURRENCY:-10}"
  BENCH_DURATION="${BENCH_DURATION:-30}"
  BENCH_RATE="${BENCH_RATE:-0}"
  BENCH_WARMUP="${BENCH_WARMUP:-5}"

  # Directory setup
  BENCH_DIR="${BENCH_DIR:-.nself/benchmarks}"
  mkdir -p "$BENCH_DIR"

  local project_name="${PROJECT_NAME:-nself}"
  local base_domain="${BASE_DOMAIN:-local.nself.org}"

  # Define targets
  BENCH_TARGETS=(
    "api:https://api.${base_domain}/v1/graphql"
    "auth:https://auth.${base_domain}/healthz"
    "functions:https://functions.${base_domain}/v1/health"
  )
}

# Check if benchmarking tools are available
check_bench_tools() {
  local has_tools=false

  if command -v ab >/dev/null 2>&1; then
    BENCH_TOOL="ab"
    has_tools=true
  elif command -v wrk >/dev/null 2>&1; then
    BENCH_TOOL="wrk"
    has_tools=true
  elif command -v hey >/dev/null 2>&1; then
    BENCH_TOOL="hey"
    has_tools=true
  elif command -v curl >/dev/null 2>&1; then
    BENCH_TOOL="curl"
    has_tools=true
  fi

  if [[ "$has_tools" != "true" ]]; then
    log_error "No benchmarking tools found"
    log_info "Install one of: ab (Apache Bench), wrk, hey"
    log_info "  macOS: brew install wrk"
    log_info "  Ubuntu: apt-get install apache2-utils"
    return 1
  fi

  return 0
}

# Get target URL
get_target_url() {
  local target="$1"

  for entry in "${BENCH_TARGETS[@]}"; do
    local name="${entry%%:*}"
    local url="${entry#*:}"
    if [[ "$name" == "$target" ]]; then
      echo "$url"
      return 0
    fi
  done

  # Custom URL
  if [[ "$target" =~ ^https?:// ]]; then
    echo "$target"
    return 0
  fi

  return 1
}

# Run benchmark with curl (fallback)
bench_with_curl() {
  local url="$1"
  local requests="$2"
  local concurrency="$3"

  local total_time=0
  local min_time=999999
  local max_time=0
  local success=0
  local failed=0

  printf "${COLOR_CYAN}➞ Running curl benchmark${COLOR_RESET}\n"
  printf "  URL: %s\n" "$url"
  printf "  Requests: %s\n" "$requests"
  echo ""

  local start_time=$(date +%s)

  for ((i = 1; i <= requests; i++)); do
    local req_start=$(date +%s%N)
    local status=$(curl -s -o /dev/null -w "%{http_code}" --max-time 30 "$url" 2>/dev/null || echo "000")
    local req_end=$(date +%s%N)

    local req_time=$(((req_end - req_start) / 1000000)) # Convert to ms
    total_time=$((total_time + req_time))

    if [[ "$req_time" -lt "$min_time" ]]; then
      min_time=$req_time
    fi
    if [[ "$req_time" -gt "$max_time" ]]; then
      max_time=$req_time
    fi

    if [[ "$status" =~ ^[23] ]]; then
      success=$((success + 1))
    else
      failed=$((failed + 1))
    fi

    # Progress
    if [[ $((i % 100)) -eq 0 ]]; then
      printf "  Progress: %d/%d\r" "$i" "$requests"
    fi
  done

  local end_time=$(date +%s)
  local elapsed=$((end_time - start_time))
  local avg_time=$((total_time / requests))
  local rps=$((requests / (elapsed > 0 ? elapsed : 1)))

  echo ""
  printf "${COLOR_CYAN}➞ Results${COLOR_RESET}\n"
  echo ""
  printf "  %-20s %s\n" "Total requests:" "$requests"
  printf "  %-20s %s\n" "Successful:" "$success"
  printf "  %-20s %s\n" "Failed:" "$failed"
  printf "  %-20s %s seconds\n" "Total time:" "$elapsed"
  printf "  %-20s %s req/sec\n" "Requests/sec:" "$rps"
  printf "  %-20s %s ms\n" "Avg response:" "$avg_time"
  printf "  %-20s %s ms\n" "Min response:" "$min_time"
  printf "  %-20s %s ms\n" "Max response:" "$max_time"

  # Return JSON data
  cat <<EOF
{
  "requests": $requests,
  "successful": $success,
  "failed": $failed,
  "total_time_sec": $elapsed,
  "requests_per_sec": $rps,
  "avg_response_ms": $avg_time,
  "min_response_ms": $min_time,
  "max_response_ms": $max_time
}
EOF
}

# Run benchmark with ab (Apache Bench)
bench_with_ab() {
  local url="$1"
  local requests="$2"
  local concurrency="$3"

  printf "${COLOR_CYAN}➞ Running Apache Bench${COLOR_RESET}\n"
  printf "  URL: %s\n" "$url"
  printf "  Requests: %s\n" "$requests"
  printf "  Concurrency: %s\n" "$concurrency"
  echo ""

  ab -n "$requests" -c "$concurrency" -q "$url" 2>/dev/null || {
    log_warning "ab failed, falling back to curl"
    bench_with_curl "$url" "$requests" "$concurrency"
  }
}

# Run benchmark with wrk
bench_with_wrk() {
  local url="$1"
  local duration="$2"
  local concurrency="$3"

  printf "${COLOR_CYAN}➞ Running wrk benchmark${COLOR_RESET}\n"
  printf "  URL: %s\n" "$url"
  printf "  Duration: %ss\n" "$duration"
  printf "  Concurrency: %s\n" "$concurrency"
  echo ""

  wrk -t"$concurrency" -c"$concurrency" -d"${duration}s" "$url" 2>/dev/null || {
    log_warning "wrk failed, falling back to curl"
    bench_with_curl "$url" 100 "$concurrency"
  }
}

# Run benchmark command
cmd_run() {
  local target="${1:-api}"
  local requests="$BENCH_REQUESTS"
  local concurrency="$BENCH_CONCURRENCY"
  local duration="$BENCH_DURATION"
  local output_file="$OUTPUT_FILE"
  local json_mode="${JSON_OUTPUT:-false}"

  init_bench

  if ! check_bench_tools; then
    return 1
  fi

  local url=$(get_target_url "$target")
  if [[ -z "$url" ]]; then
    log_error "Unknown target: $target"
    log_info "Valid targets: api, auth, functions, db, custom <url>"
    return 1
  fi

  show_command_header "nself bench" "Benchmarking $target"
  echo ""

  # Warmup
  if [[ "$BENCH_WARMUP" -gt 0 ]]; then
    printf "${COLOR_DIM}Warming up (%s seconds)...${COLOR_RESET}\n" "$BENCH_WARMUP"
    for ((i = 1; i <= BENCH_WARMUP; i++)); do
      curl -s -o /dev/null "$url" 2>/dev/null || true
      sleep 1
    done
    echo ""
  fi

  # Run benchmark
  local result
  case "$BENCH_TOOL" in
    ab)
      result=$(bench_with_ab "$url" "$requests" "$concurrency")
      ;;
    wrk)
      result=$(bench_with_wrk "$url" "$duration" "$concurrency")
      ;;
    hey)
      hey -n "$requests" -c "$concurrency" "$url"
      ;;
    curl | *)
      result=$(bench_with_curl "$url" "$requests" "$concurrency")
      ;;
  esac

  # Save output
  if [[ -n "$output_file" ]]; then
    local timestamp=$(date +%Y%m%d_%H%M%S)
    local save_file="${output_file:-${BENCH_DIR}/bench_${target}_${timestamp}.json}"

    cat >"$save_file" <<EOF
{
  "timestamp": "$(date -Iseconds)",
  "target": "$target",
  "url": "$url",
  "tool": "$BENCH_TOOL",
  "config": {
    "requests": $requests,
    "concurrency": $concurrency,
    "duration": $duration,
    "warmup": $BENCH_WARMUP
  },
  "results": $result
}
EOF

    echo ""
    log_success "Results saved to: $save_file"
  fi
}

# Establish baseline
cmd_baseline() {
  local json_mode="${JSON_OUTPUT:-false}"

  init_bench

  show_command_header "nself bench" "Establishing performance baseline"
  echo ""

  local timestamp=$(date +%Y%m%d_%H%M%S)
  local baseline_file="${BENCH_DIR}/baseline_${timestamp}.json"

  printf "${COLOR_CYAN}➞ Running baseline benchmarks${COLOR_RESET}\n"
  echo ""

  local results="["
  local first=true

  for entry in "${BENCH_TARGETS[@]}"; do
    local name="${entry%%:*}"
    local url="${entry#*:}"

    # Check if service is reachable
    if curl -s -o /dev/null --max-time 5 "$url" 2>/dev/null; then
      printf "  Testing %s...\n" "$name"

      # Quick benchmark
      local start=$(date +%s%N)
      local success=0
      for ((i = 1; i <= 100; i++)); do
        if curl -s -o /dev/null --max-time 10 "$url" 2>/dev/null; then
          success=$((success + 1))
        fi
      done
      local end=$(date +%s%N)
      local total_ms=$(((end - start) / 1000000))
      local avg_ms=$((total_ms / 100))
      local rps=$((100000 / (total_ms > 0 ? total_ms : 1)))

      if [[ "$first" != "true" ]]; then
        results+=","
      fi
      first=false

      results+=$(
        cat <<EOF

    {
      "target": "$name",
      "url": "$url",
      "requests": 100,
      "success_rate": $success,
      "avg_response_ms": $avg_ms,
      "requests_per_sec": $rps
    }
EOF
      )
    else
      printf "  %s: ${COLOR_DIM}not available${COLOR_RESET}\n" "$name"
    fi
  done

  results+="
  ]"

  # Save baseline
  cat >"$baseline_file" <<EOF
{
  "type": "baseline",
  "timestamp": "$(date -Iseconds)",
  "version": "$(cat "$CLI_SCRIPT_DIR/../VERSION" 2>/dev/null || echo "unknown")",
  "targets": $results
}
EOF

  # Create symlink to latest
  ln -sf "$(basename "$baseline_file")" "${BENCH_DIR}/baseline_latest.json"

  echo ""
  log_success "Baseline established: $baseline_file"
  log_info "Use 'nself bench compare' to compare future results"

  if [[ "$json_mode" == "true" ]]; then
    cat "$baseline_file"
  fi
}

# Compare against baseline
cmd_compare() {
  local baseline_file="$1"
  local json_mode="${JSON_OUTPUT:-false}"

  init_bench

  # Default to latest baseline
  if [[ -z "$baseline_file" ]]; then
    baseline_file="${BENCH_DIR}/baseline_latest.json"
  fi

  if [[ ! -f "$baseline_file" ]]; then
    log_error "Baseline file not found: $baseline_file"
    log_info "Run 'nself bench baseline' first"
    return 1
  fi

  show_command_header "nself bench" "Comparing against baseline"
  echo ""

  printf "${COLOR_CYAN}➞ Baseline: %s${COLOR_RESET}\n" "$baseline_file"
  echo ""

  # Parse baseline (simplified - in production use jq)
  local baseline_date=$(grep '"timestamp"' "$baseline_file" | head -1 | sed 's/.*: *"\([^"]*\)".*/\1/')
  printf "  Baseline date: %s\n" "$baseline_date"
  echo ""

  printf "${COLOR_CYAN}➞ Running comparison tests${COLOR_RESET}\n"
  echo ""

  printf "  %-15s %-12s %-12s %-10s\n" "Target" "Baseline" "Current" "Change"
  printf "  %-15s %-12s %-12s %-10s\n" "------" "--------" "-------" "------"

  for entry in "${BENCH_TARGETS[@]}"; do
    local name="${entry%%:*}"
    local url="${entry#*:}"

    # Get baseline value (simplified parsing)
    local baseline_rps=$(grep -A5 "\"target\": \"$name\"" "$baseline_file" | grep "requests_per_sec" | head -1 | sed 's/[^0-9]//g')
    baseline_rps="${baseline_rps:-0}"

    if curl -s -o /dev/null --max-time 5 "$url" 2>/dev/null; then
      # Quick test
      local start=$(date +%s%N)
      for ((i = 1; i <= 50; i++)); do
        curl -s -o /dev/null --max-time 10 "$url" 2>/dev/null || true
      done
      local end=$(date +%s%N)
      local total_ms=$(((end - start) / 1000000))
      local current_rps=$((50000 / (total_ms > 0 ? total_ms : 1)))

      # Calculate change
      local change=0
      if [[ "$baseline_rps" -gt 0 ]]; then
        change=$(((current_rps - baseline_rps) * 100 / baseline_rps))
      fi

      local change_str="${change}%"
      local change_color="$COLOR_RESET"
      if [[ "$change" -gt 10 ]]; then
        change_color="$COLOR_GREEN"
        change_str="+${change}%"
      elif [[ "$change" -lt -10 ]]; then
        change_color="$COLOR_RED"
      fi

      printf "  %-15s %-12s %-12s ${change_color}%-10s${COLOR_RESET}\n" \
        "$name" "${baseline_rps} rps" "${current_rps} rps" "$change_str"
    else
      printf "  %-15s %-12s %-12s %-10s\n" "$name" "${baseline_rps} rps" "N/A" "-"
    fi
  done

  echo ""
  log_info "Legend: Green = improved (>10%), Red = degraded (<-10%)"
}

# Run stress test
cmd_stress() {
  local target="${1:-api}"
  local duration="${BENCH_DURATION:-60}"
  local concurrency="${BENCH_CONCURRENCY:-50}"

  init_bench

  local url=$(get_target_url "$target")
  if [[ -z "$url" ]]; then
    log_error "Unknown target: $target"
    return 1
  fi

  show_command_header "nself bench" "Stress testing $target"
  echo ""

  log_warning "Running high-load stress test for ${duration}s"
  log_warning "This may impact system performance"
  echo ""

  if [[ "${FORCE:-false}" != "true" ]]; then
    read -p "Continue? (y/N) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
      log_info "Stress test cancelled"
      return 1
    fi
  fi

  printf "${COLOR_CYAN}➞ Stress Test Configuration${COLOR_RESET}\n"
  printf "  Target: %s\n" "$target"
  printf "  URL: %s\n" "$url"
  printf "  Duration: %s seconds\n" "$duration"
  printf "  Concurrency: %s\n" "$concurrency"
  echo ""

  # Run stress test with increasing load
  printf "${COLOR_CYAN}➞ Phase 1: Ramp Up${COLOR_RESET}\n"
  local ramp_duration=$((duration / 3))
  local phase_concurrency=$((concurrency / 3))

  local total_requests=0
  local failed_requests=0
  local start_time=$(date +%s)

  # Simplified stress test using curl
  for ((phase = 1; phase <= 3; phase++)); do
    local current_concurrency=$((phase_concurrency * phase))
    printf "  Phase %d: %d concurrent connections\n" "$phase" "$current_concurrency"

    local phase_end=$((start_time + (ramp_duration * phase)))

    while [[ $(date +%s) -lt $phase_end ]]; do
      for ((c = 1; c <= current_concurrency; c++)); do
        (
          if curl -s -o /dev/null --max-time 10 "$url" 2>/dev/null; then
            echo "1" >>"${BENCH_DIR}/.stress_success"
          else
            echo "1" >>"${BENCH_DIR}/.stress_fail"
          fi
        ) &
      done
      wait
      total_requests=$((total_requests + current_concurrency))

      # Progress indicator
      printf "  Requests: %d\r" "$total_requests"
    done
    echo ""
  done

  local end_time=$(date +%s)
  local elapsed=$((end_time - start_time))

  # Count results
  local success_count=0
  local fail_count=0
  if [[ -f "${BENCH_DIR}/.stress_success" ]]; then
    success_count=$(wc -l <"${BENCH_DIR}/.stress_success" | tr -d ' ')
    rm -f "${BENCH_DIR}/.stress_success"
  fi
  if [[ -f "${BENCH_DIR}/.stress_fail" ]]; then
    fail_count=$(wc -l <"${BENCH_DIR}/.stress_fail" | tr -d ' ')
    rm -f "${BENCH_DIR}/.stress_fail"
  fi

  echo ""
  printf "${COLOR_CYAN}➞ Stress Test Results${COLOR_RESET}\n"
  echo ""
  printf "  %-20s %s\n" "Total requests:" "$total_requests"
  printf "  %-20s %s\n" "Successful:" "$success_count"
  printf "  %-20s %s\n" "Failed:" "$fail_count"
  printf "  %-20s %s seconds\n" "Duration:" "$elapsed"
  printf "  %-20s %s req/sec\n" "Throughput:" "$((total_requests / (elapsed > 0 ? elapsed : 1)))"

  local error_rate=0
  if [[ "$total_requests" -gt 0 ]]; then
    error_rate=$((fail_count * 100 / total_requests))
  fi

  echo ""
  if [[ "$error_rate" -lt 1 ]]; then
    log_success "System handled stress test well (error rate: ${error_rate}%)"
  elif [[ "$error_rate" -lt 5 ]]; then
    log_warning "System showed some stress (error rate: ${error_rate}%)"
  else
    log_error "System under significant stress (error rate: ${error_rate}%)"
  fi
}

# Generate benchmark report
cmd_report() {
  local json_mode="${JSON_OUTPUT:-false}"

  init_bench

  show_command_header "nself bench" "Benchmark Report"
  echo ""

  # Find all benchmark files
  local bench_files=($(ls -1 "${BENCH_DIR}/"*.json 2>/dev/null | grep -v baseline | sort -r | head -10))

  if [[ ${#bench_files[@]} -eq 0 ]]; then
    log_warning "No benchmark results found"
    log_info "Run 'nself bench run' to create benchmarks"
    return 0
  fi

  printf "${COLOR_CYAN}➞ Recent Benchmarks${COLOR_RESET}\n"
  echo ""

  printf "  %-25s %-15s %-10s %-10s\n" "Date" "Target" "Tool" "Requests"
  printf "  %-25s %-15s %-10s %-10s\n" "----" "------" "----" "--------"

  for file in "${bench_files[@]}"; do
    local ts=$(grep '"timestamp"' "$file" | head -1 | sed 's/.*: *"\([^"]*\)".*/\1/' | cut -d'T' -f1,2 | tr 'T' ' ')
    local target=$(grep '"target"' "$file" | head -1 | sed 's/.*: *"\([^"]*\)".*/\1/')
    local tool=$(grep '"tool"' "$file" | head -1 | sed 's/.*: *"\([^"]*\)".*/\1/')
    local requests=$(grep '"requests"' "$file" | head -1 | sed 's/[^0-9]//g')

    printf "  %-25s %-15s %-10s %-10s\n" "$ts" "$target" "$tool" "$requests"
  done

  echo ""

  # Latest baseline
  if [[ -f "${BENCH_DIR}/baseline_latest.json" ]]; then
    printf "${COLOR_CYAN}➞ Latest Baseline${COLOR_RESET}\n"
    local baseline_ts=$(grep '"timestamp"' "${BENCH_DIR}/baseline_latest.json" | head -1 | sed 's/.*: *"\([^"]*\)".*/\1/')
    printf "  Created: %s\n" "$baseline_ts"
    echo ""
  fi

  log_info "Use 'nself bench compare' to compare against baseline"

  if [[ "$json_mode" == "true" ]]; then
    echo ""
    printf '{"benchmarks": %d, "baseline_exists": %s}\n' \
      "${#bench_files[@]}" \
      "$([[ -f "${BENCH_DIR}/baseline_latest.json" ]] && echo "true" || echo "false")"
  fi
}

# Main command handler
cmd_bench() {
  local subcommand="${1:-}"

  # Check for help first
  if [[ "$subcommand" == "-h" ]] || [[ "$subcommand" == "--help" ]] || [[ -z "$subcommand" ]]; then
    show_bench_help
    return 0
  fi

  # Parse global options
  local args=()
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --requests)
        BENCH_REQUESTS="$2"
        shift 2
        ;;
      --concurrency)
        BENCH_CONCURRENCY="$2"
        shift 2
        ;;
      --duration)
        BENCH_DURATION="$2"
        shift 2
        ;;
      --rate)
        BENCH_RATE="$2"
        shift 2
        ;;
      --warmup)
        BENCH_WARMUP="$2"
        shift 2
        ;;
      --output)
        OUTPUT_FILE="$2"
        shift 2
        ;;
      --force)
        FORCE=true
        shift
        ;;
      --json)
        JSON_OUTPUT=true
        shift
        ;;
      -h | --help)
        show_bench_help
        return 0
        ;;
      *)
        args+=("$1")
        shift
        ;;
    esac
  done

  # Restore positional arguments
  set -- "${args[@]}"
  subcommand="${1:-}"

  case "$subcommand" in
    run)
      shift
      cmd_run "$@"
      ;;
    baseline)
      cmd_baseline
      ;;
    compare)
      shift
      cmd_compare "$@"
      ;;
    stress)
      shift
      cmd_stress "$@"
      ;;
    report)
      cmd_report
      ;;
    *)
      log_error "Unknown subcommand: $subcommand"
      show_bench_help
      return 1
      ;;
  esac
}

# Export for use as library
export -f cmd_bench

# Execute if run directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  # Help is read-only - bypass init/env guards
  for _arg in "$@"; do
    if [[ "$_arg" == "--help" ]] || [[ "$_arg" == "-h" ]]; then
      show_bench_help
      exit 0
    fi
  done
  pre_command "bench" || exit $?
  cmd_bench "$@"
  exit_code=$?
  post_command "bench" $exit_code
  exit $exit_code
fi
