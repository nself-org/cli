#!/usr/bin/env bash
# sync-badges.sh — Sync README version badges across all nSelf repos to the
# target version from MASTER-VERSIONS.md.
#
# Usage:
#   ./scripts/sync-badges.sh                  # auto-detect version from MASTER-VERSIONS.md
#   ./scripts/sync-badges.sh --version 1.0.10 # explicit version
#
# Bash 3.2 compatible (no echo -e, no ${var,,}, no declare -A, no {1..n} brace expansion).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
NSELF_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# ---------------------------------------------------------------------------
# Argument parsing
# ---------------------------------------------------------------------------
TARGET_VERSION=""

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version)
      TARGET_VERSION="$2"
      shift 2
      ;;
    --version=*)
      TARGET_VERSION="${1#--version=}"
      shift
      ;;
    -h|--help)
      printf "Usage: %s [--version X.Y.Z]\n" "$0"
      exit 0
      ;;
    *)
      printf "Unknown argument: %s\n" "$1" >&2
      exit 1
      ;;
  esac
done

# ---------------------------------------------------------------------------
# Detect version from MASTER-VERSIONS.md if not supplied
# ---------------------------------------------------------------------------
MASTER_VERSIONS="$NSELF_ROOT/.claude/docs/MASTER-VERSIONS.md"

if [ -z "$TARGET_VERSION" ]; then
  if [ ! -f "$MASTER_VERSIONS" ]; then
    printf "ERROR: MASTER-VERSIONS.md not found at %s\n" "$MASTER_VERSIONS" >&2
    exit 1
  fi
  # Extract the CLI version line: | CLI | **v1.0.10** | ...
  TARGET_VERSION="$(grep -E '^\| CLI \|' "$MASTER_VERSIONS" | head -1 | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | head -1)"
  if [ -z "$TARGET_VERSION" ]; then
    printf "ERROR: Could not parse CLI version from MASTER-VERSIONS.md\n" >&2
    exit 1
  fi
  # Strip leading 'v' for badge text (badge uses bare version, URL uses tag)
  VERSION_BARE="${TARGET_VERSION#v}"
else
  # Accept with or without leading 'v'
  TARGET_VERSION="${TARGET_VERSION#v}"
  TARGET_VERSION="v${TARGET_VERSION}"
  VERSION_BARE="${TARGET_VERSION#v}"
fi

printf "Syncing README badges to %s across all nSelf repos...\n" "$TARGET_VERSION"

# ---------------------------------------------------------------------------
# Repo list and GitHub org names (parallel arrays — Bash 3.2 compatible)
# ---------------------------------------------------------------------------
REPO_NAMES="cli admin nchat nclaw ntask ntv nfamily clawde plugins plugins-pro homebrew-nself"
GITHUB_REPOS="nself-org/cli nself-org/admin nself-org/nchat nself-org/nclaw nself-org/ntask nself-org/ntv nself-org/nfamily nself-org/clawde nself-org/plugins nself-org/bundles nself-org/homebrew-nself"

# Build index function using positional parsing (Bash 3.2 safe)
get_github_repo() {
  local target_name="$1"
  local idx=0
  for name in $REPO_NAMES; do
    idx=$((idx + 1))
    if [ "$name" = "$target_name" ]; then
      echo "$GITHUB_REPOS" | tr ' ' '\n' | sed -n "${idx}p"
      return
    fi
  done
}

# ---------------------------------------------------------------------------
# Badge update function
# Preferred approach: switch static version badges to dynamic shields.io
# endpoints so they never go stale again.
# ---------------------------------------------------------------------------
CHANGED=0
ERRORS=0
SKIPPED=0

update_readme() {
  local repo_name="$1"
  local github_repo="$2"
  local readme="$NSELF_ROOT/$repo_name/README.md"

  if [ ! -f "$readme" ]; then
    printf "  SKIP  %-20s (README.md not found)\n" "$repo_name"
    SKIPPED=$((SKIPPED + 1))
    return
  fi

  # Dynamic badge URL for GitHub releases
  DYNAMIC_VERSION_BADGE="https://img.shields.io/github/v/release/$github_repo?label=version"
  DYNAMIC_VERSION_BADGE_URL="https://github.com/$github_repo/releases/latest"

  # Detect if there is already a static version badge that needs updating.
  # Patterns we handle:
  #   https://img.shields.io/badge/version-v1.0.9_LTS-blue.svg
  #   https://img.shields.io/badge/version-1.0.0-blue.svg
  #   https://img.shields.io/badge/version-1.0.9%2B1-blue.svg
  #   https://img.shields.io/badge/version-1.1.1-blue.svg
  # We replace ALL static version badges with the dynamic endpoint.

  local tmp_file="${readme}.tmp"

  # Use perl for in-place-safe replacement (Bash 3.2 compatible; perl is macOS default).
  # Replace static shields.io/badge/version-... with dynamic github v/release endpoint.
  perl -pe "
    s{https://img\.shields\.io/badge/version-[^)\"]+}{$DYNAMIC_VERSION_BADGE}g;
  " "$readme" > "$tmp_file"

  # Also update the badge link target if it points to a static release tag URL.
  # e.g. .../releases/tag/v1.0.9 -> .../releases/latest
  perl -pi -e "
    s{https://github\.com/$github_repo/releases/tag/v[0-9]+\.[0-9]+\.[0-9]+}{$DYNAMIC_VERSION_BADGE_URL}g;
  " "$tmp_file" 2>/dev/null || true

  # Check if the file actually changed.
  if ! diff -q "$readme" "$tmp_file" > /dev/null 2>&1; then
    mv "$tmp_file" "$readme"
    printf "  UPDATED %-20s (dynamic version badge applied)\n" "$repo_name"
    CHANGED=$((CHANGED + 1))

    # Commit the change.
    (
      cd "$NSELF_ROOT/$repo_name" || exit 1
      if git diff --quiet README.md 2>/dev/null; then
        printf "  NOTE: %s README.md has no tracked changes (may be untracked).\n" "$repo_name"
        return
      fi
      git add README.md
      git commit -m "chore: sync version badges to $TARGET_VERSION"
      printf "  COMMITTED %s\n" "$repo_name"
    ) || {
      printf "  ERROR: git commit failed for %s\n" "$repo_name" >&2
      ERRORS=$((ERRORS + 1))
    }
  else
    rm -f "$tmp_file"
    printf "  OK    %-20s (badge already dynamic or no static badge found)\n" "$repo_name"
    SKIPPED=$((SKIPPED + 1))
  fi
}

# ---------------------------------------------------------------------------
# Run across all repos
# ---------------------------------------------------------------------------
for repo_name in $REPO_NAMES; do
  github_repo="$(get_github_repo "$repo_name")"
  update_readme "$repo_name" "$github_repo"
done

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
printf "\n"
printf "Badge sync complete.\n"
printf "  Changed:  %d repos\n" "$CHANGED"
printf "  Skipped:  %d repos\n" "$SKIPPED"
printf "  Errors:   %d\n" "$ERRORS"

if [ "$ERRORS" -gt 0 ]; then
  exit 1
fi
