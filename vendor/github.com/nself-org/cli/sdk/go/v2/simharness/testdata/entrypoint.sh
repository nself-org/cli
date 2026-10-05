#!/bin/sh
# Purpose: start sshd for a simharness node: key auth only, one non-root user.
# Inputs:  PUBLIC_KEY (authorized_keys line), USER_NAME (default nself).
# Outputs: sshd in the foreground on port 22.
set -eu
user="${USER_NAME:-nself}"
home="$(getent passwd "$user" | cut -d: -f6)"
mkdir -p /run/sshd "$home/.ssh"
ssh-keygen -A >/dev/null
printf '%s\n' "${PUBLIC_KEY:?PUBLIC_KEY is required}" > "$home/.ssh/authorized_keys"
chown -R "$user" "$home/.ssh"
chmod 700 "$home/.ssh"
chmod 600 "$home/.ssh/authorized_keys"
exec /usr/sbin/sshd -D -e -p 22 \
  -o PasswordAuthentication=no \
  -o KbdInteractiveAuthentication=no \
  -o PubkeyAuthentication=yes \
  -o PermitRootLogin=no \
  -o AllowUsers="$user" \
  -o AuthorizedKeysFile=.ssh/authorized_keys
