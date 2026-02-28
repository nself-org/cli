#!/usr/bin/env bash
# Fix documentation issues found in validation report
# Date: January 31, 2026
# DO NOT RUN - Review changes manually first

set -e

DOCS_DIR="/Users/admin/Sites/nself/docs"

echo "Documentation Fix Script - DRY RUN MODE"
echo "========================================"
echo ""
echo "This script will fix the issues found in DOCUMENTATION-VALIDATION-REPORT.md"
echo ""

# Issue 1: Fix "nself db migrate apply" → "nself db migrate up"
echo "Issue 1: Fixing 'nself db migrate apply' → 'nself db migrate up'"
echo "----------------------------------------------------------------"

FILES_TO_FIX=(
  "$DOCS_DIR/examples/projects/01-simple-blog/README.md"
  "$DOCS_DIR/examples/projects/02-saas-starter/README.md"
  "$DOCS_DIR/reference/QUICK-REFERENCE-CARDS.md"
)

for file in "${FILES_TO_FIX[@]}"; do
  if [[ -f "$file" ]]; then
    echo "Would update: $file"
    # Actual fix (commented out for safety):
    # sed -i '' 's/nself db migrate apply/nself db migrate up/g' "$file"
  else
    echo "File not found: $file"
  fi
done

echo ""

# Issue 2: Fix domain inconsistencies
echo "Issue 2: Standardizing domains to *.local.nself.org"
echo "---------------------------------------------------"

FILES_WITH_DOMAIN_ISSUES=(
  "$DOCS_DIR/examples/projects/01-simple-blog/README.md"
  "$DOCS_DIR/examples/projects/01-simple-blog/TUTORIAL.md"
)

for file in "${FILES_WITH_DOMAIN_ISSUES[@]}"; do
  if [[ -f "$file" ]]; then
    echo "Would update: $file"
    # Actual fixes (commented out for safety):
    # sed -i '' 's|http://api\.localhost|https://api.local.nself.org|g' "$file"
    # sed -i '' 's|http://auth\.localhost|https://auth.local.nself.org|g' "$file"
    # sed -i '' 's|http://localhost:3000|https://app.local.nself.org|g' "$file"
  fi
done

echo ""

# Issue 3: Update docker-compose v1 → v2 (low priority)
echo "Issue 3: Updating docker-compose to docker compose"
echo "--------------------------------------------------"

FILES_WITH_DOCKER_COMPOSE=(
  "$DOCS_DIR/guides/Deployment.md"
  "$DOCS_DIR/development/ERROR-HANDLING.md"
)

for file in "${FILES_WITH_DOCKER_COMPOSE[@]}"; do
  if [[ -f "$file" ]]; then
    echo "Would update: $file"
    # Actual fix (commented out for safety):
    # sed -i '' 's/docker-compose up/docker compose up/g' "$file"
  fi
done

echo ""
echo "========================================"
echo "DRY RUN COMPLETE"
echo ""
echo "To apply these fixes:"
echo "1. Review each change manually"
echo "2. Uncomment the sed commands in this script"
echo "3. Run: bash $0"
echo ""
echo "Or apply manually using the search patterns from the validation report."
