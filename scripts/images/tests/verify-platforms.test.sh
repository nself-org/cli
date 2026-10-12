#!/usr/bin/env bash
# scripts/images/tests/verify-platforms.test.sh — fixture tests for the
# --apply platform check of scripts/images/mirror-images.sh (D-0350).
#
# Purpose: prove verify_platforms is a subset check: every lock platform
#         digest must appear in the mirror index, and extra entries
#         (BuildKit attestation manifests, platforms the lock does not pin)
#         are accepted. Runs the real tool's --apply path against a fake
#         `docker` on PATH that serves the JSON manifests in fixtures/, so no
#         container, registry or network is touched. Negative controls: a
#         missing lock platform, an empty index, an unreadable manifest and
#         an empty, blank or non-object manifest body must all fail with
#         exit 1 and print no PRESENT line.
# Usage:  bash scripts/images/tests/verify-platforms.test.sh
# Inputs: jq, bash 4+; fixtures/*.json (synthetic digests only).
# Output: one "ok N <what>" line per passing assertion; "FAIL ..." on stderr
#         and exit 1 on the first failure.
# Exit:   0 all assertions pass; 1 any assertion or precondition failed.
# Env:    MIRROR_TOOL (optional) — path of the tool under test; defaults to
#         scripts/images/mirror-images.sh (set it to an older copy to show
#         the tests catch the set-equality regression).
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../../.." && pwd)"
TOOL="${MIRROR_TOOL:-$ROOT/scripts/images/mirror-images.sh}"
FIX="$HERE/fixtures"
CRANE_IMAGE="$(sed -n 's/^CRANE_IMAGE="\(.*\)"$/\1/p' "$TOOL")"
WORK="$(mktemp -d "$HERE/.work.vp$$.XXXXXX")"
PASSED=0

# Synthetic digests: the lock pins amd64=a…a and arm64=b…b under index 9…9;
# fixtures/ reuse the same values (c/d attestations, e/f extra platforms).
hex() { printf "sha256:%s" "$(printf "%64s" "" | tr ' ' "$1")"; }
IDX="$(hex 9)"
AMD="$(hex a)"
ARM="$(hex b)"
MIRROR="mirror.invalid/nself/test-a"
DST="$MIRROR:1.0"

fail() { echo "FAIL: $*" >&2; exit 1; }
ok() { PASSED=$((PASSED + 1)); echo "ok $PASSED $*"; }
cleanup() { rm -rf "$WORK"; }
trap cleanup EXIT

command -v jq >/dev/null 2>&1 || fail "jq is required"
[ -n "$CRANE_IMAGE" ] || fail "could not read the crane pin from the tool"

# ---- fake docker ----------------------------------------------------------------
# Emulates `docker run ... <crane image> <verb> <args>` for the three crane verbs
# --apply uses. digest fails (MISSING) until copy has run, then prints the lock
# index digest; manifest prints $FAKE_MANIFEST ("fail" makes it exit 1). Every
# call is logged so the test can prove nothing else ran.
mkdir -p "$WORK/bin"
cat > "$WORK/bin/docker" <<'SHIM'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$FAKE_STATE/calls.log"
[ "${1:-}" = run ] || { echo "fake docker: only run is emulated" >&2; exit 64; }
while [ $# -gt 0 ] && [ "$1" != "$FAKE_CRANE_IMAGE" ]; do shift; done
[ $# -gt 1 ] || { echo "fake docker: no crane verb" >&2; exit 64; }
shift
case "$1" in
  digest)
    [ -f "$FAKE_STATE/copied" ] || { echo "MANIFEST_UNKNOWN: $2" >&2; exit 1; }
    printf '%s\n' "$FAKE_IDX" ;;
  copy)
    [ "$2" = "$FAKE_SRC" ] && [ "$3" = "$FAKE_DST" ] || { echo "fake docker: unexpected copy $2 -> $3" >&2; exit 1; }
    : > "$FAKE_STATE/copied" ;;
  manifest)
    [ "$2" = "$FAKE_DST@$FAKE_IDX" ] || { echo "fake docker: unexpected manifest ref $2" >&2; exit 1; }
    [ "$FAKE_MANIFEST" != fail ] || { echo "UNAUTHORIZED: $2" >&2; exit 1; }
    cat "$FAKE_MANIFEST" ;;
  *) echo "fake docker: unexpected crane verb $1" >&2; exit 64 ;;
esac
SHIM
chmod +x "$WORK/bin/docker"

printf '# no AGPL entries in this fixture\n' > "$WORK/mirror-sources.yaml"

# write_lock INDEX_DIGEST PLATFORMS_JSON — one mirrorable fixture entry.
write_lock() {
  jq -n --arg idx "$1" --argjson plats "$2" --arg mirror "$MIRROR" '{
    _generated: "fixture for scripts/images/tests/verify-platforms.test.sh",
    schema_version: "1",
    images: [{name: "test-a", role: "fixture", repository: "upstream.invalid/test-a",
      version: "1.0", index_digest: $idx, platforms: $plats, mirror: $mirror}]}' \
    > "$WORK/images.lock.json"
}

# apply_with MANIFEST_FIXTURE [INDEX_DIGEST] — fresh fake registry state, then
# run the tool's --apply; sets OUT, ERR, RC.
apply_with() {
  local idx="${2:-$IDX}"
  rm -rf "$WORK/state" && mkdir "$WORK/state"
  RC=0
  OUT="$(PATH="$WORK/bin:$PATH" FAKE_STATE="$WORK/state" FAKE_CRANE_IMAGE="$CRANE_IMAGE" \
    FAKE_IDX="$idx" FAKE_SRC="upstream.invalid/test-a@$idx" FAKE_DST="$DST" FAKE_MANIFEST="$1" \
    "$TOOL" --lock "$WORK/images.lock.json" --sources "$WORK/mirror-sources.yaml" --apply \
    2>"$WORK/err.log")" || RC=$?
  ERR="$(cat "$WORK/err.log")"
}

want_rc() { [ "$RC" = "$1" ] || fail "exit $RC, want $1 (stdout: $OUT) (stderr: $ERR)"; }
want_out() { [ "$OUT" = "$1" ] || fail "stdout mismatch: got [$OUT] want [$1]"; }
want_err() { grep -q -- "$1" <<<"$ERR" || fail "stderr missing [$1]: $ERR"; }

# want_calls — the tool ran exactly: digest (MISSING), copy, digest, manifest.
want_calls() {
  local verbs
  verbs="$(sed "s|.*$CRANE_IMAGE ||; s| .*||" "$WORK/state/calls.log" | tr '\n' ' ')"
  [ "$verbs" = "digest copy digest manifest " ] || fail "unexpected crane calls: [$verbs]"
}

PLATS="$(jq -n --arg a "$AMD" --arg b "$ARM" '{"linux/amd64": $a, "linux/arm64": $b}')"
write_lock "$IDX" "$PLATS"

# ---- accepted indexes -------------------------------------------------------------
apply_with "$FIX/index-exact.json"
want_rc 0
want_out "PRESENT test-a $IDX $IDX"
want_err "verify test-a: index and 2 platform manifest(s) preserved"
want_calls
ok "an index listing exactly the lock platforms passes"

apply_with "$FIX/index-attested.json"
want_rc 0
want_out "PRESENT test-a $IDX $IDX"
want_err "verify test-a: index and 2 platform manifest(s) preserved"
want_calls
ok "an attested index (unknown/unknown attestation manifests) passes"

apply_with "$FIX/index-superset.json"
want_rc 0
want_out "PRESENT test-a $IDX $IDX"
want_calls
ok "a superset index (ppc64le, s390x the lock does not pin, plus attestations) passes"

write_lock "$IDX" "$(jq -n --arg d "$IDX" '{"linux/amd64": $d}')"
apply_with "$FIX/manifest-single.json"
want_rc 0
want_out "PRESENT test-a $IDX $IDX"
want_err "verify test-a: index and 1 platform manifest(s) preserved"
ok "a single-manifest entry (lock platform = index digest) passes"
write_lock "$IDX" "$PLATS"

# ---- negative controls -------------------------------------------------------------
apply_with "$FIX/index-missing-arm64.json"
want_rc 1
want_out ""
want_err "lock platform digest(s) missing from the mirror index at $DST: $ARM\$"
ok "a missing lock platform (arm64) still fails and prints no PRESENT line"

apply_with "$FIX/index-empty.json"
want_rc 1
want_out ""
want_err "missing from the mirror index at $DST: $AMD $ARM\$"
ok "an empty index fails naming every lock platform"

apply_with fail
want_rc 1
want_out ""
want_err "cannot read the mirror manifest at $DST@$IDX"
ok "an unreadable mirror manifest fails closed"

printf 'not json\n' > "$WORK/garbage.json"
apply_with "$WORK/garbage.json"
want_rc 1
want_out ""
want_err "unreadable mirror manifest at $DST@$IDX"
ok "a non-JSON mirror manifest fails closed"

# crane exiting 0 with an empty or truncated body must not read as "nothing missing".
for body in '' '   ' 'null' '[]'; do
  printf '%s' "$body" > "$WORK/odd.json"
  apply_with "$WORK/odd.json"
  want_rc 1
  want_out ""
  want_err "unreadable mirror manifest at $DST@$IDX"
done
ok "an empty, blank, null or non-object manifest body fails closed"

printf '{"schemaVersion":2,"manifests":[{},{"digest":"%s"}]}' "$AMD" > "$WORK/nodigest.json"
apply_with "$WORK/nodigest.json"
want_rc 1
want_err "missing from the mirror index at $DST: $ARM\$"
ok "index entries without a digest do not satisfy a lock platform"

echo "PASS: $PASSED assertions"
