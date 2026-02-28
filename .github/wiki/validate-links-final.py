#!/usr/bin/env python3
"""
Final comprehensive link validator for .wiki directory.
Properly handles code blocks, examples, and actual broken links.
"""

import re
from pathlib import Path
from collections import defaultdict

WIKI_DIR = Path('/Users/admin/Sites/nself/.wiki')

def extract_code_blocks(content):
    """Extract ranges of code blocks to skip"""
    code_blocks = []
    in_code_block = False
    start = 0

    for i, line in enumerate(content.split('\n')):
        if line.strip().startswith('```'):
            if in_code_block:
                code_blocks.append((start, i))
                in_code_block = False
            else:
                start = i
                in_code_block = True

    return code_blocks

def is_in_code_block(line_num, code_blocks):
    """Check if line is inside a code block"""
    for start, end in code_blocks:
        if start <= line_num <= end:
            return True
    return False

def validate_links():
    broken_links = []

    for md_file in WIKI_DIR.rglob('*.md'):
        if '.git' in str(md_file):
            continue

        try:
            content = md_file.read_text(encoding='utf-8', errors='ignore')
        except:
            continue

        # Get code blocks
        code_blocks = extract_code_blocks(content)

        # Find all markdown links
        for match in re.finditer(r'\[([^\]]+)\]\(([^)]+)\)', content):
            link_path = match.group(2)

            # Skip external links
            if link_path.startswith(('http://', 'https://', 'mailto:', '#')):
                continue

            # Skip regex patterns and wildcards
            if ('[' in link_path and ']' in link_path) or '.*' in link_path or '*' in link_path:
                continue

            # Check if in code block
            line_num = content[:match.start()].count('\n')
            if is_in_code_block(line_num, code_blocks):
                continue

            # Remove anchor
            base_path = link_path.split('#')[0]
            if not base_path:
                continue

            # Resolve path
            if base_path.startswith('/'):
                target = WIKI_DIR / base_path.lstrip('/')
            else:
                target = (md_file.parent / base_path).resolve()

            # Check if exists
            if not target.exists():
                broken_links.append({
                    'source': str(md_file.relative_to(WIKI_DIR)),
                    'link': link_path,
                    'base': base_path,
                    'line': line_num + 1
                })

    return broken_links

if __name__ == '__main__':
    print("=" * 80)
    print("FINAL LINK VALIDATION - .wiki Directory")
    print("=" * 80)
    print()

    broken = validate_links()

    if not broken:
        print("🎉 SUCCESS! ZERO BROKEN LINKS FOUND!")
        print()
        print("Documentation health: 100%")
        print("All internal links are valid.")
        exit(0)

    print(f"❌ Found {len(broken)} broken links:\n")

    # Group by file
    by_file = defaultdict(list)
    for item in broken:
        by_file[item['source']].append(item)

    for source, items in sorted(by_file.items()):
        print(f"\n{source}:")
        for item in items:
            print(f"  Line {item['line']}: {item['link']}")

    print(f"\n\nTotal: {len(broken)} broken links")
    exit(1)
