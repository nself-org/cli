#!/usr/bin/env bash
# scanner.sh - Security scanning functions
# Part of nself v0.6.0 - Sprint 17: Advanced Security


# ============================================================================
# Password Security Scanner
# ============================================================================

# Check if a password meets complexity requirements
check_password_strength() {

set -euo pipefail

  local password="$1"
  local min_length="${2:-12}"
  local require_uppercase="${3:-true}"
  local require_lowercase="${4:-true}"
  local require_digit="${5:-true}"
  local require_special="${6:-true}"

  local score=0
  local issues=()

  # Length check
  if [[ ${#password} -ge $min_length ]]; then
    ((score += 25))
  else
    issues+=("Password too short (minimum: $min_length characters)")
  fi

  # Uppercase check
  if $require_uppercase; then
    if [[ "$password" =~ [A-Z] ]]; then
      ((score += 20))
    else
      issues+=("Missing uppercase letter")
    fi
  fi

  # Lowercase check
  if $require_lowercase; then
    if [[ "$password" =~ [a-z] ]]; then
      ((score += 20))
    else
      issues+=("Missing lowercase letter")
    fi
  fi

  # Digit check
  if $require_digit; then
    if [[ "$password" =~ [0-9] ]]; then
      ((score += 20))
    else
      issues+=("Missing digit")
    fi
  fi

  # Special character check
  if $require_special; then
    if [[ "$password" =~ [^A-Za-z0-9] ]]; then
      ((score += 15))
    else
      issues+=("Missing special character")
    fi
  fi

  # Output result
  echo "{\"score\": $score, \"issues\": $(printf '%s\n' "${issues[@]}" | jq -R . | jq -s .)}"
}

# Check if password is in common password list
is_common_password() {
  local password="$1"

  # Common passwords to check against (top 100 most common)
  local common_passwords=(
    "password" "123456" "password123" "12345678" "qwerty"
    "abc123" "monkey" "1234567" "letmein" "trustno1"
    "dragon" "baseball" "111111" "iloveyou" "master"
    "sunshine" "ashley" "bailey" "passw0rd" "shadow"
    "123123" "654321" "superman" "qazwsx" "michael"
    "football" "welcome" "admin" "user" "root"
  )

  local lower_password
  lower_password=$(echo "$password" | tr '[:upper:]' '[:lower:]')

  for common in "${common_passwords[@]}"; do
    if [[ "$lower_password" == "$common" ]]; then
      return 0 # Is common
    fi
  done

  return 1 # Not common
}

# ============================================================================
# Session Security Scanner
# ============================================================================

# Detect session anomalies
detect_session_anomalies() {
  local user_id="$1"
  local session_id="$2"
  local ip_address="$3"
  local user_agent="$4"

  local anomalies=()
  local risk_score=0

  # Check for impossible travel (user in multiple locations too quickly)
  # This would require geolocation data - placeholder for now
  # anomalies+=("impossible_travel")

  # Check for unusual user agent switches
  # This would require comparing with user's typical user agents
  # anomalies+=("user_agent_change")

  # Check for session hijacking indicators
  # - Sudden change in IP address
  # - Unusual session duration
  # - Abnormal request patterns

  # Output result
  echo "{\"anomalies\": $(printf '%s\n' "${anomalies[@]}" | jq -R . | jq -s .), \"risk_score\": $risk_score}"
}

# ============================================================================
# Device Fingerprinting
# ============================================================================

# Generate device fingerprint from user agent and other characteristics
generate_device_fingerprint() {
  local user_agent="$1"
  local ip_address="$2"
  local additional_data="${3:-}"

  # Create a hash of device characteristics
  local fingerprint_data="${user_agent}|${ip_address}|${additional_data}"
  local fingerprint

  if command -v openssl >/dev/null 2>&1; then
    fingerprint=$(printf "%s" "$fingerprint_data" | openssl dgst -sha256 | awk '{print $2}')
  else
    # Fallback to simple hash
    fingerprint=$(printf "%s" "$fingerprint_data" | cksum | awk '{print $1}')
  fi

  echo "$fingerprint"
}

# Parse user agent to extract device information
parse_user_agent() {
  local user_agent="$1"

  local os="unknown"
  local os_version="unknown"
  local browser="unknown"
  local browser_version="unknown"
  local device_type="desktop"

  # Detect OS
  if [[ "$user_agent" =~ Windows ]]; then
    os="Windows"
    if [[ "$user_agent" =~ "Windows NT 10" ]]; then
      os_version="10"
    elif [[ "$user_agent" =~ "Windows NT 11" ]]; then
      os_version="11"
    fi
  elif [[ "$user_agent" =~ "Mac OS X" ]]; then
    os="macOS"
    if [[ "$user_agent" =~ "Mac OS X ([0-9_]+)" ]]; then
      os_version="${BASH_REMATCH[1]//_/.}"
    fi
  elif [[ "$user_agent" =~ Linux ]]; then
    os="Linux"
  elif [[ "$user_agent" =~ iPhone ]]; then
    os="iOS"
    device_type="mobile"
  elif [[ "$user_agent" =~ Android ]]; then
    os="Android"
    device_type="mobile"
  fi

  # Detect browser
  if [[ "$user_agent" =~ Chrome/([0-9]+) ]]; then
    browser="Chrome"
    browser_version="${BASH_REMATCH[1]}"
  elif [[ "$user_agent" =~ Firefox/([0-9]+) ]]; then
    browser="Firefox"
    browser_version="${BASH_REMATCH[1]}"
  elif [[ "$user_agent" =~ Safari/([0-9]+) ]] && [[ ! "$user_agent" =~ Chrome ]]; then
    browser="Safari"
    browser_version="${BASH_REMATCH[1]}"
  elif [[ "$user_agent" =~ Edge/([0-9]+) ]]; then
    browser="Edge"
    browser_version="${BASH_REMATCH[1]}"
  fi

  # Detect device type
  if [[ "$user_agent" =~ Mobile ]]; then
    device_type="mobile"
  elif [[ "$user_agent" =~ Tablet ]]; then
    device_type="tablet"
  fi

  echo "{
    \"os\": \"$os\",
    \"os_version\": \"$os_version\",
    \"browser\": \"$browser\",
    \"browser_version\": \"$browser_version\",
    \"device_type\": \"$device_type\"
  }"
}

# ============================================================================
# Suspicious Activity Detection
# ============================================================================

# Detect brute force attempts
detect_brute_force() {
  local user_id="$1"
  local time_window="${2:-300}" # 5 minutes default
  local threshold="${3:-5}"     # 5 attempts default

  # This would query the database for failed login attempts
  # Placeholder implementation
  local failed_attempts=0

  if [[ $failed_attempts -ge $threshold ]]; then
    echo '{"detected": true, "attempts": '$failed_attempts', "recommended_action": "lock_account"}'
  else
    echo '{"detected": false, "attempts": '$failed_attempts'}'
  fi
}

# Detect credential stuffing
detect_credential_stuffing() {
  local ip_address="$1"
  local time_window="${2:-3600}" # 1 hour default
  local threshold="${3:-10}"     # 10 different users from same IP

  # This would query the database for login attempts from the same IP
  # across multiple user accounts
  # Placeholder implementation
  local unique_users=0

  if [[ $unique_users -ge $threshold ]]; then
    echo '{"detected": true, "users_attempted": '$unique_users', "recommended_action": "block_ip"}'
  else
    echo '{"detected": false, "users_attempted": '$unique_users'}'
  fi
}

# Detect account takeover indicators
detect_account_takeover() {
  local user_id="$1"
  local session_data="$2"

  local indicators=()
  local risk_score=0

  # Check for suspicious changes
  # - Password changed and email changed in quick succession
  # - Login from new location after password change
  # - Unusual activity pattern

  # Check for rapid succession of changes
  # indicators+=("rapid_changes")
  # risk_score+=30

  # Check for new location
  # indicators+=("new_location")
  # risk_score+=20

  # Output result
  echo "{\"indicators\": $(printf '%s\n' "${indicators[@]}" | jq -R . | jq -s .), \"risk_score\": $risk_score}"
}

# ============================================================================
# Risk Scoring
# ============================================================================

# Calculate overall security risk score for a user
calculate_user_risk_score() {
  local user_id="$1"

  local risk_score=0

  # Factor 1: Password age (0-20 points)
  # Older passwords = higher risk

  # Factor 2: MFA status (0-30 points)
  # No MFA = +30 points

  # Factor 3: Recent suspicious activity (0-25 points)
  # Recent incidents = higher risk

  # Factor 4: Device trust (0-15 points)
  # Untrusted devices = higher risk

  # Factor 5: Session security (0-10 points)
  # Long sessions, multiple concurrent = higher risk

  echo "$risk_score"
}

# ============================================================================
# Vulnerability Scanning
# ============================================================================

# Scan for SQL injection vulnerabilities in user input
scan_sql_injection() {
  local input="$1"

  local sql_patterns=(
    ".*(\"|').*OR.*(\"|').*=.*(\"|').*"
    ".*DROP.*TABLE.*"
    ".*UNION.*SELECT.*"
    ".*;.*--"
    ".*'.*OR.*'1'.*=.*'1"
  )

  for pattern in "${sql_patterns[@]}"; do
    if [[ "$input" =~ $pattern ]]; then
      echo '{"vulnerable": true, "pattern": "sql_injection"}'
      return 0
    fi
  done

  echo '{"vulnerable": false}'
  return 0
}

# Scan for XSS vulnerabilities
scan_xss() {
  local input="$1"

  local xss_patterns=(
    ".*<script.*>.*"
    ".*javascript:.*"
    ".*onerror=.*"
    ".*onload=.*"
    ".*<iframe.*"
  )

  for pattern in "${xss_patterns[@]}"; do
    if [[ "$input" =~ $pattern ]]; then
      echo '{"vulnerable": true, "pattern": "xss"}'
      return 0
    fi
  done

  echo '{"vulnerable": false}'
  return 0
}

# ============================================================================
# Security Recommendations
# ============================================================================

# Generate security recommendations based on scan results
generate_security_recommendations() {
  local scan_results="$1"

  local recommendations=()

  # Parse scan results and generate recommendations
  # This would analyze all scan results and provide actionable advice

  recommendations+=("Enable MFA for all users")
  recommendations+=("Enforce strong password policy")
  recommendations+=("Review and trust user devices")
  recommendations+=("Enable security event monitoring")
  recommendations+=("Set up automated incident response")

  printf '%s\n' "${recommendations[@]}" | jq -R . | jq -s .
}

# Export functions
export -f check_password_strength
export -f is_common_password
export -f detect_session_anomalies
export -f generate_device_fingerprint
export -f parse_user_agent
export -f detect_brute_force
export -f detect_credential_stuffing
export -f detect_account_takeover
export -f calculate_user_risk_score
export -f scan_sql_injection
export -f scan_xss
export -f generate_security_recommendations
