#!/usr/bin/env bash
#
# Check _Sidebar.md Links
# Verifies all links in the sidebar exist
#

set -euo pipefail

DOCS_DIR="/Users/admin/Sites/nself/.wiki"
SIDEBAR="$DOCS_DIR/_Sidebar.md"

GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

valid=0
broken=0

printf "Checking _Sidebar.md links...\n\n"

# Extract links from sidebar
while IFS= read -r line; do
    if [[ "$line" =~ \[([^\]]+)\]\(([^\)]+)\) ]]; then
        text="${BASH_REMATCH[1]}"
        url="${BASH_REMATCH[2]}"

        # Skip anchors
        [[ "$url" == \#* ]] && continue

        # Check if file exists
        file="$DOCS_DIR/${url}.md"

        if [[ -f "$file" ]]; then
            printf "${GREEN}✓${NC} %s → %s\n" "$text" "$url"
            valid=$((valid + 1))
        else
            printf "${RED}✗${NC} %s → %s (NOT FOUND)\n" "$text" "$url"
            broken=$((broken + 1))
        fi
    fi
done < "$SIDEBAR"

printf "\n=== Summary ===\n"
printf "Valid links:  %d\n" "$valid"
printf "Broken links: %d\n" "$broken"

if [[ $broken -eq 0 ]]; then
    printf "\n${GREEN}✓ All sidebar links are valid!${NC}\n"
    exit 0
else
    printf "\n${RED}✗ Found %d broken links in sidebar${NC}\n" "$broken"
    exit 1
fi
