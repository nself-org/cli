#!/usr/bin/env bash
# Evidence helpers for the importers drill leg (P7-ADOPT-19). Sourced by `run`; never executed.
#
# Purpose: build the single ci.evidence/v1 document the leg prints, one check per source
#   (`importers/<source>`). nself-ci reads the last output line that starts with `{` and contains
#   "ci.evidence/v1" (free/ci/internal/drill/nested.go) and validates it against
#   ci-evidence.v1.schema.json, so the shape here follows that schema exactly.
# Inputs: ev_check <source> <result> <class> <exit> <excerpt-file>; ev_document <start> <end> <checks>.
# Outputs: compact one-line JSON on stdout. Needs jq, git (optional), sha256sum or shasum.
# Constraints: no secret reaches argv (failure excerpts come in through --rawfile); an unbound
#   or git-less tree yields a valid unbound document, never a fake bound one.

ZERO64=0000000000000000000000000000000000000000000000000000000000000000

# ev_sha256 <file|-> : hex digest of a file (or stdin).
ev_sha256() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$@" | cut -d' ' -f1; else shasum -a 256 "$@" | cut -d' ' -f1; fi
}

# ev_check <source> <result> <class> <exit> <excerpt-file>: one check object.
# result is pass|fail|error; class is code|infra|timeout|"" (empty on pass); exit may be "null".
ev_check() {
  local findings=0
  [ "$2" = pass ] || findings=1
  jq -n -c --arg id "importers/$1" --arg job "importers-$1" --arg result "$2" --arg class "$3" \
    --argjson exit "$4" --argjson findings "$findings" --rawfile ex "$5" '
    {id: $id, job_id: $job, kind: "test", tool: "bash", tool_version: null, required: true,
     substantive: true, result: $result, findings: $findings, excerpt: $ex[0:3500], log_ref: null, exit: $exit}
    + (if $class == "" then {} else {failure_class: $class} end)'
}

# ev_document <started-epoch-s> <finished-epoch-s> <checks-ndjson-file> <root>: the evidence line.
ev_document() {
  local t0=$1 t1=$2 checks=$3 root=$4
  local rev state tcd cfg repo plat prov iso
  rev=$(git -C "$root" rev-parse HEAD 2>/dev/null || true)
  state=clean tcd=""
  if [ -z "$rev" ]; then
    rev=${ZERO64:0:40} state=dirty tcd=$ZERO64
  elif [ -n "$(git -C "$root" status --porcelain --untracked-files=no 2>/dev/null)" ]; then
    state=dirty tcd=$(git -C "$root" diff HEAD 2>/dev/null | ev_sha256 -)
  fi
  cfg=$ZERO64
  [ -f "$root/.nself/ci.yaml" ] && cfg=$(ev_sha256 "$root/.nself/ci.yaml")
  repo=$(printf %s "$(git -C "$root" config --get remote.origin.url 2>/dev/null || echo "$root")" | ev_sha256 -)
  plat="$(uname -s | tr '[:upper:]' '[:lower:]')/$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')"
  prov=local iso=process
  if [ "${CI:-}" = true ]; then prov=github-actions iso=hosted-disposable; fi
  jq -s -c --arg rev "$rev" --arg state "$state" --arg tcd "$tcd" --arg cfg "$cfg" --arg repo "local:$repo" \
    --arg run "importers-$t0-$$" --arg host "$(hostname)" --arg plat "$plat" --arg prov "$prov" --arg iso "$iso" \
    --arg zero "$ZERO64" --argjson t0 "$t0" --argjson t1 "$t1" '
    def ts: strftime("%Y-%m-%dT%H:%M:%SZ");
    def n(r): map(select(.result == r)) | length;
    {schema: "ci.evidence/v1", revision: $rev, repo_id: $repo, run_id: $run,
     pipeline: {id: "importers", name: "importers", kind: "drill"},
     trigger: "local", source_trust: "internal", policy_digest: $zero,
     input_digests: {config: $cfg, lockfiles: [], generated: []},
     worktree_state: $state, tracked_changes_digest: (if $tcd == "" then null else $tcd end),
     binding: (if $state == "clean" then {state: "bound", reason: null} else {state: "unbound", reason: "dirty"} end),
     runner: {id: $host, provider: $prov, platform: $plat, isolation: $iso},
     producer: {kind: "self-reported", version: "1"},
     timings: {queued_at: ($t0 | ts), started_at: ($t0 | ts), finished_at: ($t1 | ts), duration_ms: (($t1 - $t0) * 1000)},
     tool_versions: {}, checks: ., artifacts: [], retries: [],
     selection: {mode: "full", reason: "selection.no_base", digest: $zero},
     result: (if n("fail") > 0 then "fail" elif n("error") > 0 then "error" else "pass" end),
     counts: {pass: n("pass"), fail: n("fail"), skip: n("skip"), error: n("error"), total: length},
     freshness: (if $state == "clean" then {state: "current", reason: null} else {state: "unknown", reason: null} end),
     signature: {kind: "coordinator-recorded"}}' "$checks"
}
