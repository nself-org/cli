#!/usr/bin/env bash
#
# Documentation Link Audit Script
# Scans all .md files for internal links and verifies they exist
#

set -euo pipefail

DOCS_DIR="/Users/admin/Sites/nself/.wiki"
REPORT_FILE="/Users/admin/Sites/nself/.wiki/LINK-AUDIT-REPORT.md"
BROKEN_LINKS_FILE="/tmp/broken-links.txt"
FIXED_LINKS_FILE="/tmp/fixed-links.txt"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

total_files=0
total_links=0
broken_links=0
external_links=0

printf "${BLUE}=== Documentation Link Audit ===${NC}\n"
printf "Scanning directory: %s\n\n" "$DOCS_DIR"

# Clear temp files
> "$BROKEN_LINKS_FILE"
> "$FIXED_LINKS_FILE"

# Start report
cat > "$REPORT_FILE" << 'EOF'
# Documentation Link Audit Report

Generated: $(date)

## Executive Summary

This report audits all internal links in the nself documentation for wiki compatibility.

### Statistics

EOF

# Function to normalize path for wiki links
normalize_wiki_path() {
  local path="$1"
  # Remove leading ./
  path="${path#./}"
  # Remove leading ../
  while [[ "$path" == ../* ]]; do
    path="${path#../}"
  done
  # Remove .md extension for wiki-style links
  path="${path%.md}"
  printf "%s" "$path"
}

# Function to check if file exists
file_exists() {
  local file="$1"
  local source_dir="$2"

  # Try exact path first
  if [[ -f "$DOCS_DIR/$file" ]]; then
    return 0
  fi

  # Try with .md extension
  if [[ -f "$DOCS_DIR/${file}.md" ]]; then
    return 0
  fi

  # Try relative to source directory
  local relative_path
  relative_path="$(dirname "$source_dir")/$file"
  if [[ -f "$relative_path" ]]; then
    return 0
  fi

  # Try with .md extension relative to source
  if [[ -f "${relative_path}.md" ]]; then
    return 0
  fi

  return 1
}

# Scan all markdown files
printf "${YELLOW}Scanning files...${NC}\n"

while IFS= read -r file; do
  total_files=$((total_files + 1))

  relative_path="${file#$DOCS_DIR/}"
  printf "  Checking: %s\n" "$relative_path"

  # Extract all markdown links: [text](url)
  while IFS= read -r line; do
    # Skip empty lines
    [[ -z "$line" ]] && continue

    # Extract link URL from [text](url) format
    if [[ "$line" =~ \[([^\]]+)\]\(([^\)]+)\) ]]; then
      link_text="${BASH_REMATCH[1]}"
      link_url="${BASH_REMATCH[2]}"

      total_links=$((total_links + 1))

      # Check if external link
      if [[ "$link_url" =~ ^https?:// ]]; then
        external_links=$((external_links + 1))
        continue
      fi

      # Skip anchors/fragments only
      if [[ "$link_url" =~ ^# ]]; then
        continue
      fi

      # Remove anchor from URL for file checking
      link_file="${link_url%%#*}"

      # Check if file exists
      if ! file_exists "$link_file" "$file"; then
        broken_links=$((broken_links + 1))
        printf "${RED}  ✗ BROKEN: %s → %s${NC}\n" "$link_text" "$link_url"
        printf "%s|%s|%s|%s\n" "$relative_path" "$link_text" "$link_url" "$link_file" >> "$BROKEN_LINKS_FILE"
      fi
    fi
  done < <(grep -o '\[.*\](.*\)' "$file" 2>/dev/null || true)

done < <(find "$DOCS_DIR" -name "*.md" -type f)

printf "\n${GREEN}Scan complete!${NC}\n\n"

# Generate statistics
cat >> "$REPORT_FILE" << EOF
- **Total Files Scanned**: $total_files
- **Total Links Found**: $total_links
- **Broken Internal Links**: $broken_links
- **External Links**: $external_links
EOF

if [[ $total_links -gt 0 ]]; then
  health_score=$(( (total_links - broken_links) * 100 / total_links ))
  printf "- **Health Score**: %d%%\n" "$health_score" >> "$REPORT_FILE"
else
  printf "- **Health Score**: N/A (no links found)\n" >> "$REPORT_FILE"
fi

cat >> "$REPORT_FILE" << 'EOF'

## Broken Links

EOF

if [[ $broken_links -eq 0 ]]; then
  printf "${GREEN}✓ No broken links found!${NC}\n"
  cat >> "$REPORT_FILE" << 'EOF'
**No broken links found!** All internal documentation links are valid.

EOF
else
  printf "${RED}✗ Found %d broken links${NC}\n" "$broken_links"

  cat >> "$REPORT_FILE" << 'EOF'
The following internal links are broken and need to be fixed:

| File | Link Text | URL | Status |
|------|-----------|-----|--------|
EOF

  while IFS='|' read -r file text url target; do
    # Suggest fix
    suggested_fix=""

    # Check if it's a case mismatch
    if [[ -f "$DOCS_DIR/${target}.md" ]]; then
      suggested_fix="File exists (case sensitive issue)"
    elif [[ -f "$DOCS_DIR/$(basename "$target").md" ]]; then
      suggested_fix="Move to: $(basename "$target").md"
    else
      # Search for similar filenames
      similar=$(find "$DOCS_DIR" -name "*$(basename "$target")*.md" -type f | head -1)
      if [[ -n "$similar" ]]; then
        suggested_fix="Similar: ${similar#$DOCS_DIR/}"
      else
        suggested_fix="File not found"
      fi
    fi

    printf "| %s | %s | \`%s\` | %s |\n" "$file" "$text" "$url" "$suggested_fix" >> "$REPORT_FILE"
  done < "$BROKEN_LINKS_FILE"

  cat >> "$REPORT_FILE" << 'EOF'

EOF
fi

# Add recommendations
cat >> "$REPORT_FILE" << 'EOF'
## Recommendations

### For Wiki Compatibility

1. **Use relative paths**: `[Text](../folder/Page.md)` or `[Text](Page.md)`
2. **Remove file extensions**: Wiki links should be `[Text](Page)` not `[Text](Page.md)`
3. **Use kebab-case**: File names should use hyphens, not underscores
4. **Avoid absolute paths**: Don't use `/.wiki/...` paths
5. **Check case sensitivity**: Links are case-sensitive

### Link Format Examples

#### Good (Wiki-Compatible)
```markdown
[Installation Guide](getting-started/Installation)
[Quick Start](Quick-Start)
[Architecture](../architecture/ARCHITECTURE)
```

#### Bad (Not Wiki-Compatible)
```markdown
[Installation Guide](/.wiki/getting-started/Installation.md)
[Quick Start](./quick-start.md)
[Architecture](../../architecture/ARCHITECTURE.MD)
```

## Next Steps

EOF

if [[ $broken_links -gt 0 ]]; then
  cat >> "$REPORT_FILE" << 'EOF'
1. Review each broken link in the table above
2. Update links to use wiki-compatible format
3. Verify all referenced files exist
4. Re-run this audit script to verify fixes
5. Update _Sidebar.md with corrected links

EOF
else
  cat >> "$REPORT_FILE" << 'EOF'
1. Convert remaining links to wiki format (remove .md extensions)
2. Standardize on relative paths
3. Update _Sidebar.md to match wiki structure
4. Test links in GitHub Wiki environment

EOF
fi

# Print summary
cat >> "$REPORT_FILE" << 'EOF'
## Files Scanned

EOF

printf "Total markdown files: %d\n\n" "$total_files" >> "$REPORT_FILE"

# Save report
printf "${GREEN}Report saved to: %s${NC}\n" "$REPORT_FILE"

# Summary to console
printf "\n${BLUE}=== Summary ===${NC}\n"
printf "Files scanned:     %d\n" "$total_files"
printf "Total links:       %d\n" "$total_links"
printf "External links:    %d\n" "$external_links"
printf "Broken links:      %d\n" "$broken_links"

if [[ $broken_links -eq 0 ]]; then
  printf "\n${GREEN}✓ All internal links are valid!${NC}\n"
  exit 0
else
  printf "\n${RED}✗ Found %d broken links that need fixing${NC}\n" "$broken_links"
  printf "See report: %s\n" "$REPORT_FILE"
  exit 1
fi
