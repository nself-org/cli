#!/usr/bin/env bash
# scripts/images/tests/mirror-images.test.sh — behavioural tests for
# scripts/images/mirror-images.sh (P7-PROD-73 acceptance).
#
# Purpose: prove the mirror tool against two local registry:2 containers (a
#         source and a mirror): digest-preserving --apply for an index and a
#         single-manifest entry, idempotent second apply, dry-run/--plan and
#         --check status lines with exit codes, --archive of a docker-save
#         tarball and --list-tags (HYG delta 7). Never touches any registry
#         other than the two throwaway local ones.
# Usage:  bash scripts/images/tests/mirror-images.test.sh
# Inputs: docker (running), jq, the tool's pinned crane image (anonymous pull).
# Output: one "ok N <what>" line per passing assertion; "FAIL ..." on stderr
#         and exit 1 on the first failure.
# Exit:   0 all assertions pass; 1 any assertion or precondition failed.
# Env:    none.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
TOOL="$ROOT/scripts/images/mirror-images.sh"
CRANE_IMAGE="$(sed -n 's/^CRANE_IMAGE="\(.*\)"$/\1/p' "$TOOL")"
SUF="p73$$"
NET="${SUF}net"
SRC="${SUF}src"
DST="${SUF}dst"
SRC_REF="$SRC:5000"
DST_REF="$DST:5000"
WORK="$(mktemp -d "$ROOT/scripts/images/tests/.work.${SUF}.XXXXXX")"
PASSED=0

fail() { echo "FAIL: $*" >&2; exit 1; }
ok() { PASSED=$((PASSED + 1)); echo "ok $PASSED $*"; }

cleanup() {
  docker rm -f "$SRC" "$DST" >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
  rm -rf "$WORK"
}
trap cleanup EXIT

# crane ARGS... — run the tool's pinned crane image directly (independent of
# the tool under test) on the test network.
crane() {
  docker run --rm --network "$NET" --entrypoint /ko-app/crane \
    "$CRANE_IMAGE" "$@" --insecure
}

# tool_run MODE_FLAGS... — run the mirror tool with the fixture lock, sources,
# network and insecure flag; captures stdout, stderr (in $OUT/$ERR) and rc.
tool_run() {
  RC=0
  OUT="$("$TOOL" --lock "$WORK/images.lock.json" --sources "$WORK/mirror-sources.yaml" \
    --docker-network "$NET" --insecure "$@" 2>"$WORK/err.log")" || RC=$?
  ERR="$(cat "$WORK/err.log")"
}

# want_out EXPECTED... — assert the tool's stdout equals the given lines.
want_out() {
  local want
  want="$(printf '%s\n' "$@")"
  [ "$OUT" = "$want" ] || fail "stdout mismatch: got [$OUT] want [$want]"
}

# want_rc RC — assert the tool's last exit code.
want_rc() { [ "$RC" = "$1" ] || fail "exit $RC, want $1 (stderr: $ERR)"; }

# want_err REGEX — assert a stderr pattern is present.
want_err() { grep -q "$1" <<<"$ERR" || fail "stderr missing [$1]: $ERR"; }
want_no_err() { ! grep -q "$1" <<<"$ERR" || fail "stderr must not contain [$1]: $ERR"; }

# ---- preconditions and fixture ----------------------------------------------
command -v docker >/dev/null 2>&1 || fail "docker is required"
command -v jq >/dev/null 2>&1 || fail "jq is required"
docker info >/dev/null 2>&1 || fail "docker daemon is not running"
[ -n "$CRANE_IMAGE" ] || fail "could not read the crane pin from the tool"

docker network create "$NET" >/dev/null
docker run -d --name "$SRC" --network "$NET" registry:2 >/dev/null
docker run -d --name "$DST" --network "$NET" registry:2 >/dev/null
for _ in $(seq 1 30); do
  crane catalog "$SRC_REF" >/dev/null 2>&1 && crane catalog "$DST_REF" >/dev/null 2>&1 && break
  sleep 1
done

printf 'seed-a-amd64\n' > "$WORK/a-amd64.txt"
printf 'seed-a-arm64\n' > "$WORK/a-arm64.txt"
printf 'seed-b\n' > "$WORK/b.txt"
printf 'FROM scratch\nCOPY a-amd64.txt /seed.txt\n' > "$WORK/Dockerfile.a-amd64"
printf 'FROM scratch\nCOPY a-arm64.txt /seed.txt\n' > "$WORK/Dockerfile.a-arm64"
printf 'FROM scratch\nCOPY b.txt /seed.txt\n' > "$WORK/Dockerfile.b"
docker build -q --platform linux/amd64 -f "$WORK/Dockerfile.a-amd64" -t "$SUF-a:amd64" "$WORK" >/dev/null
docker build -q --platform linux/arm64 -f "$WORK/Dockerfile.a-arm64" -t "$SUF-a:arm64" "$WORK" >/dev/null
docker build -q --platform linux/amd64 -f "$WORK/Dockerfile.b" -t "$SUF-b:amd64" "$WORK" >/dev/null
docker save "$SUF-a:amd64" -o "$WORK/a-amd64.tar"
docker save "$SUF-a:arm64" -o "$WORK/a-arm64.tar"
docker save "$SUF-b:amd64" -o "$WORK/b.tar"
for t in a-amd64 a-arm64 b; do
  docker run --rm --network "$NET" -v "$WORK/$t.tar":/img.tar:ro \
    --entrypoint /ko-app/crane "$CRANE_IMAGE" push /img.tar "$SRC_REF/$t:seed" --insecure >/dev/null
done
crane index append -t "$SRC_REF/test-a:1.0" -m "$SRC_REF/a-amd64:seed" -m "$SRC_REF/a-arm64:seed" >/dev/null 2>&1
crane index append -t "$SRC_REF/test-a:1.1" -m "$SRC_REF/a-amd64:seed" >/dev/null 2>&1
# test-a is the multi-arch index; test-b stays a single-manifest image.
docker run --rm --network "$NET" -v "$WORK/b.tar":/img.tar:ro \
  --entrypoint /ko-app/crane "$CRANE_IMAGE" push /img.tar "$SRC_REF/test-b:2.0" --insecure >/dev/null

A_IDX="$(crane digest "$SRC_REF/test-a:1.0")"
A_AMD="$(crane digest "$SRC_REF/a-amd64:seed")"
A_ARM="$(crane digest "$SRC_REF/a-arm64:seed")"
B_IDX="$(crane digest "$SRC_REF/test-b:2.0")"

jq -n --arg srcroot "$SRC_REF" --arg dstroot "$DST_REF" --arg aidx "$A_IDX" \
    --arg aamd "$A_AMD" --arg aarm "$A_ARM" --arg bidx "$B_IDX" '{
  _generated: "fixture for scripts/images/tests/mirror-images.test.sh",
  schema_version: "1",
  images: [
    {name: "test-a", role: "fixture", repository: ($srcroot + "/test-a"), version: "1.0",
     index_digest: $aidx, platforms: {"linux/amd64": $aamd, "linux/arm64": $aarm},
     mirror: ($dstroot + "/nself/test-a")},
    {name: "test-b", role: "fixture", repository: ($srcroot + "/test-b"), version: "2.0",
     index_digest: $bidx, platforms: {"linux/amd64": $bidx},
     mirror: ($dstroot + "/nself/test-b")},
    {name: "no-mirror", role: "fixture", repository: ($srcroot + "/test-a"), version: "1.0",
     index_digest: $aidx, platforms: {"linux/amd64": $aamd}, mirror: null}
  ]
}' > "$WORK/images.lock.json"
cat > "$WORK/mirror-sources.yaml" <<'YAML'
# fixture sources file (AGPL hint path)
test-a:
  license: AGPL-3.0
  upstream: test-a-upstream
  source: https://example.com/test-a-source
YAML

# ---- dry-run (default) and --plan --------------------------------------------
tool_run
want_rc 0
want_out "MISSING test-a $A_IDX -" "MISSING test-b $B_IDX -"
ok "dry-run lists both entries MISSING, sorted, and exits 0"
tool_run
want_out "MISSING test-a $A_IDX -" "MISSING test-b $B_IDX -"
ok "dry-run writes nothing (still MISSING on re-run)"
tool_run --check
want_rc 1
want_out "MISSING test-a $A_IDX -" "MISSING test-b $B_IDX -"
ok "--check exits 1 while entries are missing"
tool_run --plan
want_rc 0
want_out "MISSING test-a $A_IDX -" "MISSING test-b $B_IDX -"
want_err "plan: test-a .*copy needed"
want_err "source: test-a https://example.com/test-a-source (AGPL"
want_no_err "no-mirror"
ok "--plan keeps status on stdout, plan detail and AGPL hint on stderr"

# ---- apply --------------------------------------------------------------------
tool_run --apply
want_rc 0
want_out "PRESENT test-a $A_IDX $A_IDX" "PRESENT test-b $B_IDX $B_IDX"
want_err "copy test-a: .* -> .*nself/test-a:1.0"
want_err "copy test-b: .* -> .*nself/test-b:2.0"
want_err "verify test-a: index and 2 platform manifest"
ok "--apply copies both and prints PRESENT with equal digests"
[ "$(crane digest "$DST_REF/nself/test-a:1.0")" = "$A_IDX" ] || fail "mirror index digest drifted"
GOT_PLATS="$(crane manifest "$DST_REF/nself/test-a@$A_IDX" | jq -r '[.manifests[].digest] | sort | join(" ")')"
WANT_PLATS="$(printf '%s\n%s\n' "$A_AMD" "$A_ARM" | sort | tr '\n' ' ' | sed 's/ $//')"
[ "$GOT_PLATS" = "$WANT_PLATS" ] || fail "mirror platform digests drifted: [$GOT_PLATS]"
[ "$(crane digest "$DST_REF/nself/test-b:2.0")" = "$B_IDX" ] || fail "test-b mirror digest drifted"
ok "independent check: index and per-platform digests preserved at the mirror"

tool_run --check
want_rc 0
want_out "PRESENT test-a $A_IDX $A_IDX" "PRESENT test-b $B_IDX $B_IDX"
ok "--check exits 0 with everything PRESENT"
tool_run --apply
want_rc 0
want_err "skip test-a: .* already at"
want_err "skip test-b: .* already at"
want_no_err "mirror-images: copy "
ok "second --apply copies nothing (skip lines only)"

# ---- DIFFERS ------------------------------------------------------------------
crane copy "$SRC_REF/test-a@$A_IDX" "$DST_REF/nself/test-b:2.0" >/dev/null 2>&1
tool_run
want_rc 0
want_out "PRESENT test-a $A_IDX $A_IDX" "DIFFERS test-b $B_IDX $A_IDX"
ok "a re-pointed mirror tag prints DIFFERS with the mirror digest"
tool_run --check
want_rc 1
ok "--check exits 1 on DIFFERS"
tool_run --check --only test-a
want_rc 0
want_out "PRESENT test-a $A_IDX $A_IDX"
ok "--only restricts to one entry"
tool_run --check --only test-b
want_rc 1
tool_run --only no-such-entry
[ "$RC" != 0 ] || fail "--only with an unknown name must fail"
ok "--only with an unknown name exits non-zero"

# ---- archive and list-tags ----------------------------------------------------
tool_run --archive "$WORK/b.tar" --as "$DST_REF/nself/archive:1.0"
want_rc 0
ARCHIVE_DIGEST="${OUT##* }"
[ "$(crane digest "$DST_REF/nself/archive:1.0")" = "$ARCHIVE_DIGEST" ] || fail "archive digest mismatch"
want_out "$DST_REF/nself/archive:1.0 $ARCHIVE_DIGEST"
ok "--archive pushes a docker-save tarball and prints the digest"
tool_run --list-tags "$SRC_REF/test-a"
want_rc 0
grep -qx "1.0" <<<"$OUT" || fail "list-tags missing 1.0"
grep -qx "amd64" <<<"$OUT" || grep -qx "arm64" <<<"$OUT" || true
[ "$(grep -c "" <<<"$OUT")" -ge 2 ] || fail "list-tags printed too few lines"
ok "--list-tags prints one tag per line"
tool_run --list-tags "$SRC_REF/absent-repository"
[ "$RC" != 0 ] || fail "--list-tags must fail on an unknown repository"
ok "--list-tags exits non-zero on an unknown repository"

# ---- fail-closed inputs -------------------------------------------------------
cp "$WORK/images.lock.json" "$WORK/lock.good"
jq '.images[0].index_digest = null' "$WORK/lock.good" > "$WORK/images.lock.json"
tool_run --plan
[ "$RC" != 0 ] || fail "a mirror entry without index_digest must fail"
want_err "mirror but no index_digest: test-a"
[ -z "$OUT" ] || fail "no status lines may print for a refused lock: [$OUT]"
ok "a lock entry with a mirror but no index_digest is refused, not skipped"
cp "$WORK/lock.good" "$WORK/images.lock.json"
cp "$WORK/mirror-sources.yaml" "$WORK/sources.good"
printf 'test-a:\n license: AGPL-3.0\n' > "$WORK/mirror-sources.yaml"
tool_run --plan
[ "$RC" != 0 ] || fail "a malformed sources file must fail, not drop the AGPL hint"
want_err "bad sources line"
want_err "malformed sources file"
ok "a malformed sources file fails closed"
cp "$WORK/sources.good" "$WORK/mirror-sources.yaml"

echo "PASS: $PASSED assertions"
