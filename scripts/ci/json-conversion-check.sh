#!/usr/bin/env bash
# Compare one canon fragment's human output with merge-base and validate its
# v1.5 JSON envelopes. A fixture project, HOME and docker stub isolate runs.
set -euo pipefail
cd "$(dirname "$0")/../.."
repo_root="$PWD"
command -v jq >/dev/null || { echo 'FAIL jq missing' >&2; exit 1; }
[ "$#" -eq 1 ] || { echo 'usage: json-conversion-check.sh <fragment>' >&2; exit 1; }
fragment="$1"
argv_file="${JCC_ARGV_FILE:-cmd/commands/testdata/json/argv/$fragment.txt}"
[ -s "$argv_file" ] || { echo "FAIL missing or empty argv file: $argv_file" >&2; exit 1; }

tmp="$(mktemp -d "${TMPDIR:-/tmp}/json-conversion.XXXXXX")"
base_tree="$tmp/base-tree"
has_worktree=0
cleanup() {
  if [ "$has_worktree" -eq 1 ]; then git worktree remove --force "$base_tree" >/dev/null 2>&1 || true; fi
  rm -rf "$tmp"
}
trap cleanup EXIT
mkdir -p "$tmp/project" "$tmp/home" "$tmp/stub"
printf 'PROJECT_NAME=smoke\nENV=dev\nBASE_DOMAIN=example.test\nPOSTGRES_PASSWORD=smoke-fixture-value\nHASURA_GRAPHQL_JWT_SECRET=fixture\n' > "$tmp/project/.env"
cat > "$tmp/stub/docker" <<'DOCKER'
#!/bin/sh
case " $* " in
  ' info '*) exit 0 ;;
  ' compose version '*) printf '2.30.0\n'; exit 0 ;;
  ' inspect '*) printf 'healthy\n'; exit 0 ;;
  *' ps --format json '*) printf '%s\n' '[{"Name":"smoke_postgres","Service":"postgres","State":"running","Health":"healthy","Ports":"127.0.0.1:5000->5000/tcp"},{"Name":"smoke_hasura","Service":"hasura","State":"running","Health":"healthy"},{"Name":"smoke_auth","Service":"auth","State":"running","Health":"healthy"},{"Name":"smoke_nginx","Service":"nginx","State":"running","Health":"healthy"}]'; exit 0 ;;
esac
exit 0
DOCKER
chmod +x "$tmp/stub/docker"

head_bin="${JCC_HEAD_BIN:-$tmp/nself-head}"
base_bin="${JCC_BASE_BIN:-$tmp/nself-base}"
if [ -z "${JCC_HEAD_BIN:-}" ]; then
  lockf -k /tmp/nself-cli-compile.lock env CGO_ENABLED=0 go build -mod=vendor -buildvcs=false -o "$head_bin" ./cmd/nself
fi
if [ -z "${JCC_BASE_BIN:-}" ]; then
  git worktree add --detach "$base_tree" "$(git merge-base HEAD origin/main)" >/dev/null
  has_worktree=1
  (cd "$base_tree" && lockf -k /tmp/nself-cli-compile.lock env CGO_ENABLED=0 go build -mod=vendor -buildvcs=false -o "$base_bin" ./cmd/nself)
fi
lockf -k /tmp/nself-cli-compile.lock env CGO_ENABLED=0 go build -mod=vendor -buildvcs=false -o "$tmp/jsonvalidate" ./tools/jsonvalidate

# run_one <binary> <mode> <suffix> <stem> <argv...> records stdout, stderr and rc.
run_one() {
  local bin="$1" mode="$2" suffix="$3" stem="$4"; shift 4
  local rc=0
  local extra=() args=("$@")
  [ -n "$mode" ] && extra+=("NSELF_V15=$mode")
  [ -n "$suffix" ] && args+=("$suffix")
  (cd "$tmp/project" && env -u NSELF_V15 HOME="$tmp/home" AI_AUTO_INSTALL=false PATH="$tmp/stub:$PATH" \
    NSELF_NONINTERACTIVE=1 NO_COLOR=1 CI=1 "${extra[@]}" "$bin" "${args[@]}") \
    >"$stem.out" 2>"$stem.err" || rc=$?
  printf '%s\n' "$rc" > "$stem.rc"
}

pass=0 fail=0
while IFS= read -r raw || [ -n "$raw" ]; do
  raw="${raw%$'\r'}"
  [ -z "$raw" ] && continue
  [[ "$raw" == \#* ]] && continue
  command_text="${raw%%#|*}"
  tags=""
  if [[ "$raw" == *'#|'* ]]; then tags="${raw#*#|}"; fi
  read -r -a argv <<< "$command_text"
  [ "${#argv[@]}" -gt 0 ] || { echo "FAIL empty argv row"; fail=$((fail+1)); continue; }
  label="${argv[*]}"
  bad=0
  for mode in v14 v15; do
    value=""
    [ "$mode" = v15 ] && value=1
    run_one "$base_bin" "$value" "" "$tmp/base-$mode" "${argv[@]}"
    run_one "$head_bin" "$value" "" "$tmp/head-$mode" "${argv[@]}"
    if [[ "$tags" != *human-skip=* ]]; then
      human_left="$tmp/base-$mode.out"; human_right="$tmp/head-$mode.out"
      if [ "$label" = doctor ]; then
        sed -E 's/(Disk space: )[0-9.]+ GB free/\1<free> GB free/' "$human_left" > "$tmp/base-$mode-stable.out"
        sed -E 's/(Disk space: )[0-9.]+ GB free/\1<free> GB free/' "$human_right" > "$tmp/head-$mode-stable.out"
        human_left="$tmp/base-$mode-stable.out"; human_right="$tmp/head-$mode-stable.out"
      fi
      # Go's default slog timestamps differ between the two sequential runs.
      for side in base head; do
        sed -E 's|^[0-9]{4}/[0-9]{2}/[0-9]{2} [0-9]{2}:[0-9]{2}:[0-9]{2} (INFO ENV resolved for \.env cascade)|<time> \1|' \
          "$tmp/$side-$mode.err" > "$tmp/$side-$mode-stable.err"
      done
      if ! cmp -s "$human_left" "$human_right" || ! cmp -s "$tmp/base-$mode-stable.err" "$tmp/head-$mode-stable.err" || ! cmp -s "$tmp/base-$mode.rc" "$tmp/head-$mode.rc"; then
        echo "FAIL $label: $mode human stdout or exit changed, or stderr changed"; bad=1
        diff -u "$human_left" "$human_right" >&2 || true
        diff -u "$tmp/base-$mode-stable.err" "$tmp/head-$mode-stable.err" >&2 || true
        printf 'exit base=%s head=%s\n' "$(cat "$tmp/base-$mode.rc")" "$(cat "$tmp/head-$mode.rc")" >&2
      fi
    fi
  done
  if [[ "$tags" == *legacy-json* ]]; then
    run_one "$base_bin" "" "--json" "$tmp/base-legacy" "${argv[@]}"
    run_one "$head_bin" "" "--json" "$tmp/head-legacy" "${argv[@]}"
    if [[ "$tags" == *volatile-json=timestamp* || "$label" == health ]]; then
      # Replace only the top-level timestamp value; preserve every other byte.
      sed -E 's/^(  "timestamp": )"[^"]*"/\1"<timestamp>"/' "$tmp/base-legacy.out" > "$tmp/base-stable.json"
      sed -E 's/^(  "timestamp": )"[^"]*"/\1"<timestamp>"/' "$tmp/head-legacy.out" > "$tmp/head-stable.json"
      left="$tmp/base-stable.json"; right="$tmp/head-stable.json"
    else
      left="$tmp/base-legacy.out"; right="$tmp/head-legacy.out"
    fi
    if ! cmp -s "$left" "$right" || ! cmp -s "$tmp/base-legacy.rc" "$tmp/head-legacy.rc"; then
      echo "FAIL $label: v1.4 --json stdout or exit changed"; bad=1
      diff -u "$left" "$right" >&2 || true
    fi
  fi
  if [[ "$tags" != *no-json=* ]]; then
    run_one "$head_bin" 1 "--json" "$tmp/head-json" "${argv[@]}"
    if [ "$(cat "$tmp/head-json.rc")" -ne 0 ]; then
      echo "FAIL $label: v1.5 --json exited non-zero"; bad=1
    fi
    if ! jq -e -s 'length == 1 and (.[0] | type == "object" and has("data") and (has("error") | not))' "$tmp/head-json.out" >/dev/null 2>&1; then
      echo "FAIL $label: stdout is not one successful data envelope"; bad=1
    elif ! (cd "$repo_root" && "$tmp/jsonvalidate" -schema envelope.v1.schema.json -data-from-registry "$label" "$tmp/head-json.out") >"$tmp/validator.out" 2>&1; then
      echo "FAIL $label: envelope or data schema invalid"; bad=1
      cat "$tmp/validator.out" >&2
    fi
  fi
  if [ "$bad" -eq 0 ]; then echo "PASS $label"; pass=$((pass+1)); else fail=$((fail+1)); fi
done < "$argv_file"
[ "$pass" -gt 0 ] && [ "$fail" -eq 0 ] || { echo "FAIL summary: $pass passed, $fail failed"; exit 1; }
echo "PASS summary: $pass rows"
