#!/usr/bin/env bash

# Fix npm ci to npm install in all templates

TEMPLATE_DIR="${NSELF_TEMPLATES:-${NSELF_ROOT:-/usr/local/lib/nself}/src/templates}/services"

set -euo pipefail


# Find all Dockerfile templates with npm ci
for file in $(find "$TEMPLATE_DIR" -name "Dockerfile.template" -type f); do
  if grep -q "npm ci" "$file"; then
    echo "Fixing: $file"
    # Replace npm ci with npm install
    sed -i.bak 's/npm ci/npm install/g' "$file"
    # Clean up backup files
    rm -f "${file}.bak"
  fi
done

echo "All templates updated to use npm install instead of npm ci"
