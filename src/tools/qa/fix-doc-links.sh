#!/usr/bin/env bash
#
# Fix Documentation Links Script
# Automatically fixes common link issues for wiki compatibility
#

set -euo pipefail

DOCS_DIR="/Users/admin/Sites/nself/.wiki"
DRY_RUN="${DRY_RUN:-true}"
BACKUP_DIR="/tmp/nself-docs-backup-$(date +%Y%m%d-%H%M%S)"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m' # No Color

fixes_applied=0
files_modified=0

printf "${BLUE}=== Documentation Link Fixer ===${NC}\n"

if [[ "$DRY_RUN" == "true" ]]; then
    printf "${YELLOW}DRY RUN MODE - No files will be modified${NC}\n"
    printf "Set DRY_RUN=false to apply fixes\n\n"
else
    printf "${GREEN}LIVE MODE - Files will be modified${NC}\n"
    printf "Creating backup at: %s\n\n" "$BACKUP_DIR"
    mkdir -p "$BACKUP_DIR"
    cp -r "$DOCS_DIR" "$BACKUP_DIR/"
fi

# Function to fix links in a file
fix_file_links() {
    local file="$1"
    local temp_file="${file}.tmp"
    local modified=false

    # Create a temp file for modifications
    cp "$file" "$temp_file"

    # Fix 1: Remove docs/ prefix from links
    # [Text](docs/folder/Page.md) → [Text](folder/Page.md)
    if sed -i.bak 's|\](docs/|\](|g' "$temp_file" 2>/dev/null; then
        if ! cmp -s "$file" "$temp_file"; then
            modified=true
            printf "  ${GREEN}✓${NC} Removed docs/ prefix\n"
            fixes_applied=$((fixes_applied + 1))
        fi
        rm -f "${temp_file}.bak"
    fi

    # Fix 2: Remove /.wiki/ prefix from absolute links
    # [Text](/.wiki/folder/Page.md) → [Text](folder/Page.md)
    if sed -i.bak 's|\](/.wiki/|\](|g' "$temp_file" 2>/dev/null; then
        if ! cmp -s "$file" "$temp_file"; then
            modified=true
            printf "  ${GREEN}✓${NC} Removed /.wiki/ prefix\n"
            fixes_applied=$((fixes_applied + 1))
        fi
        rm -f "${temp_file}.bak"
    fi

    # Fix 3: Convert ../api/ to ../reference/api/
    if sed -i.bak 's|\](../api/|\](../reference/api/|g' "$temp_file" 2>/dev/null; then
        if ! cmp -s "$file" "$temp_file"; then
            modified=true
            printf "  ${GREEN}✓${NC} Fixed ../api/ paths\n"
            fixes_applied=$((fixes_applied + 1))
        fi
        rm -f "${temp_file}.bak"
    fi

    # Fix 4: Convert ../cli/ to ../commands/
    if sed -i.bak 's|\](../cli/|\](../commands/|g' "$temp_file" 2>/dev/null; then
        if ! cmp -s "$file" "$temp_file"; then
            modified=true
            printf "  ${GREEN}✓${NC} Fixed ../cli/ paths\n"
            fixes_applied=$((fixes_applied + 1))
        fi
        rm -f "${temp_file}.bak"
    fi

    # Fix 5: Convert ../security/SECRETS-MANAGEMENT.md to ../configuration/SECRETS-MANAGEMENT.md
    if sed -i.bak 's|\](../security/SECRETS-MANAGEMENT\.md)|\](../configuration/SECRETS-MANAGEMENT.md)|g' "$temp_file" 2>/dev/null; then
        if ! cmp -s "$file" "$temp_file"; then
            modified=true
            printf "  ${GREEN}✓${NC} Fixed SECRETS-MANAGEMENT path\n"
            fixes_applied=$((fixes_applied + 1))
        fi
        rm -f "${temp_file}.bak"
    fi

    # Fix 6: Fix common file renames
    # QUICKSTART.md → Quick-Start.md
    if sed -i.bak 's|QUICKSTART\.md|Quick-Start.md|g' "$temp_file" 2>/dev/null; then
        if ! cmp -s "$file" "$temp_file"; then
            modified=true
            printf "  ${GREEN}✓${NC} Fixed QUICKSTART → Quick-Start\n"
            fixes_applied=$((fixes_applied + 1))
        fi
        rm -f "${temp_file}.bak"
    fi

    # Fix 7: Fix Installation.md path
    if sed -i.bak 's|\](Installation\.md)|\](../getting-started/Installation.md)|g' "$temp_file" 2>/dev/null; then
        if ! cmp -s "$file" "$temp_file"; then
            modified=true
            printf "  ${GREEN}✓${NC} Fixed Installation.md path\n"
            fixes_applied=$((fixes_applied + 1))
        fi
        rm -f "${temp_file}.bak"
    fi

    # Fix 8: Fix Quick-Start.md path
    if sed -i.bak 's|\](Quick-Start\.md)|\](../getting-started/Quick-Start.md)|g' "$temp_file" 2>/dev/null; then
        if ! cmp -s "$file" "$temp_file"; then
            modified=true
            printf "  ${GREEN}✓${NC} Fixed Quick-Start.md path\n"
            fixes_applied=$((fixes_applied + 1))
        fi
        rm -f "${temp_file}.bak"
    fi

    # Fix 9: Fix FAQ.md path
    if sed -i.bak 's|\](FAQ\.md)|\](../getting-started/FAQ.md)|g' "$temp_file" 2>/dev/null; then
        if ! cmp -s "$file" "$temp_file"; then
            modified=true
            printf "  ${GREEN}✓${NC} Fixed FAQ.md path\n"
            fixes_applied=$((fixes_applied + 1))
        fi
        rm -f "${temp_file}.bak"
    fi

    # Apply changes if any were made
    if [[ "$modified" == "true" ]]; then
        if [[ "$DRY_RUN" == "false" ]]; then
            mv "$temp_file" "$file"
            printf "  ${BLUE}→${NC} Applied fixes to file\n"
        else
            rm -f "$temp_file"
        fi
        files_modified=$((files_modified + 1))
        return 0
    else
        rm -f "$temp_file"
        return 1
    fi
}

# Process all markdown files
printf "${YELLOW}Processing markdown files...${NC}\n\n"

while IFS= read -r file; do
    relative_path="${file#$DOCS_DIR/}"

    # Skip certain files
    if [[ "$relative_path" == "LINK-AUDIT-REPORT.md" ]]; then
        continue
    fi

    printf "Checking: %s\n" "$relative_path"

    if fix_file_links "$file"; then
        printf "\n"
    fi

done < <(find "$DOCS_DIR" -name "*.md" -type f)

# Summary
printf "\n${BLUE}=== Summary ===${NC}\n"
printf "Files modified:  %d\n" "$files_modified"
printf "Fixes applied:   %d\n" "$fixes_applied"

if [[ "$DRY_RUN" == "true" ]]; then
    printf "\n${YELLOW}This was a dry run. No files were modified.${NC}\n"
    printf "Run with DRY_RUN=false to apply changes:\n"
    printf "  DRY_RUN=false bash %s\n" "$0"
else
    printf "\n${GREEN}✓ Fixes applied successfully${NC}\n"
    printf "Backup saved to: %s\n" "$BACKUP_DIR"
    printf "\nRun link audit again to verify:\n"
    printf "  python3 /Users/admin/Sites/nself/scripts/analyze-links.py\n"
fi
