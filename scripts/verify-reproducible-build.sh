#!/usr/bin/env bash
# verify-reproducible-build.sh
# T-0458 — Build reproducibility: CLI release binary same SHA256 from same commit
#
# Usage: ./verify-reproducible-build.sh [commit-sha]
#
# Builds the CLI release binary twice from the same git commit and compares
# SHA256 checksums. Exits 1 if they differ (non-determinism detected).
#
# Bash 3.2+ compatible.

set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
TARGET_COMMIT="${1:-HEAD}"

printf '[verify-reproducible-build] repo: %s\n' "$REPO_ROOT"
printf '[verify-reproducible-build] commit: %s\n' "$TARGET_COMMIT"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

sha256_file() {
  local file="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file" | awk '{print $1}'
  else
    printf 'ERROR: no sha256sum or shasum available\n' >&2
    return 1
  fi
}

# ---------------------------------------------------------------------------
# Determine build method
# ---------------------------------------------------------------------------

BUILD_METHOD=""

if [ -f "$REPO_ROOT/Cargo.toml" ]; then
  BUILD_METHOD="cargo"
elif [ -f "$REPO_ROOT/Makefile" ] && grep -q "release" "$REPO_ROOT/Makefile" 2>/dev/null; then
  BUILD_METHOD="make"
elif [ -f "$REPO_ROOT/src/VERSION" ]; then
  # Pure Bash CLI — package into a tarball for reproducibility check
  BUILD_METHOD="bash"
else
  printf 'ERROR: cannot determine build method for CLI repo at %s\n' "$REPO_ROOT" >&2
  exit 1
fi

printf '[verify-reproducible-build] build method: %s\n' "$BUILD_METHOD"

# ---------------------------------------------------------------------------
# Prepare two clean worktrees
# ---------------------------------------------------------------------------

WORK_DIR="$(mktemp -d)"
TREE_A="$WORK_DIR/build-a"
TREE_B="$WORK_DIR/build-b"
OUTPUT_A="$WORK_DIR/output-a"
OUTPUT_B="$WORK_DIR/output-b"

mkdir -p "$OUTPUT_A" "$OUTPUT_B"

cleanup() {
  rm -rf "$WORK_DIR"
}
trap cleanup EXIT

# Check out the target commit into two separate worktrees
cd "$REPO_ROOT"
git worktree add "$TREE_A" "$TARGET_COMMIT" --detach >/dev/null 2>&1
git worktree add "$TREE_B" "$TARGET_COMMIT" --detach >/dev/null 2>&1

printf '[verify-reproducible-build] worktrees created: %s, %s\n' "$TREE_A" "$TREE_B"

# ---------------------------------------------------------------------------
# Build A
# ---------------------------------------------------------------------------

printf '[verify-reproducible-build] starting build A...\n'

case "$BUILD_METHOD" in
  cargo)
    cd "$TREE_A"
    RUSTFLAGS="--remap-path-prefix $(pwd)=." cargo build --release 2>/dev/null
    cp target/release/nself "$OUTPUT_A/nself" 2>/dev/null || \
      find target/release -maxdepth 1 -type f -name 'nself*' ! -name '*.d' \
        -exec cp {} "$OUTPUT_A/" \;
    ;;
  make)
    cd "$TREE_A"
    make release OUTPUT_DIR="$OUTPUT_A" 2>/dev/null
    ;;
  bash)
    cd "$TREE_A"
    # Package CLI as reproducible tarball: sort files, strip timestamps
    find . -type f -name '*.sh' | sort | \
      tar --create --file="$OUTPUT_A/nself.tar" \
          --transform 's|^\./||' \
          --no-recursion \
          --files-from=- 2>/dev/null || \
      tar cf "$OUTPUT_A/nself.tar" src/ bin/ 2>/dev/null
    ;;
esac

# ---------------------------------------------------------------------------
# Build B
# ---------------------------------------------------------------------------

printf '[verify-reproducible-build] starting build B...\n'

case "$BUILD_METHOD" in
  cargo)
    cd "$TREE_B"
    RUSTFLAGS="--remap-path-prefix $(pwd)=." cargo build --release 2>/dev/null
    cp target/release/nself "$OUTPUT_B/nself" 2>/dev/null || \
      find target/release -maxdepth 1 -type f -name 'nself*' ! -name '*.d' \
        -exec cp {} "$OUTPUT_B/" \;
    ;;
  make)
    cd "$TREE_B"
    make release OUTPUT_DIR="$OUTPUT_B" 2>/dev/null
    ;;
  bash)
    cd "$TREE_B"
    find . -type f -name '*.sh' | sort | \
      tar --create --file="$OUTPUT_B/nself.tar" \
          --transform 's|^\./||' \
          --no-recursion \
          --files-from=- 2>/dev/null || \
      tar cf "$OUTPUT_B/nself.tar" src/ bin/ 2>/dev/null
    ;;
esac

# ---------------------------------------------------------------------------
# Remove git worktrees
# ---------------------------------------------------------------------------

cd "$REPO_ROOT"
git worktree remove "$TREE_A" --force >/dev/null 2>&1 || true
git worktree remove "$TREE_B" --force >/dev/null 2>&1 || true

# ---------------------------------------------------------------------------
# Compare SHA256 of all output files
# ---------------------------------------------------------------------------

printf '[verify-reproducible-build] comparing checksums...\n'

FAIL=0

for file_a in "$OUTPUT_A"/*; do
  base="$(basename "$file_a")"
  file_b="$OUTPUT_B/$base"

  if [ ! -f "$file_b" ]; then
    printf 'FAIL: build B missing file: %s\n' "$base"
    FAIL=1
    continue
  fi

  sha_a="$(sha256_file "$file_a")"
  sha_b="$(sha256_file "$file_b")"

  if [ "$sha_a" = "$sha_b" ]; then
    printf 'PASS: %s  sha256=%s\n' "$base" "$sha_a"
  else
    printf 'FAIL: %s is NOT reproducible\n' "$base"
    printf '      build A: %s\n' "$sha_a"
    printf '      build B: %s\n' "$sha_b"
    FAIL=1
  fi
done

printf '\n'

if [ "$FAIL" -ne 0 ]; then
  printf '[verify-reproducible-build] RESULT: NON-DETERMINISTIC — at least one file differed\n'
  exit 1
else
  printf '[verify-reproducible-build] RESULT: REPRODUCIBLE — all files identical\n'
  exit 0
fi
