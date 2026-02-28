#!/bin/bash
# Comprehensive broken link finder for .wiki directory

WIKI_DIR="/Users/admin/Sites/nself/.wiki"
cd "$WIKI_DIR" || exit 1

broken_count=0

echo "=== Finding ALL Broken Links in .wiki ==="
echo ""

# Function to resolve relative path
resolve_path() {
    local source_file="$1"
    local link_target="$2"
    local source_dir

    source_dir=$(dirname "$source_file")

    # Handle absolute paths from wiki root
    if [[ "$link_target" == /* ]]; then
        echo "${WIKI_DIR}${link_target}"
    else
        # Resolve relative path
        echo "$(cd "$source_dir" && pwd)/${link_target}"
    fi
}

# Find all markdown files
while IFS= read -r file; do
    # Extract markdown links
    grep -o '\]([^)]*\.md[^)]*)' "$file" 2>/dev/null | sed 's/](\([^)]*\))/\1/' | while IFS= read -r link; do
        # Skip external links
        [[ "$link" =~ ^https?:// ]] && continue

        # Remove anchor fragments
        link_path="${link%%#*}"

        # Resolve the full path
        full_path=$(resolve_path "$file" "$link_path")

        # Check if file exists
        if [[ ! -f "$full_path" ]]; then
            echo "BROKEN: $file"
            echo "  -> $link"
            echo "  -> Resolved: $full_path"
            echo ""
            ((broken_count++))
        fi
    done
done < <(find . -name "*.md" -type f)

echo ""
echo "=== Summary ==="
echo "Total broken links: $broken_count"
