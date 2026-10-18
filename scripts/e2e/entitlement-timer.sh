#!/usr/bin/env bash
# Purpose: end-to-end proof of the unattended entitlement refresh timer (P7-PLUG-64) on Linux.
# Inputs: --cases a,b,c,d,e,negative  --backends systemd,cron  [--nself LINUX_BINARY]  [--keep]
#   systemd backend: a privileged debian:12 + systemd container (scripts/e2e/entitlement-timer/Dockerfile),
#     the timer is installed by root and runs as the project owner (uid 1000).
#   cron backend: the same image without systemd, everything as the non-root user "app".
# Outputs: one PASS/FAIL line per case and backend; exit 1 on any failure. Containers are removed on exit.
# Constraints: needs docker, go (to build the binary unless --nself is given) and $TMPDIR. No ping runs:
#   the cases need none, and the negative case points NSELF_PING_API_URL at a closed port.
# Bash 3.2 compatible.
set -euo pipefail

cases="a,b,c,d,e,negative"
backends="systemd,cron"
bin=""
keep=0
while [ $# -gt 0 ]; do
  case "$1" in
    --cases) cases="$2"; shift 2 ;;
    --backends) backends="$2"; shift 2 ;;
    --nself) bin="$2"; shift 2 ;;
    --keep) keep=1; shift ;;
    *) echo "usage: $0 [--cases a,b,c,d,e,negative] [--backends systemd,cron] [--nself BIN] [--keep]" >&2; exit 2 ;;
  esac
done
: "${TMPDIR:?TMPDIR must be set}"

root=$(cd "$(dirname "$0")/../.." && pwd)
img=nself-e2e-entitlement-timer
work=$(mktemp -d "${TMPDIR%/}/entitlement-timer.XXXXXX")
fails=0
containers=""

cleanup() {
  local c
  for c in $containers; do docker rm -f "$c" >/dev/null 2>&1 || true; done
  if [ "$keep" = 0 ]; then rm -rf "$work"; else echo "kept $work"; fi
}
trap cleanup EXIT

pass() { echo "PASS $1 [$2]"; }
fail() { echo "FAIL $1 [$2]: $3"; fails=$((fails + 1)); }
has() { case ",$cases," in *",$1,"*) return 0 ;; esac; return 1; }

build_binary() {
  [ -n "$bin" ] && return 0
  local arch
  arch=$(docker info --format '{{.Architecture}}')
  case "$arch" in aarch64|arm64) arch=arm64 ;; x86_64|amd64) arch=amd64 ;; esac
  bin="$work/nself"
  (cd "$root" && GOOS=linux GOARCH="$arch" CGO_ENABLED=0 go build -mod=vendor -o "$bin" ./cmd/nself)
}

start_container() { # name systemd|plain
  local name=$1
  docker rm -f "$name" >/dev/null 2>&1 || true
  containers="$containers $name"
  if [ "$2" = systemd ]; then
    docker run -d --name "$name" --privileged --cgroupns=host --tmpfs /run --tmpfs /run/lock \
      -v /sys/fs/cgroup:/sys/fs/cgroup:rw "$img" >/dev/null
    local i state
    for i in $(seq 1 60); do
      state=$(docker exec "$name" systemctl is-system-running 2>/dev/null || true)
      case "$state" in running|degraded) break ;; esac
      sleep 1
    done
  else
    docker run -d --name "$name" --entrypoint sleep "$img" infinity >/dev/null
  fi
  docker cp "$bin" "$name:/usr/local/bin/nself"
  docker exec "$name" sh -c 'chown root:root /usr/local/bin/nself && chmod 755 /usr/local/bin/nself'
}

# reset: remove every timer, recreate the binary mode, the licensed plugin (for root and app) and the project.
reset() { # container backend
  local c=$1 plugin='{"manifest_version":2,"license":"licensed","name":"chat","version":"1.0.0","description":"fixture"}'
  docker cp "$bin" "$c:/usr/local/bin/nself"
  docker exec "$c" sh -c "
    nself license refresh --remove-timer --all >/dev/null 2>&1 || true
    su app -c 'nself license refresh --remove-timer --all >/dev/null 2>&1' || true
    su app -c 'crontab -r' >/dev/null 2>&1 || true
    systemctl unset-environment NSELF_ENTITLEMENT_AUTOREFRESH NSELF_PING_API_URL >/dev/null 2>&1 || true
    rm -rf /home/app/proj /home/app/.nself /root/.nself
    for h in /root /home/app; do
      mkdir -p \$h/.nself/plugins/chat
      printf '%s' '$plugin' > \$h/.nself/plugins/chat/plugin.json
      printf 'services:\n  chat:\n    image: fixture:latest\n' > \$h/.nself/plugins/chat/docker-compose.plugin.yml
    done
    mkdir -p /home/app/proj && printf 'PROJECT_NAME=e2e\nENV=dev\n' > /home/app/proj/.env
    chown -R app:app /home/app
    chown root:root /usr/local/bin/nself && chmod 755 /usr/local/bin/nself"
}

# two_builds: the timer is installed by two builds (E125 is expected: no ping), exactly one timer results.
two_builds() { # container backend
  local c=$1 b=$2 user=root n
  [ "$b" = cron ] && user=app
  local i
  for i in 1 2; do
    docker exec -u "$user" -w /home/app/proj -e NSELF_V15=1 -e NSELF_PING_API_URL=http://127.0.0.1:1 "$c" nself build >/dev/null 2>&1 || true
  done
  n=$(count_timers "$c" "$b")
  [ "$n" = 1 ] || { echo "expected one timer after two builds, found $n"; return 1; }
}

count_timers() { # container backend
  if [ "$2" = systemd ]; then
    docker exec "$1" sh -c 'ls /etc/systemd/system/nself-entitlement-*.timer 2>/dev/null | wc -l' | tr -d ' '
  else
    docker exec -u app "$1" sh -c "crontab -l 2>/dev/null | grep -c '# nself-entitlement:' || true" | tr -d ' '
  fi
}

# run_timer: one timer invocation; prints the exit status. extra env goes in $3 as NAME=value.
run_timer() { # container backend [env]
  local c=$1 b=$2 envp=${3:-}
  if [ "$b" = systemd ]; then
    [ -n "$envp" ] && docker exec "$c" systemctl set-environment "$envp"
    local unit
    unit=$(docker exec "$c" sh -c 'basename $(ls /etc/systemd/system/nself-entitlement-*.service | head -1)')
    docker exec "$c" systemctl start "$unit" >/dev/null 2>&1 && echo 0 || echo 1
  else
    # the crontab line through sh, cron's own unescaping of \% included
    docker exec -u app ${envp:+-e "$envp"} "$c" sh -c "line=\$(crontab -l | grep '# nself-entitlement:' | cut -d' ' -f6- | sed 's/\\\\%/%/g'); sh -c \"\$line\"" \
      >/dev/null 2>&1 && echo 0 || echo 1
  fi
}

wait_gone() { # container backend
  local i
  for i in 1 2 3 4 5 6 7 8 9 10; do
    [ "$(count_timers "$1" "$2")" = 0 ] && return 0
    sleep 1
  done
  return 1
}

units_absent() {
  local files timers
  files=$(docker exec "$1" sh -c 'ls /etc/systemd/system/nself-entitlement-* 2>/dev/null | wc -l' | tr -d ' ')
  timers=$(docker exec "$1" sh -c 'systemctl list-timers --all --no-pager | grep -c nself-entitlement || true' | tr -d ' ')
  [ "$files" = 0 ] && [ "$timers" = 0 ]
}

removal_case() { # container backend case
  local c=$1 b=$2 k=$3 rc env=""
  reset "$c" "$b"
  two_builds "$c" "$b" || { fail "$k" "$b" "install"; return; }
  case "$k" in
    a) docker exec "$c" rm -rf /home/app/.nself/plugins/chat ;;
    b) docker exec "$c" rm -rf /home/app/proj ;;
    c) docker exec "$c" rm -f /usr/local/bin/nself ;;
    d) env="NSELF_ENTITLEMENT_AUTOREFRESH=false" ;;
    e) if [ "$b" = systemd ]; then docker exec "$c" nself license refresh --remove-timer --project /home/app/proj >/dev/null
       else docker exec -u app "$c" nself license refresh --remove-timer --project /home/app/proj >/dev/null; fi ;;
  esac
  if [ "$k" != e ]; then rc=$(run_timer "$c" "$b" "$env"); [ "$rc" = 0 ] || { fail "$k" "$b" "timer run exited $rc, want 0"; return; }; fi
  wait_gone "$c" "$b" || { fail "$k" "$b" "timer still present"; return; }
  if [ "$b" = systemd ]; then units_absent "$c" || { fail "$k" "$b" "unit files or list-timers entry remain"; return; }; fi
  pass "$k" "$b"
}

negative_case() { # container backend
  local c=$1 b=$2 rc
  reset "$c" "$b"
  two_builds "$c" "$b" || { fail negative "$b" "install"; return; }
  if [ "$b" = systemd ]; then docker exec "$c" systemctl set-environment NSELF_PING_API_URL=http://127.0.0.1:1; rc=$(run_timer "$c" "$b")
  else rc=$(run_timer "$c" "$b" "NSELF_PING_API_URL=http://127.0.0.1:1"); fi
  [ "$rc" != 0 ] || { fail negative "$b" "ping unreachable but the run exited 0"; return; }
  [ "$(count_timers "$c" "$b")" = 1 ] || { fail negative "$b" "a failed refresh removed the timer"; return; }
  pass negative "$b"
}

verify_units() { # container: systemd-analyze verify must exit 0 for our two units
  local out
  if ! out=$(docker exec "$1" sh -c 'systemd-analyze verify /etc/systemd/system/nself-entitlement-*.service /etc/systemd/system/nself-entitlement-*.timer 2>&1'); then
    echo "$out"
    return 1
  fi
}

run_backend() { # backend
  local b=$1 name="nself-e2e-ent-$1-$$" kind=plain k
  [ "$b" = systemd ] && kind=systemd
  start_container "$name" "$kind"
  if [ "$b" = systemd ]; then
    reset "$name" "$b"; two_builds "$name" "$b" || fail install "$b" "two builds did not give one timer"
    verify_units "$name" && pass systemd-analyze-verify "$b" || fail systemd-analyze-verify "$b" "systemd-analyze verify"
  fi
  for k in a b c d e; do
    if has "$k"; then removal_case "$name" "$b" "$k"; fi
  done
  if has negative; then negative_case "$name" "$b"; fi
}

docker build -q -t "$img" "$root/scripts/e2e/entitlement-timer" >/dev/null
build_binary
for backend in $(echo "$backends" | tr ',' ' '); do run_backend "$backend"; done
[ "$fails" = 0 ] && echo "entitlement-timer e2e: all cases passed" || { echo "entitlement-timer e2e: $fails failure(s)"; exit 1; }
