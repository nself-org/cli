#!/usr/bin/env bash
# Purpose: name the Go packages a change can affect, so a Ticket verifies with
#          `go test -mod=vendor $(bash scripts/ci/affected-pkgs.sh origin/main)`
#          instead of the whole suite. The full suite still runs once per
#          merge-train batch and in CI.
# Usage:   affected-pkgs.sh <base-ref>
# Inputs:  <base-ref>: any git ref. The change set is every file that differs
#          between merge-base(<base-ref>, HEAD) and HEAD, plus staged,
#          unstaged and untracked files (so it also works before the commit).
# Outputs: stdout, one import path per line, sorted. A changed file seeds:
#            - the package that embeds it (go:embed, test embeds included);
#            - the package in its nearest enclosing package directory (its own
#              .go files, testdata/, data files read beside the code); a root
#              package only for root files and the root testdata/;
#            - for every non-Go file (including docs, scripts, .github/),
#              every package whose .go files name its directory
#              path (`dir/` or `/dir"` for a top-level one), its top-level
#              directory as a quoted literal, or (for a root file) its file
#              name: tests that read the wiki or a CI script are found that way.
#          Seeds from .go, embedded files and deleted data owners are closed over reverse imports
#          (`go list -deps`, test imports included); the other seeds only
#          feed their own package's tests, so they are listed as they are. Prints `./...` alone when go.mod, go.sum,
#          vendor/ or anything under internal/errs/ changed (everything
#          depends on those). Prints nothing only when no file changed or no
#          package is seeded by the rules above; a note goes to stderr then.
#          Callers must not pass an empty list to `go test`.
#          Exit 0 on success, 2 on a usage error or an unknown ref, 1 when
#          `go list` fails.
# Constraints: bash 3.2 compatible; POSIX awk only (BSD, GNU and mawk), every
#          list reaches awk as a file, never through -v; read-only apart from
#          a temp dir it removes; needs git and go. A package that was deleted
#          has no import graph entry; its importers changed too, so they are
#          listed.
set -euo pipefail

if [ "$#" -ne 1 ] || [ -z "$1" ]; then
  echo "usage: affected-pkgs.sh <base-ref>" >&2
  exit 2
fi
BASE="$1"
cd "$(git rev-parse --show-toplevel)"
if ! git rev-parse --verify --quiet "$BASE^{commit}" >/dev/null; then
  echo "affected-pkgs: unknown ref '$BASE'" >&2
  exit 2
fi

TMP="$(mktemp -d "${TMPDIR:-/tmp}/affected-pkgs.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

# core.quotepath=false: a non-ASCII path is printed as is, not "\303\251".
{
  git -c core.quotepath=false diff --name-only "$BASE"...HEAD
  git -c core.quotepath=false diff --name-only HEAD
  git -c core.quotepath=false ls-files --others --exclude-standard
} | sed '/^$/d' | LC_ALL=C sort -u > "$TMP/changed"

if [ ! -s "$TMP/changed" ]; then
  echo "affected-pkgs: no file changed against $BASE" >&2
  exit 0
fi

# Changes everything depends on: rebuild the world.
if grep -qE '^(go\.mod|go\.sum|vendor/|internal/errs/)' "$TMP/changed"; then
  echo "./..."
  exit 0
fi

MODDIR="$(CGO_ENABLED=0 go list -mod=vendor -m -f '{{.Dir}}')"
MODPATH="$(CGO_ENABLED=0 go list -mod=vendor -m -f '{{.Path}}')"

# One line per package:
#   <import path>|<dir>|<imports, test imports too>|<embedded files, test embeds too>
CGO_ENABLED=0 go list -mod=vendor -deps -test=false \
  -f '{{.ImportPath}}|{{.Dir}}|{{join .Imports " "}} {{join .TestImports " "}} {{join .XTestImports " "}}|{{join .EmbedFiles " "}} {{join .TestEmbedFiles " "}} {{join .XTestEmbedFiles " "}}' \
  ./... > "$TMP/graph"

# Pass 1: map each changed file to packages. "P <pkg>": the change reaches
# importers (a .go file or an embedded file). "D <pkg>": only that package's
# own tests read it (testdata, a data file beside the code). "U <file>":
# outside every package directory.
awk -F'|' -v modpath="$MODPATH" -v moddir="$MODDIR" '
  FNR == NR { changed[++nc] = $0; next }
  {
    pkg = $1
    if (pkg != modpath && index(pkg, modpath "/") != 1) next
    rel = substr($2, length(moddir) + 2)
    if (rel == "") rel = "."
    dirpkg[rel] = pkg
    m = split($4, emb, " ")
    for (j = 1; j <= m; j++) {
      if (emb[j] == "") continue
      embedded[(rel == "." ? "" : rel "/") emb[j]] = pkg
    }
  }
  END {
    for (i = 1; i <= nc; i++) {
      f = changed[i]; found = 0
      if (f in embedded) { print "P " embedded[f]; found = 1 }
      d = f; indir = 0
      while (index(d, "/")) {
        sub(/\/[^\/]*$/, "", d)
        if (d in dirpkg) { print (f ~ /\.go$/ ? "P " : "D ") dirpkg[d]; found = 1; indir = 1; break }
      }
      # A root package owns only root files and the root testdata/, so a
      # docs/ or scripts/ change does not seed it.
      if (!indir && (f !~ /\// || f ~ /^testdata\//) && ("." in dirpkg)) {
        print (f ~ /\.go$/ ? "P " : "D ") dirpkg["."]; found = 1
      }
      if (!found) print "U " f
    }
  }
' "$TMP/changed" "$TMP/graph" > "$TMP/mapped"

sed -n 's/^P //p' "$TMP/mapped" > "$TMP/seeds"
sed -n 's/^D //p' "$TMP/mapped" > "$TMP/direct"

# Data can be owned by one package and read by another. Scan every non-Go
# path, including owned data, for those readers.
sed '/\.go$/d' "$TMP/changed" | while IFS= read -r f; do
  case "$f" in
    */*/*)
      printf '%s\n' "${f%/*}"
      printf '"%s"\n' "${f%%/*}"
      ;;
    */*)
      # A bare one-word directory name matches too much prose.
      printf '%s/\n/%s"\n"%s"\n' "${f%%/*}" "${f%%/*}" "${f%%/*}"
      ;;
    *) printf '%s\n' "$f" ;;
  esac
done | LC_ALL=C sort -u > "$TMP/patterns"

if [ -s "$TMP/patterns" ]; then
  git ls-files --cached --others --exclude-standard -- '*.go' ':(exclude)vendor/**' > "$TMP/gofiles"
  if [ -s "$TMP/gofiles" ]; then
    # NUL-separated so odd file names survive; xargs splits long lists.
    tr '\n' '\0' < "$TMP/gofiles" | xargs -0 grep -lF -f "$TMP/patterns" -- 2>/dev/null \
      | sed 's|/[^/]*$||; s|^[^/]*\.go$|.|' | LC_ALL=C sort -u > "$TMP/refdirs" || true
    if [ -s "$TMP/refdirs" ]; then
      awk -F'|' -v modpath="$MODPATH" -v moddir="$MODDIR" '
        FNR == NR { want[$0] = 1; next }
        {
          if ($1 != modpath && index($1, modpath "/") != 1) next
          rel = substr($2, length(moddir) + 2)
          if (rel == "") rel = "."
          if (rel in want) print $1
        }
      ' "$TMP/refdirs" "$TMP/graph" >> "$TMP/direct"
    fi
  fi
fi

# A removed embedded file no longer appears in go list's EmbedFiles. Close
# deleted data owners over imports conservatively, so wildcard embeds still
# reach their consumers when another matching file keeps the pattern valid.
while IFS= read -r f; do
  [ -e "$f" ] && continue
  case "$f" in *.go) continue ;; esac
  d="$f"
  while [ "${d%/*}" != "$d" ]; do
    d="${d%/*}"
    awk -F'|' -v dir="$MODDIR/$d" '$2 == dir { print $1 }' "$TMP/graph"
  done
  case "$f" in
    */*) ;;
    *) awk -F'|' -v dir="$MODDIR" '$2 == dir { print $1 }' "$TMP/graph" ;;
  esac
done < "$TMP/changed" >> "$TMP/seeds"

if [ ! -s "$TMP/seeds" ] && [ ! -s "$TMP/direct" ]; then
  echo "affected-pkgs: no Go package is affected by this change set" >&2
  exit 0
fi

# Pass 2: close the seeds over reverse imports, then add the direct ones.
awk -F'|' -v modpath="$MODPATH" '
  FNR == NR { if ($0 != "") seed[$0] = 1; next }
  {
    pkg = $1
    if (pkg != modpath && index(pkg, modpath "/") != 1) next
    pkgs[pkg] = 1
    m = split($3, list, " ")
    for (j = 1; j <= m; j++) {
      if (list[j] == modpath || index(list[j], modpath "/") == 1) {
        importers[list[j]] = importers[list[j]] " " pkg
      }
    }
  }
  END {
    qn = 0
    for (p in seed) { hit[p] = 1; queue[++qn] = p }
    for (qi = 1; qi <= qn; qi++) {
      k = split(importers[queue[qi]], who, " ")
      for (j = 1; j <= k; j++) {
        if (!(who[j] in hit)) { hit[who[j]] = 1; queue[++qn] = who[j] }
      }
    }
    for (p in hit) if (p in pkgs) print p
  }
' "$TMP/seeds" "$TMP/graph" > "$TMP/closed"
cat "$TMP/closed" "$TMP/direct" | sed '/^$/d' | LC_ALL=C sort -u
