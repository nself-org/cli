#!/usr/bin/env bash
# Purpose: prove the CLI-owned ACME path (trust ssl setup|renew --acme, DNS-01)
#          end to end against a local Pebble CA on a real Linux docker host
#          (Constitution 10.3, final gate G12): issue, adopt certbot lineages,
#          renew/--force/--staging, crash safety, the unit's ExecStart under
#          `env -i`, and that no credential reaches argv, a file or a log.
# Inputs:  NSELF_BIN (a linux nself binary; default: built with go when found),
#          NSELF_PEBBLE_WORK (scratch dir the docker daemon can bind-mount;
#          default: a fresh mktemp dir, removed on success).
# Outputs: exit 0 and a final `ACME-PEBBLE: PASS` line only when every check passed.
# Constraints: Linux + docker + age + openssl. Talks only to Pebble and
#          challtestsrv containers (test-only, digest-pinned); never to a real
#          CA, DNS provider or production host. Bash 3.2 compatible syntax.
set -euo pipefail

PEBBLE_IMG=ghcr.io/letsencrypt/pebble@sha256:c156cabea562e43ed0060bddc03539c07de40e311b52c6b0e5ea710a496e56b8          # 2.7.0
CHALL_IMG=ghcr.io/letsencrypt/pebble-challtestsrv@sha256:26789c714fc40ab2de771fb34c06d13232b8ca4f96f77995483d5dfe8744f276 # 2.7.0
NGINX_IMG=nginx@sha256:5616878291a2eed594aee8db4dade5878cf7edcb475e59193904b198d9b830de                                  # 1.29-alpine
TOKEN=cf-fake-0123456789-ABCDEFGHIJKLMNOP

cd "$(dirname "$0")/../.."
fail() { echo "ACME-PEBBLE: FAIL: $*" >&2; exit 1; }
ok() { echo "ok: $*"; }
[ "$(uname -s)" = Linux ] || fail "needs a Linux docker host (got $(uname -s)); see Constitution 10.3"
for t in docker age age-keygen openssl curl; do command -v "$t" >/dev/null || fail "$t is required"; done
docker info >/dev/null 2>&1 || fail "docker daemon is not reachable"
uname -a

OWN_WORK=0
if [ -z "${NSELF_PEBBLE_WORK:-}" ]; then NSELF_PEBBLE_WORK=$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/acme-pebble.XXXXXX"); OWN_WORK=1; fi
WORK=$NSELF_PEBBLE_WORK; mkdir -p "$WORK"; WORK=$(cd "$WORK" && pwd -P)
for f in a c d e home letsencrypt age-key.txt ps.log all.log out.txt; do rm -rf "${WORK:?}/$f"; done # a rerun starts clean
if [ -z "${NSELF_BIN:-}" ]; then
  command -v go >/dev/null || fail "set NSELF_BIN to a linux nself binary (no go on PATH)"
  NSELF_BIN=$WORK/nself; CGO_ENABLED=0 go build -mod=vendor -o "$NSELF_BIN" ./cmd/nself
fi
NET=acme-pebble-$$; CTRS=""
PSPID=""
cleanup() {
  [ -n "$PSPID" ] && kill "$PSPID" 2>/dev/null || true
  for c in $CTRS; do docker rm -f "$c" >/dev/null 2>&1 || true; done
  docker network rm "$NET" >/dev/null 2>&1 || true
}
trap cleanup EXIT
docker network create "$NET" >/dev/null

# --- Pebble, challtestsrv and the DNS script lego's exec provider runs -------
start() { n=$1; shift; docker run -d --name "$n" "$@" >/dev/null; CTRS="$CTRS $n"; }
start "challtestsrv-$$" --network "$NET" --network-alias challtestsrv "$CHALL_IMG" -defaultIPv6 "" -defaultIPv4 10.0.0.9 -http01 "" -https01 "" -tlsalpn01 ""
# Pebble's stock config also offers a 6-day "shortlived" profile that lego may pick; keep only the 90-day default.
cat > "$WORK/pebble-config.json" <<'JSON'
{"pebble":{"listenAddress":"0.0.0.0:14000","managementListenAddress":"0.0.0.0:15000","certificate":"test/certs/localhost/cert.pem","privateKey":"test/certs/localhost/key.pem","httpPort":5002,"tlsPort":5001,"ocspResponderURL":"","externalAccountBindingRequired":false,"domainBlocklist":["blocked-domain.example"],"retryAfter":{"authz":3,"order":5},"profiles":{"default":{"description":"90 days","validityPeriod":7776000}}}}
JSON
start "pebble-$$" --network "$NET" --network-alias pebble -e PEBBLE_VA_NOSLEEP=1 -e PEBBLE_WFE_NONCEREJECT=0 -v "$WORK/pebble-config.json:/pebble-config.json:ro" "$PEBBLE_IMG" -config /pebble-config.json -dnsserver challtestsrv:8053
docker cp "pebble-$$:/test/certs/pebble.minica.pem" "$WORK/minica.pem"
cat > "$WORK/dns.sh" <<'DNS'
#!/bin/sh
# lego exec provider: $1 present|cleanup, $2 fqdn, $3 value.
case "$1" in
  present) wget -qO- --post-data "{\"host\":\"$2\",\"value\":\"$3\"}" http://challtestsrv:8055/set-txt >/dev/null ;;
  cleanup) wget -qO- --post-data "{\"host\":\"$2\"}" http://challtestsrv:8055/clear-txt >/dev/null ;;
esac
DNS
chmod 755 "$WORK/dns.sh"
age-keygen -o "$WORK/age-key.txt" 2>/dev/null; chmod 600 "$WORK/age-key.txt"
HOOKS="NSELF_ACME_DIRECTORY=https://pebble:14000/dir NSELF_ACME_CA_BUNDLE=$WORK/minica.pem NSELF_ACME_NETWORK=$NET NSELF_ACME_DNS_RESOLVERS=challtestsrv:8053 EXEC_PATH=$WORK/dns.sh"
unset ENV
export SECRETS_AGE_KEY_PATH=$WORK/age-key.txt HOME=$WORK/home; mkdir -p "$HOME"
for kv in $HOOKS; do export "${kv?}"; done
for i in $(seq 1 30); do docker logs "pebble-$$" 2>&1 | grep "Listening on" >/dev/null && break; sleep 1; done

# Sample every process's argv for the whole run (the credential must never show).
( while :; do ps -eww -o args= 2>/dev/null | grep -E 'nself|docker|lego|age |dns\.sh' >> "$WORK/ps.log" || true; sleep 0.1; done ) & PSPID=$!

# --- helpers -------------------------------------------------------------------
# selfsigned <certdir> <days> <name>...: a self-signed pair in <certdir> (fullchain.pem, privkey.pem).
selfsigned() {
  d=$1; days=$2; shift 2; san=""; for n in "$@"; do san="$san,DNS:$n"; done
  mkdir -p "$d"
  openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -keyout "$d/privkey.pem" -out "$d/fullchain.pem" \
    -days "$days" -subj "/CN=$1" -addext "subjectAltName=${san#,}" 2>/dev/null
  chmod 600 "$d/privkey.pem" "$d/fullchain.pem"
}
# conf <stack> <file> <certdir> <server_name>...: an nginx 443 server block.
conf() {
  s=$1; f=$2; cd_=$3; shift 3
  printf 'server {\n  listen 443 ssl;\n  server_name %s;\n  ssl_certificate /etc/nginx/ssl/certificates/%s/fullchain.pem;\n  ssl_certificate_key /etc/nginx/ssl/certificates/%s/privkey.pem;\n  location / { return 200 "ok"; }\n}\n' "$*" "$cd_" "$cd_" > "$s/nginx/conf.d/$f.conf"
}
# mkstack <id> <port> <base-domain>: <WORK>/<id>/nself-web (served stack) + backend project.
mkstack() {
  s=$WORK/$1/nself-web; mkdir -p "$s/ssl/certificates" "$s/nginx/conf.d" "$s/backend"
  printf 'NGINX_BIND_IP=127.0.0.1\nNGINX_HTTPS_PORT=%s\n' "$2" > "$s/.env"
  printf 'BASE_DOMAIN=%s\nENV=dev\nPROJECT_NAME=backend\nADMIN_EMAIL=ops@pebble.test\nNGINX_FRONTED_BY=nself-web\n' "$3" > "$s/backend/.env"
}
# startnginx <id> <port>: the stack's nginx, labelled the way compose labels it.
startnginx() {
  s=$WORK/$1/nself-web
  start "nginx-$1-$$" --network "$NET" -p "127.0.0.1:$2:443" --label com.docker.compose.service=nginx \
    --label "com.docker.compose.project.working_dir=$s" --label com.docker.compose.project=nself-web \
    -v "$s/ssl:/etc/nginx/ssl:ro" -v "$s/nginx/conf.d:/etc/nginx/conf.d:ro" "$NGINX_IMG"
  for i in $(seq 1 20); do docker exec "nginx-$1-$$" nginx -t >/dev/null 2>&1 && return 0; sleep 1; done
  fail "nginx for $1 did not start"
}
# nself <id> <args...>: run `nself trust ssl <args>` in the stack's project dir; stdout+stderr to $OUT, status to $RC.
nself() { id=$1; shift; set +e; (cd "$WORK/$id/nself-web/backend" && "$NSELF_BIN" trust ssl "$@") > "$WORK/out.txt" 2>&1; RC=$?; set -e; cat "$WORK/out.txt" >> "$WORK/all.log"; }
must() { nself "$@"; [ "$RC" -eq 0 ] || { cat "$WORK/out.txt" >&2; fail "nself trust ssl $* exited $RC"; }; }
out_has() { grep -qF -- "$1" "$WORK/out.txt" || { cat "$WORK/out.txt" >&2; fail "output lacks: $1"; }; }
leaf_fp() { openssl x509 -in "$1" -noout -fingerprint -sha256; }
served_fp() { echo | openssl s_client -connect "127.0.0.1:$2" -servername "$1" 2>/dev/null | openssl x509 -noout -fingerprint -sha256; }
snap() { (cd "$1" && find . | sort && find . -type f -exec sha256sum {} + | sort); }
link() { readlink "$WORK/$1/nself-web/ssl/certificates/$2"; }
cert() { echo "$WORK/$1/nself-web/ssl/certificates/$2/fullchain.pem"; }
pairmatch() { [ "$(openssl x509 -in "$1/fullchain.pem" -noout -pubkey | openssl sha256)" = "$(openssl pkey -in "$1/privkey.pem" -pubout | openssl sha256)" ]; }

# === A. prod-layout dry run, then issue (wildcard + apex = two names) ===========
mkstack a 18443 task.pebble.test
A=$WORK/a/nself-web
selfsigned "$A/ssl/certificates/task-pebble-test" 20 task.pebble.test
conf "$A" task task-pebble-test '*.task.pebble.test' task.pebble.test
startnginx a 18443
CID_A=$(docker inspect -f '{{.Id}}' "nginx-a-$$"); OLD_FP=$(served_fp task.pebble.test 18443)
snap "$WORK/a" > "$WORK/a.before"
must a setup --acme --provider exec --wildcard --dry-run
for want in "served root:       $A" "served ssl dir:    $A/ssl" "served nginx dir:  $A/nginx" "nginx container:   nginx-a-$$" \
  "$A/ssl -> /etc/nginx/ssl" "lineage task-pebble-test: dns-01 via exec" "certificates/task-pebble-test" "dry run: nothing written"; do out_has "$want"; done
snap "$WORK/a" | cmp -s - "$WORK/a.before" || fail "dry run changed the tree"
ok "A1 prod-layout dry run prints the resolution and writes nothing"
must a setup --acme --provider exec --wildcard --agree-tos
out_has "served certificate verified"
[ "$(link a task-pebble-test)" = ".task-pebble-test.gen-1" ] || fail "target is not generation 1: $(link a task-pebble-test)"
[ "$(served_fp task.pebble.test 18443)" = "$(leaf_fp "$(cert a task-pebble-test)")" ] || fail "served fingerprint is not the installed certificate"
[ "$(served_fp task.pebble.test 18443)" != "$OLD_FP" ] || fail "nginx still serves the old certificate"
openssl x509 -in "$(cert a task-pebble-test)" -noout -ext subjectAltName | grep -q '\*\.task\.pebble\.test' || fail "wildcard SAN missing"
[ -f "$A/ssl/.acme/.gitignore" ] && [ "$(cat "$A/ssl/.acme/.gitignore")" = '*' ] || fail ".acme/.gitignore missing"
[ "$(stat -c %a "$A/ssl/.acme")" = 700 ] && [ "$(stat -c %a "$A/ssl/.acme/lineages.json")" = 600 ] || fail "state permissions"
grep -q '"challenge": "dns-01"' "$A/ssl/.acme/lineages.json" || fail "lineages.json lacks the lineage"
[ "$(docker inspect -f '{{.Id}}' "nginx-a-$$")" = "$CID_A" ] || fail "nginx container was recreated"
ok "A2 issued over DNS-01, installed as a generation, nginx reloaded, served fingerprint matches"

# === B. renew: not due, kill before the switch, recovery =======================
G=$(link a task-pebble-test)
must a renew --acme; out_has "due: false"; [ "$(link a task-pebble-test)" = "$G" ] || fail "a not-due lineage was renewed"
ok "B1 a lineage with more than 30 days left is skipped"
set +e; (cd "$A/backend" && NSELF_ACME_FAULT=after-generation-write "$NSELF_BIN" trust ssl renew --acme --force) > "$WORK/out.txt" 2>&1; RC=$?; set -e
[ "$RC" -eq 137 ] || fail "fault hook exit status $RC, want 137"
[ "$(link a task-pebble-test)" = "$G" ] || fail "link moved despite the kill"
pairmatch "$A/ssl/certificates/task-pebble-test" || fail "key and certificate mismatch after the kill"
docker exec "nginx-a-$$" nginx -t >/dev/null 2>&1 || fail "nginx -t fails after the kill"
must a renew --acme --force
[ "$(link a task-pebble-test)" != "$G" ] || fail "recovery run did not switch"
pairmatch "$A/ssl/certificates/task-pebble-test" && docker exec "nginx-a-$$" nginx -t >/dev/null 2>&1 || fail "pair or nginx -t after recovery"
[ "$(served_fp task.pebble.test 18443)" = "$(leaf_fp "$(cert a task-pebble-test)")" ] || fail "served fingerprint after recovery"
ok "B2 kill between generation write and link rename leaves a matching pair; the next run recovers"

# === C. due (10 d), not due (60 d), --force, --staging ==========================
mkstack c 18444 due.pebble.test
C=$WORK/c/nself-web
selfsigned "$C/ssl/certificates/due-pebble-test" 10 due.pebble.test
selfsigned "$C/ssl/certificates/ok-pebble-test" 60 ok.pebble.test
conf "$C" due due-pebble-test due.pebble.test; conf "$C" ok ok-pebble-test ok.pebble.test
mkdir -p "$C/ssl/.acme"; chmod 700 "$C/ssl/.acme"; printf '*\n' > "$C/ssl/.acme/.gitignore"
lin() { printf '{"name":"%s","domains":["%s"],"challenge":"dns-01","dns_provider":"exec","credential_secrets":[],"targets":["certificates/%s"],"key_type":"ec256","adopted_from":null,"last_issued":null}' "$1" "$2" "$1"; }
printf '{"schema_version":"1","contact":"ops@pebble.test","lineages":[%s,%s]}\n' "$(lin due-pebble-test due.pebble.test)" "$(lin ok-pebble-test ok.pebble.test)" > "$C/ssl/.acme/lineages.json"
chmod 600 "$C/ssl/.acme/lineages.json"
startnginx c 18444
must c renew --acme --agree-tos
[ -L "$C/ssl/certificates/due-pebble-test" ] && [ -d "$C/ssl/certificates/ok-pebble-test" ] && [ ! -L "$C/ssl/certificates/ok-pebble-test" ] || fail "only the 10-day lineage should have renewed"
[ "$(served_fp due.pebble.test 18444)" = "$(leaf_fp "$(cert c due-pebble-test)")" ] || fail "renewed lineage not served"
ok "C1 renew --acme renews only the lineage with 30 days or fewer left"
H1=$(sha256sum "$(cert c due-pebble-test)" "$(cert c ok-pebble-test)"); L1=$(link c due-pebble-test)
must c renew --acme --staging --agree-tos
ls "$C"/ssl/.acme/staging/certificates/*.crt >/dev/null 2>&1 || fail "no staging certificate under .acme/staging"
[ "$(sha256sum "$(cert c due-pebble-test)" "$(cert c ok-pebble-test)")" = "$H1" ] && [ "$(link c due-pebble-test)" = "$L1" ] || fail "--staging installed something"
ok "C2 --staging writes under .acme/staging and installs nothing"
must c renew --acme --force
[ -L "$C/ssl/certificates/ok-pebble-test" ] && [ "$(link c due-pebble-test)" != "$L1" ] || fail "--force did not renew both"
[ "$(served_fp ok.pebble.test 18444)" = "$(leaf_fp "$(cert c ok-pebble-test)")" ] || fail "--force result not served"
ok "C3 --force renews every lineage"

# === D. adopt a certbot tree: dns-cloudflare + standalone -> dns-01 =============
mkstack d 18445 adopt.pebble.example # not .test: the served-host reader skips .test names
D=$WORK/d/nself-web; LE=$WORK/letsencrypt
for n in dns sa; do
  h=$n.adopt.pebble.example; dir=$n-adopt-pebble-example
  selfsigned "$D/ssl/certificates/$dir" 20 "$h"
  mkdir -p "$LE/live/$h" "$LE/renewal"; cp "$D/ssl/certificates/$dir/fullchain.pem" "$D/ssl/certificates/$dir/privkey.pem" "$LE/live/$h/"
  cp "$LE/live/$h/fullchain.pem" "$LE/live/$h/cert.pem"; conf "$D" "$n" "$dir" "$h"
done
printf 'version = 2.9.0\n[renewalparams]\nauthenticator = dns-cloudflare\ndns_cloudflare_credentials = /etc/letsencrypt/cf.ini\n' > "$LE/renewal/dns.adopt.pebble.example.conf"
printf 'version = 2.9.0\n[renewalparams]\nauthenticator = standalone\npre_hook = docker stop nginx\npost_hook = docker start nginx\n' > "$LE/renewal/sa.adopt.pebble.example.conf"
printf '# certbot cloudflare\ndns_cloudflare_api_token = %s\n' "$TOKEN" > "$WORK/cf.ini"
startnginx d 18445
CID_D=$(docker inspect -f '{{.Id}}' "nginx-d-$$")
snap "$D/ssl/certificates" > "$WORK/d.served"; snap "$D/nginx" > "$WORK/d.nginx"; snap "$LE" > "$WORK/d.le"
nself d setup --acme --adopt-certbot="$LE"
[ "$RC" -ne 0 ] && grep -q -- '--dns-credential-file' "$WORK/out.txt" && grep -q 'refused sa.adopt.pebble.example' "$WORK/out.txt" && ! grep -q 'no served nginx conf' "$WORK/out.txt" || { cat "$WORK/out.txt"; fail "standalone lineage was not refused with a remediation"; }
[ ! -e "$D/ssl/.acme/lineages.json" ] || fail "a refused adoption wrote lineages.json"
ok "D1 without a credential the standalone lineage is refused with a remediation"
must d setup --acme --adopt-certbot="$LE" --dns-credential-file="$WORK/cf.ini"
[ "$(grep -c '"challenge": "dns-01"' "$D/ssl/.acme/lineages.json")" -eq 2 ] || fail "both lineages should be dns-01"
age -d -i "$WORK/age-key.txt" "$D/backend/.secrets/dev.age" | grep -q SSL_DNS_CLOUDFLARE_API_TOKEN || fail "credential not in the age store under its name"
snap "$D/ssl/certificates" | cmp -s - "$WORK/d.served" || fail "served files changed by adoption"
snap "$D/nginx" | cmp -s - "$WORK/d.nginx" || fail "nginx confs changed by adoption"
snap "$LE" | cmp -s - "$WORK/d.le" || fail "certbot state changed by adoption"
[ "$(docker inspect -f '{{.Id}}' "nginx-d-$$")" = "$CID_D" ] || fail "nginx container changed"
rm -f "$WORK/cf.ini"
ok "D2 both lineages adopted as dns-01; credential in the age store; served files, confs, container and certbot untouched"

# === E. the unit's ExecStart under env -i renews a due lineage ===================
mkstack e 18446 unit.pebble.test
E=$WORK/e/nself-web
selfsigned "$E/ssl/certificates/unit-pebble-test" 5 unit.pebble.test; conf "$E" unit unit-pebble-test unit.pebble.test
mkdir -p "$E/ssl/.acme"; chmod 700 "$E/ssl/.acme"; printf '*\n' > "$E/ssl/.acme/.gitignore"
printf '{"schema_version":"1","contact":"ops@pebble.test","lineages":[%s]}\n' "$(lin unit-pebble-test unit.pebble.test)" > "$E/ssl/.acme/lineages.json"; chmod 600 "$E/ssl/.acme/lineages.json"
cp -r "$A/ssl/.acme/accounts" "$E/ssl/.acme/accounts"
startnginx e 18446
must e setup --acme --install-cron --dry-run
UNIT=$WORK/out.txt.unit; cp "$WORK/out.txt" "$UNIT"
EXECSTART=$(sed -n 's/^ExecStart=//p' "$UNIT"); WD=$(sed -n 's/^WorkingDirectory=//p' "$UNIT")
ENVS=$(sed -n 's/^Environment=//p' "$UNIT" | tr '\n' ' ')
case "$ENVS" in *SECRETS_AGE_KEY_PATH=*HOME=*|*HOME=*SECRETS_AGE_KEY_PATH=*) ;; *) fail "unit lacks SECRETS_AGE_KEY_PATH/HOME: $ENVS";; esac
[ "$WD" = "$E/backend" ] || fail "unit WorkingDirectory is $WD"
set +e
UOUT=$(cd "$WD" && env -i PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin $ENVS $HOOKS $EXECSTART 2>&1); URC=$?
set -e
[ "$URC" -eq 0 ] || { echo "$UOUT" >&2; fail "unit ExecStart under env -i exited $URC"; }
[ -z "$UOUT" ] || fail "--quiet printed output: $UOUT"
[ -L "$E/ssl/certificates/unit-pebble-test" ] && [ "$(served_fp unit.pebble.test 18446)" = "$(leaf_fp "$(cert e unit-pebble-test)")" ] || fail "unit run did not renew the due lineage"
ok "E the unit's ExecStart under env -i renews a due lineage quietly"

# === F. no credential in argv, files or logs ======================================
sleep 1; kill "$PSPID" 2>/dev/null || true; PSPID=""
grep -qF "$TOKEN" "$WORK/ps.log" && fail "credential value seen in a process argv"
[ -s "$WORK/ps.log" ] || fail "ps capture is empty"
if grep -rqF --exclude='*.age' --exclude=ps.log "$TOKEN" "$WORK"; then fail "credential value found in a file or log"; fi
ok "F no credential in argv (ps capture), any file other than the age store, or any log"
echo "ACME-PEBBLE: PASS"
[ "$OWN_WORK" = 1 ] && rm -rf "$WORK" || true
