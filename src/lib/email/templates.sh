#!/usr/bin/env bash
# templates.sh - Email templates system
# Part of nself v0.6.0 - Phase 2

template_welcome() {

set -euo pipefail

  cat <<TEMPLATE
Subject: Welcome to ${PROJECT_NAME:-nself}!

Hi $1,

Welcome! Please verify your email: $2

This link expires in 24 hours.

Best regards,
The ${PROJECT_NAME:-nself} Team
TEMPLATE
}

template_password_reset() {
  cat <<TEMPLATE
Subject: Password Reset

Hi $1,

Reset your password: $2

Link expires in 1 hour.

Best regards,
The ${PROJECT_NAME:-nself} Team
TEMPLATE
}

template_mfa_code() {
  cat <<TEMPLATE
Subject: Verification Code

Your code: $1

Expires in 10 minutes.

Best regards,
The ${PROJECT_NAME:-nself} Team
TEMPLATE
}

export -f template_welcome template_password_reset template_mfa_code
