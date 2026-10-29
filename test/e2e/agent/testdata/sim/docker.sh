#!/bin/sh
# Recording docker stub for the agent E2E sim hosts (P7-SHIP-02). It mirrors
# the stubs of internal/controlplane/sim: the sim hosts are sshd containers
# without a Docker engine, so `compose up` records the image set of the
# shipped docker-compose.yml in /tmp/agent-e2e/running and the read commands
# answer from that record. Image digests are derived from the reference, so
# two hosts running the same references report the same digests.
S=/tmp/agent-e2e
mkdir -p "$S"
echo "$*" >> "$S/docker.log"
images() { awk '/^    image:/{print $2}' docker-compose.yml 2>/dev/null | sort -u; }
digest() { printf '%s' "$1" | sha256sum | cut -c1-64; }
case "$*" in
  *"config --services"*) awk '/^services:/{s=1;next} s&&/^  [A-Za-z0-9_.-]+:/{sub(":","",$1);print $1} s&&/^[^ ]/{s=0}' docker-compose.yml ;;
  *"config --format json"*) printf '{"services":{}}' ;;
  *"up -d --dry-run"*) : ;;
  *" up "*|*" up") for i in $(images); do printf '%s@sha256:%s\n' "$i" "$(digest "$i")"; done > "$S/running" ;;
  *" images --format json"*)
    printf '['; sep=''
    while IFS= read -r line; do
      [ -n "$line" ] || continue
      ref=${line%@*}; d=${line#*@}
      printf '%s{"ContainerName":"app-%s-1","Repository":"%s","Tag":"%s","ID":"%s"}' "$sep" "$(digest "$ref" | cut -c1-8)" "${ref%:*}" "${ref##*:}" "$d"; sep=','
    done < "$S/running" 2>/dev/null
    printf ']' ;;
  *"ps -a --format json"*|*"ps --all --format json"*) printf '[]' ;;
  *"image inspect --format"*)
    for last; do :; done
    ref=$(grep -F "$last" "$S/running" 2>/dev/null | head -1); ref=${ref%@*}
    printf '["%s@%s"]|linux/amd64' "${ref%:*}" "$last" ;;
  "network inspect"*) exit 1 ;;
esac
exit 0
