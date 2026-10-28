#!/usr/bin/env bash
# Purpose: static guard for scripts/ci/hasura-upgrade-smoke.sh (D-1152). The switch
#          `nself start` (step 4) must pass --skip-db-init: a plain start re-applies the
#          saved Hasura metadata to the new engine (start.go: hasura.ApplyIfPresent), so
#          the preservation checks in step 5 could never fail. The smoke itself needs
#          Docker; this check does not.
# Inputs:  none (reads the smoke script next to this file).
# Outputs: "ok <case>" per passing case; exit 0 when all pass, 1 otherwise.
# Constraints: bash 3.2 compatible; offline.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
SMOKE="${1:-${HERE}/hasura-upgrade-smoke.sh}"

# The `nsf start` lines between the step 4 and step 5 headers.
STEP4="$(awk '/^step "4 /{on=1} /^step "5 /{on=0} on && /^nsf start/' "${SMOKE}")"

if [ -z "${STEP4}" ]; then
  printf 'FAIL step 4 has no `nsf start` line (script layout changed: update this check)\n' >&2
  exit 1
fi
case "${STEP4}" in
  *--skip-db-init*) printf 'ok step 4 start passes --skip-db-init\n' ;;
  *) printf 'FAIL step 4 start re-applies saved metadata (no --skip-db-init): %s\n' "${STEP4}" >&2; exit 1 ;;
esac

# Check ordering and exercise the real redactor without starting Docker.
python3 - "$SMOKE" <<'PYTEST'
import json, os, pathlib, subprocess, sys, tempfile
script = pathlib.Path(sys.argv[1]).read_text()
step5 = script.split('step "5 after the switch"', 1)[1]
assert step5.index("nsf db hasura diff") < step5.index('export_tree "$WORK/meta-post"'), "diff must run before export rewrites metadata"
print("ok diff precedes post-switch export")
redactor = script.split("redact() {", 1)[1].split("\nfail()", 1)[0]
with tempfile.TemporaryDirectory(prefix="dw9-cli-7.") as d:
    admin = "synthetic-admin-with/slashes+and.dots"
    key = "synthetic-jwt-signing-key/with+punctuation"
    jwt = json.dumps({"type": "HS256", "key": key}, separators=(",", ":"))
    pathlib.Path(d, ".env").write_text("HASURA_GRAPHQL_ADMIN_SECRET=" + admin + "\nHASURA_JWT_KEY=" + key + "\nHASURA_GRAPHQL_JWT_SECRET='" + jwt + "'\n")
    payload = "ok " + admin + " " + key + " " + jwt + " eyJsynthetic.token " + "ab" * 32
    result = subprocess.run(["bash", "-c", "redact() {" + redactor + "\nredact"], input=payload, text=True, capture_output=True, check=True, env=dict(os.environ, PDIR=d))
    assert all(v not in result.stdout for v in (admin, key, jwt, "eyJsynthetic.token", "ab" * 32)), "redaction leaked a synthetic secret"
    assert result.stdout.startswith("ok "), "redaction discarded unrelated output"
print("ok redaction masks literal admin and JWT secrets")
PYTEST
