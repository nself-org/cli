#!/bin/bash
# Sync /.wiki source into the GitHub wiki repository.
# Source of truth is always main repo /.wiki.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DOCS_DIR="$REPO_ROOT/.wiki"
WIKI_REPO="https://github.com/nself-org/cli.wiki.git"
WIKI_DIR="/tmp/nself-wiki-$$"

if [[ ! -d "$DOCS_DIR" ]]; then
  echo "Missing wiki source directory: $DOCS_DIR"
  exit 1
fi

echo "Syncing wiki from: $DOCS_DIR"

if git clone "$WIKI_REPO" "$WIKI_DIR" 2>/dev/null; then
  echo "Cloned existing wiki repo"
else
  echo "Wiki repo not initialized yet; creating local repo scaffold"
  mkdir -p "$WIKI_DIR"
  cd "$WIKI_DIR"
  git init
  git remote add origin "$WIKI_REPO"
fi

cd "$WIKI_DIR"

# Replace content (except .git) with /.wiki source.
find "$WIKI_DIR" -mindepth 1 -maxdepth 1 ! -name '.git' -exec rm -rf {} +
cp -R "$DOCS_DIR"/. "$WIKI_DIR"/

# Ensure required wiki entrypoints exist.
if [[ ! -f "$WIKI_DIR/Home.md" && -f "$WIKI_DIR/README.md" ]]; then
  cp "$WIKI_DIR/README.md" "$WIKI_DIR/Home.md"
fi

if [[ ! -f "$WIKI_DIR/CHANGELOG.md" && -f "$WIKI_DIR/releases/CHANGELOG.md" ]]; then
  cat > "$WIKI_DIR/CHANGELOG.md" <<'PAGE'
# Changelog

- [Full changelog](releases/CHANGELOG.md)
- [Release index](releases/INDEX.md)
PAGE
fi

git add -A
if git diff --staged --quiet; then
  echo "No wiki changes to commit"
  exit 0
fi

git commit -m "Sync wiki from /.wiki source"

echo "Pushing wiki changes"
git push origin HEAD

echo "Wiki sync complete"
