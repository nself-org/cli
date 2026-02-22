#!/usr/bin/env bash

# constants.sh - Global constants
#
# Note: No set -euo pipefail here; library files must not alter parent shell
# options when sourced. See encryption.sh comment for full rationale.
# readonly declarations would also fail on second source under set -e without
# the guard below.

# Double-source guard (MUST be before readonly declarations)
[[ -n "${CONSTANTS_SOURCED:-}" ]] && return 0
export CONSTANTS_SOURCED=1


# Exit codes
readonly EXIT_SUCCESS=0
readonly EXIT_GENERAL_ERROR=1
readonly EXIT_MISUSE=2
readonly EXIT_CANT_EXEC=126
readonly EXIT_NOT_FOUND=127

# Service names
readonly CORE_SERVICES=(postgres hasura minio auth storage)
readonly OPTIONAL_SERVICES=(redis mailhog webhook-service)

# File patterns
readonly DOCKER_FILE_PATTERN="Dockerfile*"
readonly COMPOSE_FILE_PATTERN="docker-compose*.yml"
readonly ENV_FILE_PATTERN=".env*"

# Regex patterns
readonly DOMAIN_REGEX='^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$'
readonly EMAIL_REGEX='^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$'
readonly PORT_REGEX='^[0-9]+$'

# Time constants
readonly SECOND=1
readonly MINUTE=60
readonly HOUR=3600

# Size constants
readonly KB=1024
readonly MB=$((1024 * KB))
readonly GB=$((1024 * MB))

# Version info
readonly NSELF_VERSION="0.9.9"
readonly MIN_DOCKER_VERSION="20.10.0"
readonly MIN_COMPOSE_VERSION="2.0.0"
