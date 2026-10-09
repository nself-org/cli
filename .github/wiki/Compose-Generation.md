# Compose Generation

`nself build` is the heart of the CLI. It reads your `.env` files, resolves which services to enable, and generates `docker-compose.yml` and nginx configs. You should run it whenever your config changes.

---

> **The Golden Rule**
>
> Never hand-edit `docker-compose.yml`. It is regenerated on every `nself build`. Any manual changes will be lost. If you need to customize a service, use env vars or the `.env.local` override file.

---

## Build Sequence

When you run `nself build`, the CLI executes the following steps in order:

1. **Load env cascade**, reads `.env.dev` → `.env.{ENV}` → `.env.secrets` → `.env.local` → `.env`, with later files overriding earlier ones
2. **Apply smart defaults**, fills in any unset variables with safe, sensible values
3. **Normalize values**, resolves ENV aliases, normalizes boolean flags, sanitizes `PROJECT_NAME` for Docker compatibility
4. **Run security validation**, checks password strength, CORS origins, port bindings, and other security-sensitive settings; aborts with a clear error on any violation
5. **Detect enabled services**, reads all `*_ENABLED` environment variables to determine which services should be included
6. **Run change detection**, compares file modification times to skip regeneration if nothing has changed (use `--force` to bypass)
7. **Generate SSL certificates**, uses mkcert if available, falls back to OpenSSL; skips if valid certs already exist
8. **Generate nginx configuration**, writes all nginx config files based on enabled services and SSL state
9. **Generate `docker-compose.yml`**, assembles the full compose file from all enabled service definitions
10. **Apply post-processing**, configures log drivers, graceful shutdown timeouts, and security hardening appropriate for the current environment
11. **Validate the generated compose**, runs `docker compose config` to verify the output is well-formed
12. **Write `.env.computed`**, records derived values (resolved ports, computed flags, etc.) for use by downstream tooling

---

## Change Detection and Smart Cache

`nself build` is smart, it only regenerates files that actually need updating:

- If `.env` is newer than `docker-compose.yml` → regenerate compose
- If `.env` is newer than `nginx/nginx.conf` → regenerate nginx
- If SSL certs are missing or expired → regenerate SSL
- If the CLI version has changed → rebuild everything from scratch

This makes repeated builds fast. Use `--force` to bypass the cache and rebuild everything unconditionally.

---

## Multi-File Compose Merge

After a build, `.nself/compose-files.txt` lists the generated Compose files in merge order. `.nself/compose-env-files.txt` lists the absolute paths to the interpolation files in order: `.env`, then the computed `.nself/compose.env` when present. Consumers should pass each path with `docker compose --env-file` before the `-f` files. The env manifest contains paths only; it is empty for legacy projects without `.nself/compose.env`.

When plugins are installed, each one provides a `docker-compose.plugin.yml` overlay file. During `nself build`, these overlay files are merged into the final `docker-compose.yml` in a well-defined order:

1. **Base compose**, core required services
2. **Optional service sections**, Redis, MinIO, mailpit, and other optional services if enabled
3. **Monitoring bundle**, Prometheus, Grafana, Loki, and the rest of the monitoring stack (if `MONITORING_ENABLED=true`)
4. **Custom services**, `CS_1` through `CS_10` as defined in your env
5. **Plugin overlay files**, one per installed plugin, applied in install order

Plugin overlays can add new services, volumes, and networks. They cannot remove or rename services that already exist in the base compose. This ensures the core stack remains stable regardless of which plugins are installed.

---

## Service Generation Order

The compose file is assembled in this order. Services later in the list can depend on services earlier in the list.

1. **Core**, `postgres`, `hasura`, `auth`, `nginx`
2. **Optional**, `minio` (with init container), `redis`, `mailpit`, `admin`, `functions`, `mlflow`, `search`
3. **Monitoring**, `prometheus`, `grafana`, `loki`, `promtail`, `tempo`, `alertmanager`, `cadvisor`, `node-exporter`, `postgres-exporter`, `redis-exporter`
4. **Custom**, `CS_1` through `CS_10`
5. **Plugins**, loaded from `~/.nself/plugins/`, in install order

---

## Security Hardening

In `staging` and `prod` environments, `nself build` automatically applies security hardening to every generated service definition:

- `cap_drop: ALL`, drops all Linux capabilities by default
- `security_opt: no-new-privileges: true`, prevents privilege escalation
- `cap_add: NET_BIND_SERVICE`, re-adds only the capability that services actually need

These are applied automatically based on your `ENV` setting. You do not need to configure them. In `dev`, hardening is relaxed to avoid friction during local development.

---

## Common Commands

```bash
nself build                   # Rebuild (only regenerates changed files)
nself build --force           # Force full rebuild, bypassing the cache
nself build --check           # Validate config without writing any files
nself build --security-report # Show the full security validation report
```

---

**See also:** [[Architecture]] | [[Config-System]] | [[Nginx-Generation]] | [[Plugin-Architecture]] | [[Home]]
