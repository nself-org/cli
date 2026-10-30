#!/usr/bin/env bash
# mcp-sandbox.sh — start an MCP server (normally `nself mcp`) for the agent E2E
# harness in a sealed environment (P7-SHIP-02, SHIP Epic D7).
#
# Usage:
#   mcp-sandbox.sh --home DIR --project DIR --operator-home DIR
#                  [--inventory FILE] [--allow NAME]... [--env K=V]...
#                  -- BIN [ARGS...]
#
# What it does: refuses to start (exit 1, naming the entry) when
#   - SSH_AUTH_SOCK is set (an agent socket could reach real keys),
#   - the sandbox HOME is, contains, links to or names the operator's ~/.ssh,
#   - HOME, the inventory or the ssh config names a host outside the
#     allowlist (127.0.0.1, localhost and each --allow name),
#   - an --env entry is outside the fixed list below;
# otherwise it cds into --project and execs BIN under `env -i` with only
# PATH, HOME, TMPDIR, NSELF_V15 and the allowed --env entries.
#
# Allowed --env names: NSELF_V15, NSELF_E2E_SIM_KEY (a path inside HOME),
# DOCKER_HOST (unix:// sockets only).
#
# Exit codes: 0 never returns on success (exec), 1 refused, 2 usage.
# Bash 3.2 compatible.

set -u

die_usage() { printf 'mcp-sandbox: usage: %s\n' "$1" >&2; exit 2; }
refuse() { printf 'mcp-sandbox: refused: %s\n' "$1" >&2; exit 1; }

HOME_DIR="" PROJECT="" OPHOME="" INVENTORY=""
ALLOW="127.0.0.1 localhost"
ENVS=()
while [ $# -gt 0 ]; do
  case "$1" in
    --home) HOME_DIR="${2:-}"; shift 2 ;;
    --project) PROJECT="${2:-}"; shift 2 ;;
    --operator-home) OPHOME="${2:-}"; shift 2 ;;
    --inventory) INVENTORY="${2:-}"; shift 2 ;;
    --allow) ALLOW="$ALLOW ${2:-}"; shift 2 ;;
    --env) ENVS+=("${2:-}"); shift 2 ;;
    --) shift; break ;;
    *) die_usage "unknown argument $1" ;;
  esac
done
[ -n "$HOME_DIR" ] && [ -d "$HOME_DIR" ] || die_usage "--home must be an existing directory"
[ -n "$PROJECT" ] && [ -d "$PROJECT" ] || die_usage "--project must be an existing directory"
[ -n "$OPHOME" ] || die_usage "--operator-home is required"
[ $# -gt 0 ] || die_usage "missing -- BIN"

resolve() { readlink -f "$1" 2>/dev/null || printf '%s' "$1"; }

# 1. No SSH agent.
[ -z "${SSH_AUTH_SOCK:-}" ] || refuse "SSH_AUTH_SOCK is set; unset it so no agent key is reachable"

# 2. HOME must not reach the operator's ~/.ssh.
H=$(resolve "$HOME_DIR")
OP=$(resolve "$OPHOME")
OPSSH="$OP/.ssh"
[ "$H" != "$OP" ] || refuse "HOME $HOME_DIR is the operator's home"
case "$H/" in "$OPSSH"/*) refuse "HOME $HOME_DIR is inside the operator's ~/.ssh" ;; esac
while IFS= read -r link; do
  [ -n "$link" ] || continue
  target=$(resolve "$link")
  case "$target/" in
    "$OPSSH"/*|"$OP"/) refuse "$link reaches the operator's ~/.ssh ($target)" ;;
  esac
  case "$target/" in "$H"/*) ;; *) refuse "$link points outside HOME ($target)" ;; esac
done <<EOF
$(find "$HOME_DIR" -type l 2>/dev/null)
EOF

# 3. Files to scan: every regular file in HOME, plus the inventory.
FILES=$(find "$HOME_DIR" -type f 2>/dev/null)
if [ -n "$INVENTORY" ]; then
  [ -f "$INVENTORY" ] || die_usage "--inventory $INVENTORY does not exist"
  FILES="$FILES
$INVENTORY"
fi

allowed() {
  for a in $ALLOW; do [ "$1" = "$a" ] && return 0; done
  return 1
}

check_host() { # file host
  h="$2"
  h="${h#*@}"                       # user@host
  case "$h" in \[*\]*) h="${h#\[}"; h="${h%%\]*}" ;; *) h="${h%%:*}" ;; esac
  h=$(printf '%s' "$h" | tr -d '"'"'"' ')
  [ -n "$h" ] || return 0
  allowed "$h" || refuse "$1 names host $h outside the allowlist ($ALLOW)"
}

while IFS= read -r f; do
  [ -n "$f" ] || continue
  if grep -qF "$OPSSH" "$f" 2>/dev/null; then
    refuse "$f names the operator's ~/.ssh"
  fi
  # IPv4 literals anywhere.
  for ip in $(grep -oE '([0-9]{1,3}\.){3}[0-9]{1,3}' "$f" 2>/dev/null); do
    check_host "$f" "$ip"
  done
  # ssh config Host / HostName / ProxyJump lines; Include, Match and
  # ProxyCommand are refused outright (they can reach anything).
  for h in $(awk 'tolower($1)=="host"||tolower($1)=="hostname"||tolower($1)=="proxyjump"{for(i=2;i<=NF;i++)print $i}' "$f" 2>/dev/null); do
    check_host "$f" "$h"
  done
  kw=$(awk 'tolower($1)=="include"||tolower($1)=="match"||tolower($1)=="proxycommand"{print $1; exit}' "$f" 2>/dev/null)
  [ -z "$kw" ] || refuse "$f uses $kw, which can reach hosts or config outside the sandbox"
  # Inventory host fields (YAML `host: ...`).
  for h in $(sed -n 's/^[[:space:]-]*host:[[:space:]]*//p' "$f" 2>/dev/null); do
    check_host "$f" "$h"
  done
done <<EOF
$FILES
EOF

# 4. The environment the server gets.
V15=1
EXTRA=()
for kv in ${ENVS[@]+"${ENVS[@]}"}; do
  k="${kv%%=*}"; v="${kv#*=}"
  case "$k" in
    NSELF_V15) V15="$v" ;;
    NSELF_E2E_SIM_KEY)
      case "$(resolve "$v")/" in "$H"/*) EXTRA+=("$kv") ;; *) refuse "--env $k must be a path inside HOME" ;; esac ;;
    DOCKER_HOST)
      case "$v" in unix://*) EXTRA+=("$kv") ;; *) refuse "--env DOCKER_HOST must be a unix:// socket" ;; esac ;;
    *) refuse "--env $k is not on the sandbox list" ;;
  esac
done

SAFE_PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
[ -d /opt/homebrew/bin ] && SAFE_PATH="/opt/homebrew/bin:$SAFE_PATH"
TMP_DIR="$HOME_DIR/.tmp"
mkdir -p "$TMP_DIR" || refuse "cannot create $TMP_DIR"

cd "$PROJECT" || refuse "cannot enter $PROJECT"
exec env -i PATH="$SAFE_PATH" HOME="$HOME_DIR" TMPDIR="$TMP_DIR" NSELF_V15="$V15" \
  ${EXTRA[@]+"${EXTRA[@]}"} "$@"
