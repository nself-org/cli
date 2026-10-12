# shellcheck shell=bash
# Purpose: build one isolated release.sh test environment (sourced by test.sh).
# Inputs:  ROOT (worktree root), RELDIR, FIXTURES, BASE (scratch dir), HAVE_GO, ORIG_PATH
# Outputs: globals T FIX ST REL; bare fixture repos cli/admin/homebrew-nself/web, a
#          copy of scripts/release next to STUB admin-merge.sh / release-create.sh,
#          stub gh/curl/sleep/node on PATH, a clone $T/cwd of cli to run from.
# shellcheck disable=SC2034
# Constraints: no network, no real GitHub; everything lives under $BASE.

seed_begin() { SD=$T/seed/$1; mkdir -p "$SD"; git init -q "$SD"; git -C "$SD" symbolic-ref HEAD refs/heads/main; }
seed_end() {
  git -C "$SD" add -A
  git -C "$SD" commit -q -m seed
  git init -q --bare "$FIX/$1.git"
  git -C "$FIX/$1.git" symbolic-ref HEAD refs/heads/main
  git -C "$SD" push -q "$FIX/$1.git" main
}
put() { mkdir -p "$(dirname "$SD/$1")"; cat > "$SD/$1"; }

seed_cli() { # <with-facts 0|1>
  seed_begin cli
  echo 1.4.13 | put .github/VERSION
  printf 'package version\n\nvar (\n\tVersion string = "1.4.13"\n)\n' | put internal/version/version.go
  printf 'package sdk\n\nconst Version = "1.4.13"\n' | put sdk/go/doc.go
  printf '{"name":"a","version":"1.4.13"}\n' | put sdk/ts/package.json
  printf '{"name":"b","version":"1.4.13"}\n' | put sdk/ts-sdk/package.json
  printf '[project]\nname = "x"\nversion = "1.4.13"\n' | put sdk/py/pyproject.toml
  printf '# cli\n\ncli version: 1.4.13\n' | put README.md
  mkdir -p "$SD/scripts"; cp "$ROOT/scripts/bump-version.sh" "$SD/scripts/bump-version.sh"
  for f in sdk-ts-publish sdk-py-publish; do
    printf 'on:\n  push:\n    tags:\n      - "v*"\n  workflow_dispatch:\n    inputs:\n      version:\n        required: false\n' | put ".github/workflows/$f.yml"
  done
  printf 'on:\n  push:\n    tags:\n      - "v*"\n' | put .github/workflows/sdk-manual-publish.yml
  if [ "$1" = 1 ]; then
    mkdir -p "$SD/tools" "$SD/vendor"; cp -R "$FIXTURES/facts" "$SD/tools/facts"; : > "$SD/vendor/modules.txt"
    printf 'module fixture\n\ngo 1.21\n' | put go.mod
    rm -f "$SD/tools/facts/go.mod"
  fi
  seed_end cli
}

mk_env() { # <with-facts 0|1>
  T=$(mktemp -d "$BASE/env.XXXXXX")
  FIX=$T/fix; ST=$T/st; mkdir -p "$FIX" "$ST" "$T/home" "$T/tmp" "$T/tree/scripts"
  export HOME=$T/home FIX ST FIXTURES TMPDIR=$T/tmp GOTOOLCHAIN=local GOCACHE=$BASE/gocache GOPATH=$BASE/gopath
  export GH_LOG=$T/gh.log CURL_LOG=$T/curl.log NODE_LOG=$T/node.log ADMIN_LOG=$T/admin.log RC_LOG=$T/rc.log
  export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@example.invalid GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@example.invalid
  export PATH="$FIXTURES/bin:$ORIG_PATH"
  : > "$GH_LOG"; : > "$CURL_LOG"; : > "$NODE_LOG"; : > "$ADMIN_LOG"; : > "$RC_LOG"
  echo v1.4.13 > "$ST/latest"
  mkdir -p "$T/tree/scripts/release"
  cp -R "$RELDIR/release.sh" "$RELDIR/lib" "$T/tree/scripts/release/"
  cp "$FIXTURES/admin-merge.sh" "$FIXTURES/release-create.sh" "$T/tree/scripts/"
  REL=$T/tree/scripts/release/release.sh
  seed_cli "$1"
  seed_begin admin
  echo '{"name":"admin","version":"1.4.13"}' | put package.json
  echo "export const CLI_VERSION = '1.4.13'" | put src/lib/cli-version.ts
  printf 'ARG NSELF_VERSION=1.4.13\nENV ADMIN_VERSION=1.4.13\nLABEL org.opencontainers.image.version="1.4.13"\n' | put Dockerfile
  seed_end admin
  seed_begin homebrew-nself
  mkdir -p "$SD/Formula" "$SD/.github/scripts"
  tail -n +2 "$FIXTURES/nself.rb" > "$SD/Formula/nself.rb"
  cp "$FIXTURES/update-formula.sh" "$SD/.github/scripts/update-formula.sh"
  seed_end homebrew-nself
  seed_begin web
  echo '{"cli":"v1.4.13"}' | put org/sources.lock.json
  seed_end web
  git clone -q "$FIX/cli.git" "$T/cwd"
}

# snap: one checksum over every ref of every fixture repo.
snap() { for r in cli admin homebrew-nself web; do git --git-dir="$FIX/$r.git" for-each-ref; done | cksum; }
# show <repo> <path>: file on the fixture repo's main.
show() { git --git-dir="$FIX/$1.git" show "main:$2"; }
# formula_hash <arm|intel>: sha256 inside that block of the homebrew formula on main.
formula_hash() {
  show homebrew-nself Formula/nself.rb | awk -v want="$1" '
    /on_arm do/ { b = "arm" } /on_intel do/ { b = "intel" } /^[[:space:]]*end[[:space:]]*$/ { b = "" }
    b == want && /sha256/ { sub(/^[^"]*"/, ""); sub(/".*$/, ""); print; exit }'
}

run_plain() { # <args...>: stdin is /dev/null (not a TTY)
  RC=0
  (cd "$T/cwd" && bash "$REL" "$@") < /dev/null > "$T/out" 2> "$T/err" || RC=$?
}

# run_pty <args...>: run under a pseudo-terminal; 'y' lines are typed after startup.
run_pty() {
  local cmd full
  rm -f "$T/rc"
  cmd=$(printf '%q ' bash "$REL" "$@")
  full="cd $(printf '%q' "$T/cwd") && $cmd; echo \$? > $(printf '%q' "$T/rc")"
  if [ "$(uname)" = Darwin ]; then
    ( sleep 2; printf 'y\ny\ny\ny\ny\n'; sleep 1 ) | script -q /dev/null bash -c "$full" > "$T/out" 2>&1 || true
  else
    ( sleep 2; printf 'y\ny\ny\ny\ny\n'; sleep 1 ) | script -qec "$full" /dev/null > "$T/out" 2>&1 || true
  fi
  RC=$(cat "$T/rc" 2>/dev/null || echo none)
}
