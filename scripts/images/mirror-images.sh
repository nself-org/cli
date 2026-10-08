#!/usr/bin/env bash
# scripts/images/mirror-images.sh — lock-driven image mirror tool (P7-PROD-73).
#
# Purpose: copy every mirrorable entry of the generated image lock (P7-LIVE-17,
# internal/compose/images.lock.json) to its mirror repository, digest-preserving
# across the index and every platform manifest, using crane run from a
# digest-pinned container (ADR 0030 §2, EPIC P7-PROD D19). No host crane install
# and no new Go dependency.
# Usage:  scripts/images/mirror-images.sh [--lock <path>] [--plan|--apply|--check]
#           [--only <name>] [--insecure] [--docker-network <net>]
#           [--sources <path>] [--auth-user <u> --auth-token-stdin <registry>]
#         scripts/images/mirror-images.sh --archive <tarball> --as <repo:tag>
#         scripts/images/mirror-images.sh --list-tags <repository>
# Inputs: the image lock (jq), scripts/images/mirror-sources.yaml (AGPL source
#         links), an optional docker-save tarball, an optional registry token
#         on stdin (never argv, never a committed file).
# Output: dry-run (default), --plan and --check print one status line per
#         mirrorable lock entry on stdout, sorted by name:
#           PRESENT|MISSING|DIFFERS <name> <upstream index digest> <mirror digest or ->
#         --apply prints the same lines for the resulting state; copy, verify
#         and AGPL-source detail goes to stderr. --archive prints
#         "<ref> <digest>"; --list-tags prints one tag per line.
# Exit:   0 success (for --check: every line PRESENT); 1 a line is not PRESENT
#         (--check) or a copy/verify failed; 2 usage or input error.
# Env:    none required. Dry-run is the default; --apply is the only mode that
#         writes to a registry.
set -euo pipefail

# The crane image is pinned by index digest (EPIC D19): crane never runs from a
# host install or a floating tag. Bumping this reference is a reviewed change
# (update .github/wiki/operations/image-mirror.md in the same PR).
CRANE_IMAGE="gcr.io/go-containerregistry/crane:v0.22.1@sha256:1f968817b95790bed063f71175aa6b8ff879fa17064020415f3e18bb6e6a36e1"
CRANE_ENTRYPOINT="/ko-app/crane"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
OPT_LOCK="$SCRIPT_DIR/../../internal/compose/images.lock.json"
OPT_SOURCES="$SCRIPT_DIR/mirror-sources.yaml"
OPT_MODE=plan            # plan (default dry-run) | apply | check
OPT_ONLY=""
OPT_INSECURE=0
OPT_NETWORK=""
OPT_AUTH_USER=""
OPT_AUTH_REGISTRY=""
OPT_ARCHIVE=""
OPT_AS=""
OPT_LIST_TAGS=""
AUTH_DIR=""

die() { echo "mirror-images: $*" >&2; exit 2; }        # usage / input error
fail() { echo "mirror-images: $*" >&2; exit 1; }       # copy or verify failure
note() { echo "mirror-images: $*" >&2; }

usage() { awk 'NR>1 && /^#/ {sub(/^# ?/,""); print; next} NR>1 {exit}' "$0"; exit 0; }

# ---- flag parsing -----------------------------------------------------------
while [ $# -gt 0 ]; do
  case "$1" in
    --lock) OPT_LOCK="${2:-}"; shift 2 ;;
    --sources) OPT_SOURCES="${2:-}"; shift 2 ;;
    --only) OPT_ONLY="${2:-}"; shift 2 ;;
    --docker-network) OPT_NETWORK="${2:-}"; shift 2 ;;
    --auth-user) OPT_AUTH_USER="${2:-}"; shift 2 ;;
    --auth-token-stdin) OPT_AUTH_REGISTRY="${2:-}"; shift 2 ;;
    --archive) OPT_ARCHIVE="${2:-}"; shift 2 ;;
    --as) OPT_AS="${2:-}"; shift 2 ;;
    --list-tags) OPT_LIST_TAGS="${2:-}"; shift 2 ;;
    --plan) OPT_MODE=plan; shift ;;
    --apply) OPT_MODE=apply; shift ;;
    --check) OPT_MODE=check; shift ;;
    --insecure) OPT_INSECURE=1; shift ;;
    -h|--help) usage 0 ;;
    *) die "unknown argument: $1" ;;
  esac
done
command -v jq >/dev/null 2>&1 || die "jq is required"
command -v docker >/dev/null 2>&1 || die "docker is required"
[ -f "$OPT_LOCK" ] || die "lock not found: $OPT_LOCK"
[ -f "$OPT_SOURCES" ] || die "sources file not found: $OPT_SOURCES"

# ---- auth (credentials only via stdin into a temp DOCKER_CONFIG) ------------
# shellcheck disable=SC2329
cleanup() { [ -n "$AUTH_DIR" ] && rm -rf "$AUTH_DIR" || true; }
trap 'cleanup' EXIT
if [ -n "$OPT_AUTH_REGISTRY" ]; then
  [ -n "$OPT_AUTH_USER" ] || die "--auth-token-stdin needs --auth-user"
  token="$(head -n1)" && [ -n "$token" ] || die "no token on stdin"
  key="$OPT_AUTH_REGISTRY"
  [ "$key" = "docker.io" ] && key="https://index.docker.io/v1/"
  AUTH_DIR="$(mktemp -d)"
  umask 077
  printf '{"auths":{"%s":{"auth":"%s"}}}' "$key" \
    "$(printf '%s:%s' "$OPT_AUTH_USER" "$token" | base64 | tr -d '\n')" \
    > "$AUTH_DIR/config.json"
  unset token
  note "auth: using stdin token for $OPT_AUTH_REGISTRY (temp DOCKER_CONFIG, removed on exit)"
fi

# ---- crane ------------------------------------------------------------------
# crane_run ARGS... — run the pinned crane container. Every docker call is an
# argv array; refs and flags are single arguments, never a container shell.
crane_run() {
  local -a args=(run --rm --entrypoint "$CRANE_ENTRYPOINT")
  [ -n "$OPT_NETWORK" ] && args+=(--network "$OPT_NETWORK")
  if [ -n "$AUTH_DIR" ]; then
    args+=(-v "$AUTH_DIR":/crane-auth:ro -e DOCKER_CONFIG=/crane-auth)
  fi
  args+=("$CRANE_IMAGE" "$@")
  [ "$OPT_INSECURE" = 1 ] && args+=(--insecure)
  docker "${args[@]}"
}

# mirror_digest REPO:TAG — print the tag's digest, or fail when the reference
# does not resolve (read as MISSING by the caller).
mirror_digest() {
  crane_run digest "$1" 2>/dev/null
}

# ---- mirror-sources.yaml (AGPL entries; fixed two-space block shape) --------
# sources_field NAME FIELD — print one field (license|upstream|source) of a
# sources entry; exit 1 when NAME or FIELD is absent, 2 when the file is malformed.
sources_field() {
  awk -v want="$1" -v field="$2" '
    /^#/ { next }
    /^[A-Za-z0-9][A-Za-z0-9._\/-]*:$/ { cur = substr($0, 1, length($0) - 1); ok = (cur == want); next }
    /^  [a-z]+: / { if (ok && $1 == field ":") { sub(/^  [a-z]+: /, ""); print; found = 1 } next }
    /^[[:space:]]*$/ { next }
    { print "mirror-images: bad sources line: " $0 > "/dev/stderr"; bad = 1 }
    END { exit bad ? 2 : (found ? 0 : 1) }' "$OPT_SOURCES"
}

# source_hint NAME MIRROR_REPO — stderr reminder of the upstream source URL for
# an AGPL entry (the mirror repository description carries it; ADR 0030 §2).
source_hint() {
  local url rc=0
  url="$(sources_field "$1" source)" || rc=$?
  case "$rc" in
    0) ;;
    1) return 0 ;;   # not an AGPL entry
    *) die "malformed sources file $OPT_SOURCES" ;;
  esac
  note "source: $1 $url (AGPL — set as the $2 repository description)"
}

# ---- status computation -----------------------------------------------------
# probe_entry NAME INDEX_DIGEST MIRROR:VERSION — print one status line
# (PRESENT|MISSING|DIFFERS + name + upstream index digest + mirror digest/-)
# by resolving the mirror tag; non-PRESENT also returns 1.
probe_entry() {
  local name="$1" want="$2" dst="$3" have
  have="$(mirror_digest "$dst" || true)"
  if [ -z "$have" ]; then
    printf 'MISSING %s %s -\n' "$name" "$want"
  elif [ "$have" = "$want" ]; then
    printf 'PRESENT %s %s %s\n' "$name" "$want" "$have"
  else
    printf 'DIFFERS %s %s %s\n' "$name" "$want" "$have"
  fi
  [ -n "$have" ] && [ "$have" = "$want" ]
}

# verify_platforms MIRROR INDEX_DIGEST PLATFORMS... — the mirror's manifest at
# the index digest must list exactly the lock's platform manifest digests (a
# single-manifest entry carries the index digest itself).
verify_platforms() {
  local dst="$1" idx="$2" got expected
  shift 2
  got="$(crane_run manifest "$dst@$idx" \
    | jq -r --arg self "$idx" 'if .manifests then [.manifests[].digest] else [$self] end | sort | join(" ")')"
  expected="$(printf '%s\n' "$@" | sort | tr '\n' ' ' | sed 's/ $//')"
  [ "$got" = "$expected" ] || fail "platform digests differ at $dst: lock=[$expected] mirror=[$got]"
}

# ---- --list-tags (weekly pin bump, P7-HYG-34) -------------------------------
if [ -n "$OPT_LIST_TAGS" ]; then
  crane_run ls "$OPT_LIST_TAGS"
  exit 0
fi

# ---- --archive (docker-save tarball, e.g. the 2026-09-28 MinIO image) -------
if [ -n "$OPT_ARCHIVE" ]; then
  [ -n "$OPT_AS" ] || die "--archive needs --as <repo:tag>"
  [ -f "$OPT_ARCHIVE" ] || die "archive not found: $OPT_ARCHIVE"
  case "$OPT_AS" in *:*) ;; *) die "--as must be <repo:tag>" ;; esac
  abs="$(cd "$(dirname "$OPT_ARCHIVE")" && pwd)/$(basename "$OPT_ARCHIVE")"
  note "archive: pushing $abs as $OPT_AS"
  push=(run --rm --entrypoint "$CRANE_ENTRYPOINT")
  [ -n "$OPT_NETWORK" ] && push+=(--network "$OPT_NETWORK")
  push+=(-v "$abs":/archive.tar:ro "$CRANE_IMAGE" push /archive.tar "$OPT_AS")
  [ "$OPT_INSECURE" = 1 ] && push+=(--insecure)
  docker "${push[@]}" >&2
  digest="$(crane_run digest "$OPT_AS")"
  printf '%s %s\n' "$OPT_AS" "$digest"
  as_repo="${OPT_AS%:*}"
  source_hint "${as_repo##*/}" "$as_repo"
  exit 0
fi


# ---- build the plan from the lock -------------------------------------------
NODIGEST="$(jq -r '[.images[] | select(.mirror != null and ((.index_digest // "") == "")) | .name] | join(" ")' "$OPT_LOCK")"
[ -z "$NODIGEST" ] || die "lock entries with a mirror but no index_digest: $NODIGEST"
mapfile -t ENTRIES < <(jq -c --arg only "$OPT_ONLY" '
  .images | map(select(.mirror != null))
    | if $only != "" then map(select(.name == $only)) else . end
    | sort_by(.name) | .[]' "$OPT_LOCK")
[ "${#ENTRIES[@]}" -gt 0 ] || die "no mirrorable lock entries matched${OPT_ONLY:+ (only: $OPT_ONLY)}"
for e in "${ENTRIES[@]}"; do
  jq -e 'has("name") and has("repository") and has("version") and has("platforms") and (.platforms|length>0)' \
    >/dev/null <<<"$e" || die "malformed lock entry: $e"
done

STATUS_FAIL=0
for e in "${ENTRIES[@]}"; do
  name="$(jq -r '.name' <<<"$e")"
  repo="$(jq -r '.repository' <<<"$e")"
  idx="$(jq -r '.index_digest' <<<"$e")"
  version="$(jq -r '.version' <<<"$e")"
  mirror="$(jq -r '.mirror' <<<"$e")"
  src="$repo@$idx"
  dst="$mirror:$version"
  mapfile -t plats < <(jq -r '.platforms[]' <<<"$e")

  if [ "$OPT_MODE" = apply ]; then
    if [ "$(mirror_digest "$dst" || true)" = "$idx" ]; then
      note "skip $name: $dst already at $idx"
      printf 'PRESENT %s %s %s\n' "$name" "$idx" "$idx"
      source_hint "$name" "$mirror"
      continue
    fi
    note "copy $name: $src -> $dst"
    crane_run copy "$src" "$dst" >&2
    have="$(crane_run digest "$dst")"
    [ "$have" = "$idx" ] || fail "mirror digest mismatch for $name: lock=$idx mirror=$have"
    verify_platforms "$dst" "$idx" "${plats[@]}"
    note "verify $name: index and ${#plats[@]} platform manifest(s) preserved"
    printf 'PRESENT %s %s %s\n' "$name" "$idx" "$have"
    source_hint "$name" "$mirror"
  else
    line="$(probe_entry "$name" "$idx" "$dst" || true)"
    printf '%s\n' "$line"
    case "$line" in PRESENT\ *) ;; *) STATUS_FAIL=1 ;; esac
    if [ "$OPT_MODE" = plan ]; then
      state="${line%% *}"
      [ "$state" = PRESENT ] && action="present, no copy needed" || action="copy needed"
      note "plan: $name $src -> $dst ($action)"
    fi
    source_hint "$name" "$mirror"
  fi
done

if [ "$OPT_MODE" = "check" ] && [ "$STATUS_FAIL" = 1 ]; then
  note "check: not every mirrorable entry is PRESENT"
  exit 1
fi
exit 0
