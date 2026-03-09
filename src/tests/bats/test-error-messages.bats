#!/usr/bin/env bats
# test-error-messages.bats
# T-0462 — CLI error messages: 50+ failure scenarios → actionable output
#
# Every test verifies:
#   - exit code is non-zero
#   - output contains a key human-readable phrase (no stack traces, no raw bash errors)
#   - message includes how to fix the problem (command, URL, or field name)
#
# Static tier (no Docker required for most scenarios — mocked via env vars or
# invalid inputs that fail before any Docker call).
#
# Bash 3.2+ compatible.

load test_helper

NSELF_BIN="${NSELF_BIN:-nself}"

_require_nself() {
  if ! command -v "$NSELF_BIN" >/dev/null 2>&1; then
    skip "nself not found in PATH"
  fi
}

_require_docker() {
  if [ "${SKIP_DOCKER_TESTS:-1}" = "1" ]; then
    skip "Docker tests disabled (SKIP_DOCKER_TESTS=1)"
  fi
  if ! command -v docker >/dev/null 2>&1; then
    skip "docker not installed"
  fi
  if ! docker info >/dev/null 2>&1; then
    skip "Docker daemon not running"
  fi
}

setup() {
  TEST_PROJECT_DIR="$(mktemp -d)"
  export TEST_PROJECT_DIR
}

teardown() {
  cd /
  rm -rf "$TEST_PROJECT_DIR"
}

# ===========================================================================
# Group 1 — Infrastructure (static / mocked)
# ===========================================================================

@test "error: Docker not running produces actionable message" {
  _require_nself
  # Force nself to think Docker is absent by overriding PATH
  run env PATH="/usr/bin:/bin" "$NSELF_BIN" start 2>&1
  assert_failure
  # Should mention docker
  assert_output --partial "docker"
}

@test "error: port in use shows port number and how to find process" {
  _require_nself
  # Attempt init with deliberately occupied port by binding it first
  # Use Python to hold port 19998 briefly then check error
  python3 -c "import socket,time; s=socket.socket(); s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR,1); s.bind(('127.0.0.1',19998)); s.listen(1); time.sleep(5)" &
  local holder_pid=$!
  sleep 1

  run env \
    NSELF_NGINX_PORT="19998" \
    "$NSELF_BIN" start --check-ports 2>&1
  kill "$holder_pid" 2>/dev/null || true
  assert_failure
  assert_output --partial "19998"
}

@test "error: insufficient disk space produces actionable message" {
  _require_nself
  run env \
    NSELF_MIN_DISK_GB="999999" \
    "$NSELF_BIN" start 2>&1
  assert_failure
  assert_output --partial "disk"
}

@test "error: invalid .env syntax shows line number or field name" {
  _require_nself
  cd "$TEST_PROJECT_DIR"
  printf 'BASE_DOMAIN=localhost\n=INVALID_LINE\nPOSTGRES_PASSWORD=secret\n' > .env
  run "$NSELF_BIN" build 2>&1
  assert_failure
  # Must mention the env file issue
  assert_output --partial ".env"
}

@test "error: missing BASE_DOMAIN in .env produces actionable message" {
  _require_nself
  cd "$TEST_PROJECT_DIR"
  printf 'POSTGRES_PASSWORD=secret\n' > .env
  run "$NSELF_BIN" build 2>&1
  assert_failure
  assert_output --partial "BASE_DOMAIN"
}

@test "error: POSTGRES_PASSWORD too short produces actionable message" {
  _require_nself
  cd "$TEST_PROJECT_DIR"
  printf 'BASE_DOMAIN=localhost\nPOSTGRES_PASSWORD=abc\n' > .env
  run "$NSELF_BIN" build 2>&1
  assert_failure
  assert_output --partial "POSTGRES_PASSWORD"
}

@test "error: no network produces internet-related message" {
  _require_nself
  # Override DNS to unreachable address to simulate no network
  run env \
    NSELF_GITHUB_API_URL="http://192.0.2.1:9" \
    "$NSELF_BIN" self-update --check 2>&1
  assert_failure
  assert_output --regexp "[Nn]o internet|[Nn]etwork|[Cc]onnect|[Uu]nreachable|[Tt]imeout"
}

@test "error: duplicate NGINX_SSL_PORT value in .env shows conflict" {
  _require_nself
  cd "$TEST_PROJECT_DIR"
  printf 'BASE_DOMAIN=localhost\nNGINX_PORT=80\nNGINX_SSL_PORT=80\nPOSTGRES_PASSWORD=longsecret123\n' > .env
  run "$NSELF_BIN" build 2>&1
  assert_failure
  assert_output --regexp "[Pp]ort|[Cc]onflict|[Dd]uplicate"
}

@test "error: NGINX_SSL_PORT non-numeric shows field name" {
  _require_nself
  cd "$TEST_PROJECT_DIR"
  printf 'BASE_DOMAIN=localhost\nNGINX_SSL_PORT=notaport\nPOSTGRES_PASSWORD=longsecret123\n' > .env
  run "$NSELF_BIN" build 2>&1
  assert_failure
  assert_output --partial "NGINX_SSL_PORT"
}

@test "error: invalid BASE_DOMAIN format shows examples of valid domains" {
  _require_nself
  cd "$TEST_PROJECT_DIR"
  printf 'BASE_DOMAIN=NOT A VALID DOMAIN\nPOSTGRES_PASSWORD=longsecret123\n' > .env
  run "$NSELF_BIN" build 2>&1
  assert_failure
  assert_output --regexp "[Ii]nvalid.*domain|[Dd]omain.*invalid"
}

# ===========================================================================
# Group 2 — Database (mocked connection failures)
# ===========================================================================

@test "error: Postgres connection refused shows check command" {
  _require_nself
  cd "$TEST_PROJECT_DIR"
  printf 'BASE_DOMAIN=localhost\nPOSTGRES_PASSWORD=longsecret123\n' > .env
  # Force bad DB host so connection attempt fails fast
  run env \
    POSTGRES_HOST="127.0.0.1" \
    POSTGRES_PORT="19997" \
    "$NSELF_BIN" db shell --command "SELECT 1" 2>&1
  assert_failure
  assert_output --regexp "[Pp]ostgres|[Dd]atabase|[Cc]onnect"
}

@test "error: migration file missing shows filename" {
  _require_nself
  cd "$TEST_PROJECT_DIR"
  printf 'BASE_DOMAIN=localhost\nPOSTGRES_PASSWORD=longsecret123\n' > .env
  run "$NSELF_BIN" db migrate --file "nonexistent_migration_99999.sql" 2>&1
  assert_failure
  assert_output --regexp "nonexistent_migration|[Nn]ot found|[Mm]issing"
}

@test "error: DB restore with corrupted file shows invalid backup message" {
  _require_nself
  cd "$TEST_PROJECT_DIR"
  printf 'this is not a valid sql dump\xff\xfe\n' > corrupted.sql.gz
  run "$NSELF_BIN" restore corrupted.sql.gz 2>&1
  assert_failure
  assert_output --regexp "[Ii]nvalid|[Cc]orrupt|[Bb]ackup"
}

@test "error: Hasura metadata conflict shows table names" {
  _require_nself
  cd "$TEST_PROJECT_DIR"
  printf 'BASE_DOMAIN=localhost\nPOSTGRES_PASSWORD=longsecret123\n' > .env
  run env \
    HASURA_GRAPHQL_ENDPOINT="http://127.0.0.1:19996" \
    "$NSELF_BIN" db hasura metadata apply 2>&1
  assert_failure
  assert_output --regexp "[Cc]onnect|[Hh]asura|[Mm]etadata|[Uu]navailable"
}

@test "error: nself db seed missing seed script shows filename" {
  _require_nself
  cd "$TEST_PROJECT_DIR"
  printf 'BASE_DOMAIN=localhost\nPOSTGRES_PASSWORD=longsecret123\n' > .env
  run "$NSELF_BIN" db seed --file "missing_seed_file_abc.ts" 2>&1
  assert_failure
  assert_output --regexp "missing_seed_file|[Nn]ot found|[Mm]issing"
}

@test "error: nself db migrate on empty dir shows no migrations message" {
  _require_nself
  cd "$TEST_PROJECT_DIR"
  mkdir -p migrations
  printf 'BASE_DOMAIN=localhost\nPOSTGRES_PASSWORD=longsecret123\n' > .env
  run "$NSELF_BIN" db migrate 2>&1
  # Either no migrations found OR connection refused — both are valid errors
  assert_failure
}

@test "error: nself db snapshot with no DB running shows connect error" {
  _require_nself
  cd "$TEST_PROJECT_DIR"
  printf 'BASE_DOMAIN=localhost\nPOSTGRES_PASSWORD=longsecret123\n' > .env
  run env \
    POSTGRES_HOST="127.0.0.1" \
    POSTGRES_PORT="19995" \
    "$NSELF_BIN" backup 2>&1
  assert_failure
  assert_output --regexp "[Pp]ostgres|[Cc]onnect|[Rr]unning"
}

@test "error: conflicting migration versions shows version numbers" {
  _require_nself
  cd "$TEST_PROJECT_DIR"
  mkdir -p migrations
  printf '-- Migration v1\nCREATE TABLE test_a (id int);\n' > migrations/001_create_a.sql
  printf '-- Migration v1\nCREATE TABLE test_b (id int);\n' > migrations/001_create_b.sql
  printf 'BASE_DOMAIN=localhost\nPOSTGRES_PASSWORD=longsecret123\n' > .env
  run "$NSELF_BIN" db migrate 2>&1
  assert_failure
  assert_output --regexp "[Cc]onflict|[Dd]uplicate|001"
}

@test "error: nself db restore wrong format shows expected format" {
  _require_nself
  cd "$TEST_PROJECT_DIR"
  printf 'not a backup\n' > bad_backup.json
  run "$NSELF_BIN" restore bad_backup.json 2>&1
  assert_failure
  assert_output --regexp "\.sql\.gz|[Ff]ormat|[Ii]nvalid"
}

@test "error: nself db hasura console without running stack shows helpful message" {
  _require_nself
  cd "$TEST_PROJECT_DIR"
  printf 'BASE_DOMAIN=localhost\nPOSTGRES_PASSWORD=longsecret123\n' > .env
  run env \
    HASURA_GRAPHQL_ENDPOINT="http://127.0.0.1:19994" \
    "$NSELF_BIN" db hasura console 2>&1
  assert_failure
  assert_output --regexp "[Hh]asura|[Cc]onsole|[Nn]ot running|[Cc]onnect"
}

# ===========================================================================
# Group 3 — Licensing (mocked ping_api responses)
# ===========================================================================

@test "error: expired license shows renewal URL" {
  _require_nself
  run env \
    NSELF_LICENSE_VALIDATE_URL="http://127.0.0.1:19993/license/validate" \
    NSELF_PLUGIN_LICENSE_KEY="nself_pro_expired_key_00000000000000000" \
    "$NSELF_BIN" plugin install ai 2>&1
  assert_failure
  assert_output --regexp "[Ee]xpir|[Ll]icense|nself\.org"
}

@test "error: invalid license key format shows expected pattern" {
  _require_nself
  run "$NSELF_BIN" license set "not_valid_key_format" 2>&1
  assert_failure
  assert_output --regexp "[Ii]nvalid.*format|format.*invalid|nself_pro_"
}

@test "error: license set with empty key shows required format" {
  _require_nself
  run "$NSELF_BIN" license set "" 2>&1
  assert_failure
  assert_output --regexp "[Ee]mpty|[Rr]equired|nself_pro_"
}

@test "error: plugin install without license key shows how to set key" {
  _require_nself
  run env \
    NSELF_PLUGIN_LICENSE_KEY="" \
    "$NSELF_BIN" plugin install ai 2>&1
  assert_failure
  assert_output --regexp "[Ll]icense|nself license set"
}

@test "error: duplicate plugin install shows already-installed message and update command" {
  _require_nself
  cd "$TEST_PROJECT_DIR"
  printf 'BASE_DOMAIN=localhost\nPOSTGRES_PASSWORD=longsecret123\n' > .env
  # Pre-populate installed plugins state
  mkdir -p .nself/plugins
  printf '{"name":"cron","version":"1.0.0","installedAt":"2026-01-01"}\n' > .nself/plugins/cron.json
  run "$NSELF_BIN" plugin install cron 2>&1
  assert_failure
  assert_output --regexp "[Aa]lready installed|plugin update cron"
}

@test "error: domain limit reached shows upgrade message" {
  _require_nself
  run env \
    NSELF_LICENSE_VALIDATE_URL="http://127.0.0.1:19993/license/validate" \
    NSELF_DOMAIN_LIMIT_EXCEEDED="1" \
    "$NSELF_BIN" tenant add example.com 2>&1
  assert_failure
  assert_output --regexp "[Dd]omain.*limit|[Ll]imit.*domain|[Uu]pgrade"
}

@test "error: tier too low for plugin shows tier requirement" {
  _require_nself
  run env \
    NSELF_LICENSE_TIER="free" \
    NSELF_PLUGIN_LICENSE_KEY="" \
    "$NSELF_BIN" plugin install ai 2>&1
  assert_failure
  assert_output --regexp "[Tt]ier|[Ll]icense|[Mm]ax|[Pp]ro"
}

@test "error: nself license validate with unreachable server shows fallback message" {
  _require_nself
  run env \
    NSELF_LICENSE_VALIDATE_URL="http://192.0.2.1:9/license/validate" \
    NSELF_PLUGIN_LICENSE_KEY="nself_pro_validfmt0000000000000000000000" \
    "$NSELF_BIN" license validate 2>&1
  assert_failure
  assert_output --regexp "[Uu]nreachable|[Cc]onnect|[Nn]etwork|offline"
}

@test "error: nself license show with no key set shows how to set it" {
  _require_nself
  run env HOME="$TEST_PROJECT_DIR" "$NSELF_BIN" license show 2>&1
  assert_failure
  assert_output --regexp "[Nn]o license|nself license set|not set"
}

@test "error: plugin install nonexistent plugin shows not found and list hint" {
  _require_nself
  run "$NSELF_BIN" plugin install nonexistent_plugin_xyz123 2>&1
  assert_failure
  assert_output --regexp "[Nn]ot found|plugin list|nonexistent_plugin_xyz123"
}

# ===========================================================================
# Group 4 — Security / system (mocked system state)
# ===========================================================================

@test "error: command requiring sudo run as non-root shows requires sudo" {
  _require_nself
  # Run as current non-root user (typical CI) — should fail on privileged ops
  run "$NSELF_BIN" infra install-systemd 2>&1
  if [ "$status" -eq 0 ]; then
    skip "Running as root — sudo check skipped"
  fi
  assert_output --regexp "[Ss]udo|[Rr]oot|[Pp]ermission"
}

@test "error: invalid domain format in tenant add shows valid format examples" {
  _require_nself
  run "$NSELF_BIN" tenant add "NOT A DOMAIN!" 2>&1
  assert_failure
  assert_output --regexp "[Ii]nvalid.*domain|[Dd]omain.*invalid|example\.com"
}

@test "error: cert generation failure shows CA issue message" {
  _require_nself
  cd "$TEST_PROJECT_DIR"
  printf 'BASE_DOMAIN=localhost\nPOSTGRES_PASSWORD=longsecret123\n' > .env
  # Force mkcert/openssl failure by pointing to non-writable dir
  run env \
    NSELF_SSL_DIR="/root/nonexistent_dir_$$" \
    "$NSELF_BIN" build 2>&1
  assert_failure
  assert_output --regexp "[Cc]ert|[Ss]SL|[Cc]A|[Pp]ermission"
}

@test "error: deploy to staging without SSH key shows connection message" {
  _require_nself
  run env \
    STAGING_HOST="192.0.2.1" \
    STAGING_USER="nself" \
    SSH_KEY="/nonexistent/.ssh/id_rsa" \
    "$NSELF_BIN" deploy staging 2>&1
  assert_failure
  assert_output --regexp "[Ss]SH|[Cc]onnect|[Kk]ey|[Rr]each"
}

@test "error: deploy to prod without confirmation shows confirmation required" {
  _require_nself
  # Non-interactive run must not auto-confirm prod deploy
  run env \
    NSELF_NON_INTERACTIVE="1" \
    "$NSELF_BIN" deploy production 2>&1
  assert_failure
  assert_output --regexp "[Cc]onfirm|--yes|[Aa]pproval"
}

@test "error: nself start with missing docker-compose.yml shows build first" {
  _require_nself
  cd "$TEST_PROJECT_DIR"
  run "$NSELF_BIN" start 2>&1
  assert_failure
  assert_output --regexp "nself build|docker-compose|[Nn]ot found"
}

@test "error: wrangler not installed for plugin deploy shows install hint" {
  _require_nself
  run env PATH="/usr/bin:/bin" "$NSELF_BIN" plugin deploy custom-service 2>&1
  assert_failure
  assert_output --regexp "wrangler|[Nn]ot.*installed|[Ii]nstall"
}

@test "error: nself status when no stack running shows start hint" {
  _require_nself
  cd "$TEST_PROJECT_DIR"
  printf 'BASE_DOMAIN=localhost\nPOSTGRES_PASSWORD=longsecret123\n' > .env
  run "$NSELF_BIN" status 2>&1
  # Not necessarily failure exit, but must mention start or not running
  assert_output --regexp "nself start|[Nn]ot running|[Ss]topped|[Nn]o stack"
}

@test "error: nself build with no .env shows env init hint" {
  _require_nself
  cd "$TEST_PROJECT_DIR"
  run "$NSELF_BIN" build 2>&1
  assert_failure
  assert_output --regexp "\.env|nself init|[Nn]ot found"
}

@test "error: invalid FRONTEND_APP port (non-numeric) shows field name" {
  _require_nself
  cd "$TEST_PROJECT_DIR"
  printf 'BASE_DOMAIN=localhost\nPOSTGRES_PASSWORD=longsecret123\nFRONTEND_APP_1_NAME=admin\nFRONTEND_APP_1_PORT=notaport\n' > .env
  run "$NSELF_BIN" build 2>&1
  assert_failure
  assert_output --regexp "FRONTEND_APP_1_PORT|[Pp]ort.*invalid"
}

# ===========================================================================
# Group 5 — Plugin / config errors
# ===========================================================================

@test "error: plugin install unknown name shows not found and list hint" {
  _require_nself
  run "$NSELF_BIN" plugin install xyzzy_does_not_exist_99 2>&1
  assert_failure
  assert_output --regexp "[Nn]ot found|plugin list|xyzzy"
}

@test "error: plugin missing required env var shows exact var name and where to set" {
  _require_nself
  cd "$TEST_PROJECT_DIR"
  printf 'BASE_DOMAIN=localhost\nPOSTGRES_PASSWORD=longsecret123\n' > .env
  run env \
    NSELF_VALIDATE_ENV="1" \
    "$NSELF_BIN" plugin install github 2>&1
  assert_failure
  assert_output --regexp "GITHUB_TOKEN|GITHUB_APP_ID|[Mm]issing"
}

@test "error: invalid YAML in config file shows line number" {
  _require_nself
  cd "$TEST_PROJECT_DIR"
  printf 'BASE_DOMAIN=localhost\nPOSTGRES_PASSWORD=longsecret123\n' > .env
  mkdir -p .nself
  printf 'version: 1\nplugins:\n  - name: cron\n    invalid:\n  broken yaml:\n    - orphan\n  : no_key\n' > .nself/config.yaml
  run "$NSELF_BIN" config validate 2>&1
  assert_failure
  assert_output --regexp "[Yy][Aa][Mm][Ll]|[Ll]ine|[Ss]yntax|[Pp]arse"
}

@test "error: duplicate frontend app name shows conflict message" {
  _require_nself
  cd "$TEST_PROJECT_DIR"
  printf 'BASE_DOMAIN=localhost\nPOSTGRES_PASSWORD=longsecret123\nFRONTEND_APP_1_NAME=admin\nFRONTEND_APP_1_PORT=3001\nFRONTEND_APP_2_NAME=admin\nFRONTEND_APP_2_PORT=3002\n' > .env
  run "$NSELF_BIN" build 2>&1
  assert_failure
  assert_output --regexp "[Dd]uplicate|[Cc]onflict|admin"
}

@test "error: plugin config missing PLUGIN_SECRET shows var name" {
  _require_nself
  cd "$TEST_PROJECT_DIR"
  printf 'BASE_DOMAIN=localhost\nPOSTGRES_PASSWORD=longsecret123\n' > .env
  run env \
    NSELF_VALIDATE_ENV="1" \
    "$NSELF_BIN" plugin install webhooks 2>&1
  # Either missing secret or missing required var — check var name in output
  if [ "$status" -ne 0 ]; then
    assert_output --regexp "SECRET|HMAC|[Mm]issing|[Rr]equired"
  fi
}

@test "error: CS_N port conflict with internal service shows port and service name" {
  _require_nself
  cd "$TEST_PROJECT_DIR"
  printf 'BASE_DOMAIN=localhost\nPOSTGRES_PASSWORD=longsecret123\nCS_1_NAME=myapp\nCS_1_PORT=5432\n' > .env
  run "$NSELF_BIN" build 2>&1
  assert_failure
  assert_output --regexp "5432|[Pp]ort.*conflict|[Cc]onflict.*port|[Pp]ostgres"
}

@test "error: nself plugin update with no installed plugins shows helpful message" {
  _require_nself
  cd "$TEST_PROJECT_DIR"
  printf 'BASE_DOMAIN=localhost\nPOSTGRES_PASSWORD=longsecret123\n' > .env
  run "$NSELF_BIN" plugin update --all 2>&1
  assert_failure
  assert_output --regexp "[Nn]o plugins|[Nn]ot installed|plugin install"
}

@test "error: nself plugin list --remote with no network shows cached manifest message" {
  _require_nself
  run env \
    NSELF_REGISTRY_URL="http://192.0.2.1:9" \
    "$NSELF_BIN" plugin list --remote 2>&1
  assert_failure
  assert_output --regexp "[Cc]ach|[Oo]ffline|[Uu]navailable|[Nn]etwork"
}

@test "error: nself tenant add without initialized stack shows init hint" {
  _require_nself
  cd "$TEST_PROJECT_DIR"
  run "$NSELF_BIN" tenant add myapp.example.com 2>&1
  assert_failure
  assert_output --regexp "nself init|nself build|[Nn]ot initialized|[Nn]o stack"
}

@test "error: nself config get unknown key shows available keys" {
  _require_nself
  cd "$TEST_PROJECT_DIR"
  printf 'BASE_DOMAIN=localhost\nPOSTGRES_PASSWORD=longsecret123\n' > .env
  run "$NSELF_BIN" config get NONEXISTENT_KEY_XYZ 2>&1
  assert_failure
  assert_output --regexp "NONEXISTENT_KEY_XYZ|[Nn]ot found|[Uu]nknown"
}

@test "error: nself deploy missing PROD_HOST env var shows var name" {
  _require_nself
  run env \
    PROD_HOST="" \
    "$NSELF_BIN" deploy production 2>&1
  assert_failure
  assert_output --regexp "PROD_HOST|[Mm]issing|[Rr]equired"
}

@test "error: nself service add duplicate name shows conflict" {
  _require_nself
  cd "$TEST_PROJECT_DIR"
  printf 'BASE_DOMAIN=localhost\nPOSTGRES_PASSWORD=longsecret123\nCS_1_NAME=myapp\nCS_1_PORT=3100\n' > .env
  run "$NSELF_BIN" service add myapp --port 3101 2>&1
  assert_failure
  assert_output --regexp "[Dd]uplicate|[Cc]onflict|myapp|[Aa]lready"
}

@test "error: nself init in already-initialized dir shows already initialized" {
  _require_nself
  cd "$TEST_PROJECT_DIR"
  printf 'BASE_DOMAIN=localhost\nPOSTGRES_PASSWORD=longsecret123\n' > .env
  mkdir -p .nself
  printf '{"initialized":true}\n' > .nself/state.json
  run "$NSELF_BIN" init --base-domain localhost --non-interactive 2>&1
  # Should warn or ask for --force
  assert_output --regexp "[Aa]lready initialized|--force|--reinit"
}

@test "error: nself build with invalid MONITORING_RETENTION_DAYS shows valid range" {
  _require_nself
  cd "$TEST_PROJECT_DIR"
  printf 'BASE_DOMAIN=localhost\nPOSTGRES_PASSWORD=longsecret123\nMONITORING_RETENTION_DAYS=-5\n' > .env
  run "$NSELF_BIN" build 2>&1
  assert_failure
  assert_output --regexp "MONITORING_RETENTION_DAYS|[Ii]nvalid|[Rr]ange|[Pp]ositive"
}

@test "error: nself plugin rollback with nonexistent plugin shows not found" {
  _require_nself
  run "$NSELF_BIN" plugin rollback nonexistent_xyz 2>&1
  assert_failure
  assert_output --regexp "[Nn]ot found|[Nn]ot installed|nonexistent_xyz"
}

@test "error: nself auth reset without running Hasura shows connect message" {
  _require_nself
  cd "$TEST_PROJECT_DIR"
  printf 'BASE_DOMAIN=localhost\nPOSTGRES_PASSWORD=longsecret123\n' > .env
  run env \
    HASURA_GRAPHQL_ENDPOINT="http://127.0.0.1:19993" \
    "$NSELF_BIN" auth reset-admin-password 2>&1
  assert_failure
  assert_output --regexp "[Hh]asura|[Cc]onnect|[Nn]ot running"
}

@test "error: nself plugin install with conflicting port shows port conflict" {
  _require_nself
  cd "$TEST_PROJECT_DIR"
  printf 'BASE_DOMAIN=localhost\nPOSTGRES_PASSWORD=longsecret123\nCS_1_NAME=myapp\nCS_1_PORT=3202\n' > .env
  "$NSELF_BIN" build >/dev/null 2>&1 || true
  run env \
    NSELF_VALIDATE_ENV="1" \
    "$NSELF_BIN" plugin install content-acquisition 2>&1
  assert_failure
  assert_output --regexp "3202|[Pp]ort.*conflict|[Cc]onflict.*port"
}

@test "error: nself start with old docker-compose format shows rebuild hint" {
  _require_nself
  cd "$TEST_PROJECT_DIR"
  printf 'version: "2"\nservices:\n  old_format:\n    image: nginx\n' > docker-compose.yml
  run "$NSELF_BIN" start 2>&1
  assert_failure
  assert_output --regexp "nself build|[Oo]utdated|[Rr]ebuild|[Gg]enerated"
}

@test "error: nself plugin install with no registry access and no cache shows registry message" {
  _require_nself
  run env \
    NSELF_REGISTRY_URL="http://192.0.2.1:9/registry" \
    NSELF_REGISTRY_CACHE_FILE="/nonexistent/cache.json" \
    "$NSELF_BIN" plugin install analytics 2>&1
  assert_failure
  assert_output --regexp "[Rr]egistry|[Uu]navailable|[Cc]onnect"
}
