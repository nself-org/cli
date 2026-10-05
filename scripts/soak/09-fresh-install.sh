#!/usr/bin/env bash
# T09 — fresh-install smoke test on a clean Hetzner CX23.
# Requires HETZNER_NSELF_TOKEN in env. Provisions a disposable CX23, installs
# nself, runs `nself init --demo && nself start`, asserts end-to-end in <5min,
# then destroys the box.

set -u

HCLOUD_TOKEN="${HETZNER_NSELF_TOKEN:-${HCLOUD_TOKEN:-}}"
SSH_KEY_NAME="${SSH_KEY_NAME:-soak-key}"
SERVER_NAME="soak-fresh-$(date +%s)"
MAX_SECS="${MAX_SECS:-300}"

if [ -z "$HCLOUD_TOKEN" ]; then
  printf "FAIL T09: HETZNER_NSELF_TOKEN not set\n"
  exit 2
fi

if ! command -v hcloud >/dev/null 2>&1; then
  printf "FAIL T09: hcloud CLI not installed\n"
  exit 2
fi

export HCLOUD_TOKEN

printf "T09: provisioning %s (CX23, fsn1, debian-12)\n" "$SERVER_NAME"
hcloud server create \
  --name "$SERVER_NAME" \
  --type cx23 \
  --image debian-12 \
  --location fsn1 \
  --ssh-key "$SSH_KEY_NAME" \
  --start-after-create 2>&1 | tee /tmp/soak-provision.$$.log

ip="$(hcloud server ip "$SERVER_NAME" 2>/dev/null || echo '')"
if [ -z "$ip" ]; then
  printf "FAIL T09: could not get IP for %s\n" "$SERVER_NAME"
  hcloud server delete "$SERVER_NAME" 2>/dev/null || true
  exit 1
fi

# Wait for SSH.
for i in $(seq 1 30); do
  if ssh -o StrictHostKeyChecking=no -o ConnectTimeout=5 "root@$ip" true 2>/dev/null; then
    break
  fi
  sleep 5
done

# Run the install + smoke on the remote box, timed.
start="$(date +%s)"
ssh -o StrictHostKeyChecking=no "root@$ip" bash -s <<'REMOTE' | tee /tmp/soak-remote.$$.log
set -e
apt-get update -qq
apt-get install -y -qq curl ca-certificates jq
curl -fsSL https://install.nself.org | bash
export PATH="/root/.nself/bin:$PATH"
mkdir -p /opt/demo && cd /opt/demo
nself init --demo
# start already runs health checks by default (skip with --skip-health-checks,
# tune the wait with --timeout) — there is no separate --wait-healthy flag.
nself start
# Bare JSON in 1.4.x, v1 envelope with NSELF_V15=1: `.data // .` reads both.
nself status --json | jq -e '(.data // .).summary.unhealthy == 0'
echo "REMOTE_OK"
REMOTE
rc=$?
end="$(date +%s)"
elapsed=$((end - start))

# Tear down.
hcloud server delete "$SERVER_NAME" 2>/dev/null || true

if [ "$rc" -ne 0 ]; then
  printf "FAIL T09: remote install exited %s\n" "$rc"
  exit 1
fi

if [ "$elapsed" -gt "$MAX_SECS" ]; then
  printf "FAIL T09: install took %ss (> %ss budget)\n" "$elapsed" "$MAX_SECS"
  exit 1
fi

printf "OK T09: fresh-install smoke = %ss (budget %ss)\n" "$elapsed" "$MAX_SECS"
