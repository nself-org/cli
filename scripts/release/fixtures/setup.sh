# shellcheck shell=bash
# Purpose: build one isolated release.sh test environment (sourced by test.sh).
# Inputs:  ROOT (worktree root), RELDIR, FIXTURES, BASE (scratch dir), HAVE_GO, ORIG_PATH
# Outputs: globals T FIX ST REL; bare fixture repos cli/admin/homebrew-nself/web, a
#          copy of scripts/release next to a STUB admin-merge.sh,
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
  printf 'on:\n  push:\n    tags:\n      - "v*"\n  workflow_dispatch:\n    inputs:\n      version:\n        required: false\n' | put .github/workflows/sdk-publish-go.yml
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
  export GH_LOG=$T/gh.log CURL_LOG=$T/curl.log NODE_LOG=$T/node.log ADMIN_LOG=$T/admin.log
  export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@example.invalid GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@example.invalid
  export PATH="$FIXTURES/bin:$ORIG_PATH"
  : > "$GH_LOG"; : > "$CURL_LOG"; : > "$NODE_LOG"; : > "$ADMIN_LOG"
  echo v1.4.13 > "$ST/latest"
  mkdir -p "$T/tree/scripts/release"
  cp -R "$RELDIR/release.sh" "$RELDIR/lib" "$T/tree/scripts/release/"
  cp "$FIXTURES/admin-merge.sh" "$T/tree/scripts/"
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

# bump_mains [admin|cli]: commit the 1.5.0 bump straight onto admin and/or cli main
# (as if S2/S3 had merged). No PR record is made, so S4/S9 take their no-PR path.
bump_mains() {
  local r
  for r in ${1:-admin cli}; do
    git clone -q "$FIX/$r.git" "$T/bm-$r"
    if [ "$r" = admin ]; then
      echo '{"name":"admin","version":"1.5.0"}' > "$T/bm-$r/package.json"
    else
      echo 1.5.0 > "$T/bm-$r/.github/VERSION"
    fi
    git -C "$T/bm-$r" commit -q -am bump && git -C "$T/bm-$r" push -q origin HEAD:main
  done
}

# fake_merged_pr <repo> <branch> <sha>: a merged PR record, as admin-merge would leave.
fake_merged_pr() {
  local n=$(( $(cat "$ST/prcount" 2>/dev/null || echo 0) + 1 ))
  echo "$n" > "$ST/prcount"; mkdir -p "$ST/pr/$1"
  printf '%s' "$2" > "$ST/pr/$1/$n.head"; echo MERGED > "$ST/pr/$1/$n.state"; echo "$3" > "$ST/pr/$1/$n.sha"
}

# bump_brew [good]: the homebrew formula on main already carries 1.5.0; with "good"
# also the fixture checksums' hashes, otherwise the old 1.4.13 hashes.
bump_brew() {
  local f="$T/bm-brew/Formula/nself.rb"
  git clone -q "$FIX/homebrew-nself.git" "$T/bm-brew"
  sed 's/version "1.4.13"/version "1.5.0"/' "$f" > "$f.new"; mv "$f.new" "$f"
  if [ "${1:-}" = good ]; then
    sed -e 's/73432f349295c10e6f28621d92fdc7736284ecb8287b5df697f98d5289acd138/554a93c77f0a4d50fabbdc3ed7bd01638f092ce082f2d54d866d659d662bfd77/' \
      -e 's/eda10ee8bd97ae6707c92bef4a77624fa26883e5bea39efb2c413c18cf2aa809/6f823864cb70bb06e0d87f751db0fafcf96375da658da2b281b0fb1d9a4deeba/' "$f" > "$f.new"
    mv "$f.new" "$f"
  fi
  git -C "$T/bm-brew" commit -q -am bump && git -C "$T/bm-brew" push -q origin HEAD:main
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
