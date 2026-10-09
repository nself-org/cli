# Deploy tiers and targets

The control plane reads `.nself/control-plane.yaml` at schema version 2. Version 1 files migrate in memory when loaded; `nself env target migrate` writes the version 2 file. A missing file synthesizes one local environment and one remote environment per `NSELF_DEPLOY_HOST_<ENV>` variable.

Each environment has a `tier`: `local`, `local-servers`, or `prod`. When omitted, `kind: local` derives `local`, `prod` or `production` derives `prod`, and every other name derives `local-servers`. A custom name such as `live` with `tier: prod` receives the same production confirmation gate. Exactly one local environment is required. A prod environment needs a host and exactly one primary server. Server names are unique across the inventory.

```yaml
schema_version: 2
project: example
environments:
  local:
    name: local
    kind: local
    tier: local
    servers:
      - {name: local-app, role: app, primary: true}
  qa:
    name: qa
    kind: remote
    tier: local-servers
    require_verified_release: false
    servers:
      - {name: qa-app, role: app, host: deploy@qa.example.test:2222, remote_path: /opt/nself, primary: true, arch: amd64}
```

`host` accepts `[user@]host[:port]`, including a bracketed IPv6 literal. Put the install path in `remote_path`. Legacy `user@host:/absolute/path` and `user@host` plus `NSELF_REMOTE_PATH_<ENV>` still load. In v1.5 mode, the legacy path form warns once per process; it is removed at v1.6.0. Invalid hosts are refused before ssh starts.

`nself env target add <env> <server> --tier <tier> --arch <arch> --host <host>` adds a server. For a secret-bearing deploy or a prod target in v1.5 mode, an unknown host key is refused before any copy. Verify the reported SHA256 fingerprint independently, then run the exact `nself env target add <env> <server> --trust-host-key SHA256:<fingerprint>` command. nSelf pins the verified key in `~/.config/nself/deploy_known_hosts`; it does not write `~/.ssh/known_hosts`.

`nself deploy targets --json` emits a v1 JSON envelope with every target sorted by environment and server. Rows include tier, role, primary status, user, host, port, `ssh_hostname`, sorted DNS addresses, known-hosts fingerprints, architecture and source. The command runs `ssh -G`, DNS lookup and `ssh-keygen -F`; it never opens a connection to a deploy host. OpenSSH `ssh -G` evaluates the operator's `Match exec` configuration, which can run local operator-configured commands.

`controlplane.ResolveTargets` filters by environment, tier and server and returns matches ordered by environment, role (observability, app, load balancer, database, worker), and name. An empty result is E486.
