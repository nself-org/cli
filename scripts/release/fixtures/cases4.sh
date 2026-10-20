# shellcheck shell=bash
# Purpose: fourth part of test.sh (sourced before the pty block): bump-version.sh must
# regenerate the four version-hash goldens in the same step (the v1.4.14 release PR was
# red on its first push because it did not), and refuse to launder other tree drift.
# Inputs:  ROOT BASE from test.sh. A stub `go` stands in for the toolchain: it logs each
#          call and, under UPDATE_GOLDEN=1, rewrites the `.nself/build-version` lines.

GOLD_PATHS="internal/build/testdata/plan-mode/golden-dev-minimal.txt internal/build/testdata/plan-mode/golden-fronted.txt internal/build/testdata/plan-mode/golden-prod-plugins.txt internal/reconcile/testdata/fixtures/dev-minimal/golden-tree.txt"
# Fake sha256-shaped hashes (the guard masks 64 hex digits); obviously synthetic.
OLDH=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
NEWH=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb

# mk_bump_env: a nself/{cli,admin} tree at 1.4.13 with the golden tests present and a stub go.
mk_bump_env() {
  B=$(mktemp -d "$BASE/bump.XXXXXX"); C=$B/nself/cli; A=$B/nself/admin
  mkdir -p "$C/scripts" "$C/internal/version" "$C/sdk/go" "$C/sdk/ts" "$C/sdk/ts-sdk" "$C/sdk/ts-cli-api" "$C/sdk/py" "$C/.github" \
    "$C/internal/build" "$C/cmd/commands" "$A/src/lib" "$B/bin"
  cp "$ROOT/scripts/bump-version.sh" "$C/scripts/"
  cp -R "$ROOT/scripts/bump-version" "$C/scripts/"
  mkdir -p "$A/.github/wiki"
  printf 'Version 1.4.13 · MIT License\n**v1.4.13** - See CHANGELOG\n' > "$A/.github/wiki/Home.md"
  echo 1.4.13 > "$C/.github/VERSION"
  printf 'package version\n\nvar (\n\tVersion string = "1.4.13"\n)\n' > "$C/internal/version/version.go"
  printf 'package sdk\n\nconst Version = "1.4.13"\n' > "$C/sdk/go/doc.go"
  for f in sdk/ts sdk/ts-sdk sdk/ts-cli-api; do echo '{"version":"1.4.13"}' > "$C/$f/package.json"; done
  printf '[project]\nversion = "1.4.13"\n' > "$C/sdk/py/pyproject.toml"
  echo '{"version":"1.4.13"}' > "$A/package.json"
  echo "export const CLI_VERSION = '1.4.13'" > "$A/src/lib/cli-version.ts"
  printf 'ARG NSELF_VERSION=1.4.13\nENV ADMIN_VERSION=1.4.13\nLABEL org.opencontainers.image.version="1.4.13"\n' > "$A/Dockerfile"
  : > "$C/internal/build/plan_mode_test.go"; : > "$C/cmd/commands/build_plan_test.go"
  for g in $GOLD_PATHS; do
    mkdir -p "$C/$(dirname "$g")"
    printf 'd 0755 project/.nself\nf 0644 project/.nself/build-version %s\nf 0644 project/other keep\n' "$OLDH" > "$C/$g"
  done
  cat > "$B/bin/go" <<'STUB'
#!/usr/bin/env bash
echo "UPDATE_GOLDEN=${UPDATE_GOLDEN:-} $*" >> "$GO_LOG"
[ "${STUB_GO_FAIL_PLAIN:-}" = 1 ] && [ -z "${UPDATE_GOLDEN:-}" ] && exit 1
if [ "${UPDATE_GOLDEN:-}" = 1 ] && [ "${STUB_GO_NOOP:-}" != 1 ]; then
  for g in $GOLD_PATHS; do
    sed "s/build-version $OLDH/build-version $NEWH/" "$GOLD_ROOT/$g" > "$GOLD_ROOT/$g.new"
    [ "${STUB_GO_DRIFT:-}" = 1 ] && sed 's/other keep/other changed/' "$GOLD_ROOT/$g.new" > "$GOLD_ROOT/$g.new2" && mv "$GOLD_ROOT/$g.new2" "$GOLD_ROOT/$g.new"
    # Edits to the build-version line itself that are not a plain hash swap.
    [ "${STUB_GO_DELETE:-}" = 1 ] && sed '/build-version/d' "$GOLD_ROOT/$g.new" > "$GOLD_ROOT/$g.new2" && mv "$GOLD_ROOT/$g.new2" "$GOLD_ROOT/$g.new"
    [ "${STUB_GO_MODE:-}" = 1 ] && sed 's/^f 0644 \(.*build-version\)/f 0600 \1/' "$GOLD_ROOT/$g.new" > "$GOLD_ROOT/$g.new2" && mv "$GOLD_ROOT/$g.new2" "$GOLD_ROOT/$g.new"
    [ "${STUB_GO_EXTRA:-}" = 1 ] && printf 'f 0644 project/.nself/build-version-extra %s\n' "$NEWH" >> "$GOLD_ROOT/$g.new"
    mv "$GOLD_ROOT/$g.new" "$GOLD_ROOT/$g"
  done
fi
exit 0
STUB
  chmod +x "$B/bin/go"
  export GO_LOG=$B/go.log GOLD_PATHS GOLD_ROOT=$C OLDH NEWH; : > "$GO_LOG"
}
run_bump() { PATH="$B/bin:$ORIG_PATH" bash "$C/scripts/bump-version.sh" "$@" >"$B/out" 2>"$B/err"; RC=$?; }
gold_all() { local g; for g in $GOLD_PATHS; do grep -q "$1" "$C/$g" || return 1; done; }

# 13. bump regenerates the goldens, then re-runs the tests plain -------------------------
mk_bump_env
run_bump 9.9.9
check "bump exits 0" test "$RC" -eq 0
check "bump updated version.go" grep -q '"9.9.9"' "$C/internal/version/version.go"
check "bump regenerated all four goldens" gold_all "build-version $NEWH"
check "bump kept the other golden lines" gold_all 'other keep'
check "bump ran the build golden with UPDATE_GOLDEN=1" grep -q '^UPDATE_GOLDEN=1 test .*./internal/build ' "$GO_LOG"
check "bump ran the cmd golden with UPDATE_GOLDEN=1" grep -q '^UPDATE_GOLDEN=1 test .*./cmd/commands ' "$GO_LOG"
check "bump re-ran both tests plain" test "$(grep -c '^UPDATE_GOLDEN= test' "$GO_LOG")" -eq 2
rm -rf "$B"

# 14. other drift is refused and the goldens come back -------------------------------------
mk_bump_env
export STUB_GO_DRIFT=1; run_bump 9.9.9; unset STUB_GO_DRIFT
check "drift in a golden exits 1" test "$RC" -eq 1
check "drift is named" grep -q 'changed beyond .nself/build-version' "$B/err"
check "drift restored the goldens" gold_all "build-version $OLDH"
rm -rf "$B"

# 14b. the guard accepts only the hash on a build-version line: deleting, re-moding or
# adding such a line is refused too (each one restores the goldens) --------------------------
for mode in DELETE MODE EXTRA; do
  mk_bump_env
  export "STUB_GO_$mode=1"; run_bump 9.9.9; unset "STUB_GO_$mode"
  check "$mode of a build-version line exits 1" test "$RC" -eq 1
  check "$mode of a build-version line is named" grep -q 'changed beyond .nself/build-version' "$B/err"
  check "$mode of a build-version line restored the goldens" gold_all "build-version $OLDH"
  rm -rf "$B"
done

# 14c. a golden test that did not regenerate anything (renamed or skipped) fails the bump --
mk_bump_env
export STUB_GO_NOOP=1; run_bump 9.9.9; unset STUB_GO_NOOP
check "no-op golden regeneration exits 1" test "$RC" -eq 1
check "no-op golden regeneration is named" grep -q 'did not change although the version moved 1.4.13 -> 9.9.9' "$B/err"
rm -rf "$B"
# re-running a bump at the version already in place leaves the goldens alone and passes
mk_bump_env
run_bump 9.9.9; run_bump 9.9.9
check "re-running the same bump exits 0" test "$RC" -eq 0
check "re-running the same bump keeps the new goldens" gold_all "build-version $NEWH"
rm -rf "$B"

# 14d. the caller's UPDATE_GOLDEN does not reach the plain re-run
mk_bump_env
export UPDATE_GOLDEN=1; run_bump 9.9.9; unset UPDATE_GOLDEN
check "exported UPDATE_GOLDEN: plain runs stay plain" test "$(grep -c '^UPDATE_GOLDEN= test' "$GO_LOG")" -eq 2
rm -rf "$B"

# 15. a red plain run after regeneration fails the bump ------------------------------------
mk_bump_env
export STUB_GO_FAIL_PLAIN=1; run_bump 9.9.9; unset STUB_GO_FAIL_PLAIN
check "red plain tests exit 1" test "$RC" -eq 1
check "red plain tests are named" grep -q 'golden tests still fail' "$B/err"
rm -rf "$B"

# 16. --no-goldens and --dry-run never call go ---------------------------------------------
mk_bump_env
run_bump 9.9.9 --no-goldens
check "--no-goldens exits 0" test "$RC" -eq 0
check "--no-goldens made no go call" test ! -s "$GO_LOG"
check "--no-goldens left the goldens" gold_all "build-version $OLDH"
rm -rf "$B"
mk_bump_env
run_bump 9.9.9 --dry-run
check "--dry-run exits 0" test "$RC" -eq 0
check "--dry-run made no go call" test ! -s "$GO_LOG"
check "--dry-run says it would regenerate" grep -q 'Would regenerate 4 version-hash goldens' "$B/out"
rm -rf "$B"

# 17. the S3 plan names the golden regeneration; S2 skips it --------------------------------
mk_env "$HAVE_GO"
run_plain --plan --version 9.9.9
check "plan S3 shows the golden regeneration" grep -q '^S3 cli-version cli .*version-hash goldens' "$T/out"
check "S2 bumps with --no-goldens" grep -q 'bump_all --no-goldens' "$RELDIR/lib/steps1.sh"
rm -rf "$T"

# 18. the image wait is 60 minutes (180 polls x 20 s) for S8 and S9 -------------------------
check "image wait budget is 60 minutes" bash -c ". '$RELDIR/lib/common.sh'; test \$((IMAGE_WAIT_POLLS * POLL_SECONDS)) -eq 3600"
# shellcheck disable=SC2016
check "image steps use the constant" test "$(grep -c '\$IMAGE_WAIT_POLLS' "$RELDIR/lib/steps3.sh")" -eq 2
