#!/usr/bin/env bash
# Purpose: every [[wiki-link]] in .github/wiki/ must resolve to a page that
#          exists. Broken links are invisible in review and only surface after
#          publish, so they are gated here (CLI-R08).
# Inputs:  none. Operates on .github/wiki/.
# Outputs: exit 0 when every link resolves; exit 1 listing the broken ones.
# Constraints:
#   - GitHub Wiki flattens the namespace, so a page in any subdirectory is
#     reachable by its basename; path-style links are matched on basename too.
#   - Anchors (#section) and a trailing .md are stripped before matching.
#   - comm needs both lists in one collation. The page list comes from sort and
#     the link list from Python's code-point sort, so the script pins LC_ALL=C;
#     under en_US.UTF-8 (macOS) comm reported ~195 false broken links (D-1288).
#     Tested by scripts/ci/wiki-link-audit_test.sh.
#   - Fenced code blocks AND inline `code` spans are skipped: templates and the
#     wiki's own link-audit notes show [[link]] syntax as an example, and
#     GitHub Wiki does not resolve a backtick-enclosed token as a link either.
set -euo pipefail

export LC_ALL=C

cd "$(dirname "$0")/../.."
WIKI=".github/wiki"

if [ ! -d "$WIKI" ]; then
  echo "wiki-link-audit: $WIKI not found" >&2
  exit 1
fi

PAGES=$(mktemp)
LINKS=$(mktemp)
trap 'rm -f "$PAGES" "$LINKS"' EXIT

find "$WIKI" -name '*.md' -exec basename {} .md \; | sort -u > "$PAGES"

python3 - "$WIKI" > "$LINKS" <<'PYEOF'
import os
import re
import sys

wiki = sys.argv[1]
link_re = re.compile(r"\[\[([^\]|#]+)")
found = set()

for root, _dirs, files in os.walk(wiki):
    for name in files:
        if not name.endswith(".md"):
            continue
        path = os.path.join(root, name)
        with open(path, encoding="utf-8") as fh:
            in_fence = False
            for line in fh:
                stripped = line.strip()
                if stripped.startswith("```") or stripped.startswith("~~~"):
                    in_fence = not in_fence
                    continue
                if in_fence:
                    continue
                # Strip inline code spans before looking for links: GitHub Wiki
                # does not resolve `[[Example]]` inside backticks, and the
                # wiki's own LINK-AUDIT page quotes the syntax that way.
                line = re.sub(r"`[^`]*`", "", line)
                for match in link_re.findall(line):
                    target = match.strip()
                    if target.endswith(".md"):
                        target = target[:-3]
                    # GitHub Wiki flattens directories: match on the basename.
                    target = target.rsplit("/", 1)[-1]
                    if not target:
                        continue
                    found.add(target)

for target in sorted(found):
    print(target)
PYEOF

BROKEN=$(comm -23 "$LINKS" "$PAGES")

if [ -n "$BROKEN" ]; then
  COUNT=$(echo "$BROKEN" | grep -c . || true)
  echo "::error::$COUNT broken wiki link target(s) — no page of that name exists:" >&2
  echo "$BROKEN" | sed 's/^/  [[/;s/$/]]/' >&2
  echo "" >&2
  echo "Create the page, fix the link, or run \`make wiki-commands\`." >&2
  exit 1
fi

echo "Wiki link audit passed: $(wc -l < "$LINKS" | tr -d ' ') distinct targets, all resolve."
