#!/usr/bin/env bash
# Purpose: test scripts/ci/affected-pkgs.sh on a throwaway fixture module, once
#          per awk found on this host (system awk, gawk, mawk, nawk,
#          original-awk, busybox awk), so the BSD awk on macOS and the GNU awk
#          on Linux both run it. Covers .go, go:embed, testdata, docs read by a
#          test, a package-free change, the rebuild-the-world rule and a bad
#          ref.
# Usage:   bash scripts/ci/affected-pkgs_test.sh [<script-under-test>]
# Inputs:  optional path to the script under test (default: the sibling
#          affected-pkgs.sh). A positive control runs a copy of the script
#          with its embed mapping removed and must see a mismatch.
# Outputs: "ok <awk> <case>" per passing case, a note when no strict (BSD)
#          awk or only one awk flavour ran, "awks tested: ..." and
#          "AFFECTED-PKGS-TEST: PASS"; exit 1 after a run with any mismatch.
# Constraints: bash 3.2 compatible; needs git and go; no network (the fixture
#          module has no dependencies). Writes only under a mktemp dir.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="${1:-$HERE/affected-pkgs.sh}"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/affected-pkgs-test.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

export GOWORK=off GOFLAGS='' GOTOOLCHAIN=local
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
export GIT_AUTHOR_NAME=fixture GIT_AUTHOR_EMAIL=fixture@example.invalid
export GIT_COMMITTER_NAME=fixture GIT_COMMITTER_EMAIL=fixture@example.invalid

# ---- fixture module --------------------------------------------------------
# a: embeds assets/conf.yaml, which sits in package a/assets, so only the
# embed mapping (not the enclosing directory) names a.  b: imports a.
# c: has testdata/.  d: its test reads ../docs/guide.md.  e: imports nothing
# and nothing imports it.  f: imports d (a docs change must not reach f).
R="$WORK/repo"
mkdir -p "$R/a/assets" "$R/b" "$R/c/testdata" "$R/d" "$R/e" "$R/f" "$R/docs" "$R/notes"
GOV="$(go env GOVERSION | sed -E 's/^go([0-9]+\.[0-9]+).*/\1/')"
printf 'module example.invalid/fx\n\ngo %s\n' "$GOV" > "$R/go.mod"
cat > "$R/a/a.go" <<'EOF'
package a

import "embed"

//go:embed assets/*.yaml
var Conf embed.FS
EOF
printf 'key: one\n' > "$R/a/assets/conf.yaml"
printf 'key: two\n' > "$R/a/assets/other.yaml"
printf 'package assets\n' > "$R/a/assets/assets.go"
cat > "$R/b/b.go" <<'EOF'
package b

import "example.invalid/fx/a"

func Conf() ([]byte, error) { return a.Conf.ReadFile("assets/other.yaml") }
EOF
printf 'package c\n' > "$R/c/c.go"
printf 'input\n' > "$R/c/testdata/in.txt"
printf 'package d\n' > "$R/d/d.go"
cat > "$R/d/d_test.go" <<'EOF'
package d

import (
	"os"
	"testing"
)

func TestGuide(t *testing.T) {
	_, _ = os.ReadFile("../c/testdata/in.txt")
	if _, err := os.ReadFile("../docs/guide.md"); err != nil {
		t.Fatal(err)
	}
}
EOF
printf 'package e\n' > "$R/e/e.go"
cat > "$R/f/f.go" <<'EOF'
package f

import _ "example.invalid/fx/d"
EOF
printf '# guide\n' > "$R/docs/guide.md"
printf 'todo\n' > "$R/notes/todo.txt"
(cd "$R" && go test ./...)
git -C "$R" init -q
git -C "$R" add -A
git -C "$R" commit -qm base
git -C "$R" tag base

# reset_fixture: back to the base commit with a clean tree.
reset_fixture() {
  git -C "$R" checkout -q -f base 2>/dev/null
  git -C "$R" clean -fdq
}

# ---- awk shims -------------------------------------------------------------
AWKS=""
add_awk() { # add_awk <label> <command...>
  local label="$1"; shift
  mkdir -p "$WORK/awk-$label"
  { printf '#!/bin/sh\nexec'; printf ' %s' "$@"; printf ' "$@"\n'; } > "$WORK/awk-$label/awk"
  chmod +x "$WORK/awk-$label/awk"
  AWKS="$AWKS $label"
}
SYS_AWK="$(command -v awk)"
add_awk system "$SYS_AWK"
for a in gawk mawk nawk original-awk; do
  p="$(command -v "$a" 2>/dev/null || true)"
  [ -n "$p" ] && [ "$p" != "$SYS_AWK" ] && add_awk "$a" "$p"
done
if command -v busybox >/dev/null 2>&1 && busybox awk 'BEGIN{}' >/dev/null 2>&1; then
  add_awk busybox "$(command -v busybox)" awk
fi

# ---- runner ----------------------------------------------------------------
FAILED=0
# check <awk> <script> <case> <want-exit> <want-stdout> <change...>
# A change is "edit:<path>" (append a line), "new:<path>", "stage" or "commit".
check() {
  local awk="$1" script="$2" name="$3" want_rc="$4" want="$5"; shift 5
  reset_fixture
  local c
  for c in "$@"; do
    case "$c" in
      edit:*) printf '// changed\n' >> "$R/${c#edit:}" ;;
      new:*) mkdir -p "$(dirname "$R/${c#new:}")"; printf 'new\n' > "$R/${c#new:}" ;;
      delete:*) rm "$R/${c#delete:}" ;;
      stage) git -C "$R" add -A ;;
      commit) git -C "$R" add -A && git -C "$R" commit -qm change ;;
    esac
  done
  local ref=base
  [ "$name" = bad-ref ] && ref=no-such-ref
  local got rc=0
  got="$(cd "$R" && PATH="$WORK/awk-$awk:$PATH" bash "$script" "$ref" 2>"$WORK/err")" || rc=$?
  got="$(printf '%s' "$got" | tr '\n' ' ' | sed 's/ *$//')"
  if [ "$rc" != "$want_rc" ] || [ "$got" != "$want" ]; then
    printf 'FAIL %s %s: exit %s want %s; got [%s] want [%s]; stderr: %s\n' \
      "$awk" "$name" "$rc" "$want_rc" "$got" "$want" "$(tr '\n' ' ' < "$WORK/err")" >&2
    FAILED=1
    return 1
  fi
  printf 'ok %s %s\n' "$awk" "$name"
}

P=example.invalid/fx
cases() { # cases <awk> <script>
  local w="$1" s="$2"
  check "$w" "$s" go-file 0 "$P/a $P/b" edit:a/a.go commit
  check "$w" "$s" go-file-uncommitted 0 "$P/a $P/b" edit:a/a.go
  check "$w" "$s" go-file-staged 0 "$P/a $P/b" edit:a/a.go stage
  check "$w" "$s" embedded-yaml 0 "$P/a $P/a/assets $P/b" edit:a/assets/conf.yaml commit
  check "$w" "$s" testdata 0 "$P/c $P/d" new:c/testdata/more.txt
  check "$w" "$s" owned-data-reader 0 "$P/c $P/d" edit:c/testdata/in.txt
  check "$w" "$s" deleted-embedded-yaml 0 "$P/a $P/a/assets $P/b" delete:a/assets/conf.yaml
  check "$w" "$s" docs-read-by-test 0 "$P/d" edit:docs/guide.md commit
  check "$w" "$s" go-file-importer 0 "$P/d $P/f" edit:d/d.go
  check "$w" "$s" two-dirs 0 "$P/a $P/a/assets $P/b $P/c" edit:c/c.go edit:a/assets/conf.yaml commit
  check "$w" "$s" package-free 0 "" edit:notes/todo.txt commit
  check "$w" "$s" nothing-changed 0 ""
  check "$w" "$s" go-mod 0 "./..." edit:go.mod commit
  check "$w" "$s" bad-ref 2 ""
}

# strict_awk <label>: "yes" when this awk rejects a newline inside a -v value
# (BSD awk does: the D-0269/D-0290 failure), so the run proves the script
# survives it.
strict_awk() {
  if PATH="$WORK/awk-$1:$PATH" awk -v x="$(printf 'a\nb')" 'BEGIN { exit 0 }' >/dev/null 2>&1; then
    echo no
  else
    echo yes
  fi
}

TESTED=""
STRICT=0
NAWKS=0
for w in $AWKS; do
  NAWKS=$((NAWKS + 1))
  st="$(strict_awk "$w")"
  [ "$st" = yes ] && STRICT=$((STRICT + 1))
  cases "$w" "$SCRIPT" || true
  TESTED="$TESTED $w(newline-in--v-rejected=$st)"
done
[ "$FAILED" = 0 ] || { echo "AFFECTED-PKGS-TEST: FAIL" >&2; exit 1; }

# ---- positive control --------------------------------------------------------
# A copy that ignores go:embed must list a/assets alone (exit 0) for the
# embedded-yaml change: the exact wrong answer, not just any mismatch, so a
# copy that merely errors cannot pass the control.
sed 's/if (f in embedded) {/if (0) {/' "$SCRIPT" > "$WORK/broken.sh"
cmp -s "$SCRIPT" "$WORK/broken.sh" && { echo "FAIL positive control: the embed check was not found to break" >&2; exit 1; }
if ! check system "$WORK/broken.sh" embedded-yaml 0 "$P/a/assets" edit:a/assets/conf.yaml commit; then
  echo "FAIL positive control: a script without the embed mapping did not give the expected wrong answer" >&2
  exit 1
fi
echo "ok positive-control (without the embed mapping the embedded-yaml case lists a/assets only)"

if [ "$STRICT" = 0 ]; then
  echo "note: no awk here rejects a newline in -v (BSD awk does); run on macOS for that class"
fi
if [ "$NAWKS" -lt 2 ]; then
  echo "note: only one awk ran; install gawk or mawk for a second flavour"
fi

echo "awks tested:$TESTED"
echo "AFFECTED-PKGS-TEST: PASS"
