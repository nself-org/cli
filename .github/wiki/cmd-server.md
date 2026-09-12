# nself server

<!-- BEGIN PROSE:summary -->
> Provision, list, resize, and destroy Hetzner Cloud servers.
<!-- END PROSE:summary -->

## Synopsis

```
nself server <subcommand> [flags]
```

## Description

<!-- BEGIN PROSE:description -->
Manage the lifecycle of a Hetzner Cloud server: create one, list what exists,
resize one, or destroy one, all without a raw `hcloud` invocation.

`nself access` manages SSH keys on an already-deployed server, and `nself
security` audits one — neither can create, resize, or destroy the server
itself. `nself server` fills that gap, and encodes the safety checks a manual
`hcloud server create` / `hcloud server delete` does not: `destroy` refuses
to run without a verified backup, protects the server's primary IP(s) from
being deleted along with it, and `resize` explains (rather than raw-errors
on) Hetzner's disk-shrink limitation.

### nself server provision
Create a new server. Every server this command creates is labeled
`managed-by=nself-cli` (unless you pass your own `--label managed-by=...`,
which is respected as-is), so `nself server list` and any future cleanup pass
can tell nself-created servers apart from anything else in the same Hetzner
project.

```bash
nself server provision --name ci-runner-3 --type cx22 --location fsn1 --image ubuntu-24.04
```

Flags: `--name` (required), `--type` (required, e.g. `cx22`), `--location`
(required, e.g. `fsn1`), `--image` (required, e.g. `ubuntu-24.04`),
`--ssh-key` (repeatable, Hetzner SSH key name to authorize), `--label`
(repeatable `key=value`), `--json`.

### nself server list
List servers in the Hetzner project, optionally filtered by label.

```bash
nself server list --label-selector managed-by=nself-cli
```

Flags: `--label-selector`, `--json`.

### nself server resize
Change a server's type (CPU/RAM/disk). Hetzner Cloud has no API to shrink a
server's disk: if `--type` names a type with a smaller disk than the server
currently has, this command refuses and explains the only supported path
(snapshot the current server, provision a new server of the smaller type,
restore from the snapshot, then `nself server destroy` the original) instead
of surfacing Hetzner's raw `invalid_input` error.

```bash
nself server resize --id 12345 --type cx41
```

Flags: `--id` (required), `--type` (required, target server type),
`--upgrade-disk` (also grow the disk to match the new type, irreversible),
`--json`.

### nself server destroy
Delete a server. Safe by default: refuses to run at all unless you pass
`--snapshot` (takes one and waits for it to reach `status=available` before
deleting anything) or `--force-no-backup` (an explicit acknowledgment that no
backup is taken). Hetzner primary IPs default to `auto_delete=true`, so
deleting the server would permanently destroy its IP too; this command sets
`auto_delete=false` on the server's primary IP(s) first and prints which IPs
were retained, unless `--release-ip` says to let them go with the server. If
the snapshot fails or never reaches `status=available` within
`--snapshot-timeout`, the server is NOT deleted.

```bash
nself server destroy --id 12345 --snapshot
```

Flags: `--id` (required), `--snapshot`, `--force-no-backup`, `--release-ip`,
`--snapshot-timeout` (default `10m`), `--json`.

Every subcommand also takes `--token` (Hetzner Cloud API token, overrides
`--token-env`) and `--token-env` (env var to read the token from, default
`HETZNER_NSELF_TOKEN`; falls back to `HCLOUD_TOKEN` if unset). The token is
never logged.
<!-- END PROSE:description -->

## Flags

<!-- BEGIN GENERATED:flags -->
| Flag | Default | Description |
|------|---------|-------------|
| `--help`, `-h` | — | Show help |
<!-- END GENERATED:flags -->

## Subcommands

<!-- BEGIN GENERATED:subcommands -->
| Name | Description |
|------|-------------|
| `destroy` | Delete a Hetzner Cloud server |
| `list` | List Hetzner Cloud servers |
| `provision` | Create a new Hetzner Cloud server |
| `resize` | Change a server's type (CPU/RAM/disk) |
<!-- END GENERATED:subcommands -->

## Examples

<!-- BEGIN PROSE:examples -->
```bash
# Provision a new CI box
nself server provision --name ci-runner-3 --type cx22 --location fsn1 --image ubuntu-24.04 --ssh-key deploy
```

```bash
# List every nself-managed server
nself server list --label-selector managed-by=nself-cli
```

```bash
# Grow a server, keeping the same disk-shrink-safe path
nself server resize --id 12345 --type cx41
```

```bash
# Destroy a server after a verified snapshot, retaining its primary IP
nself server destroy --id 12345 --snapshot
```

```bash
# Destroy a throwaway CI box with no backup, releasing its IP too
nself server destroy --id 12345 --force-no-backup --release-ip
```
<!-- END PROSE:examples -->

## See Also

<!-- BEGIN PROSE:see-also -->
- [[cmd-access]], SSH key access on a server this command already provisioned
- [[cmd-security]], firewall, fail2ban, and sshd hardening for the same server
- [[cmd-deploy]], deploying the nself stack onto a server once it exists
- [[Commands]], full command index
<!-- END PROSE:see-also -->

← [[Commands]] | [[Home]] →
