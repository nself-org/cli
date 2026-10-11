# Guide: Security Hardening

Follow this guide after deploying ɳSelf to production. For a quick checklist, see [[Security-Hardening]].

## Firewall

Close all ports except 80, 443, and 22. All internal service ports (Postgres 5432, Hasura 8080, Auth 4000) must not be reachable from the internet.

```bash
ufw allow 22/tcp
ufw allow 80/tcp
ufw allow 443/tcp
ufw default deny incoming
ufw enable
ufw status verbose
```

## Rotate Default Secrets

Before going live, replace all generated default secrets with strong random values:

```bash
# Generate strong secrets (32+ characters)
openssl rand -hex 32   # use output for each secret
```

Set in `.env.secrets`:
```env
POSTGRES_PASSWORD=<strong-random>
HASURA_GRAPHQL_ADMIN_SECRET=<strong-random>
HASURA_JWT_KEY=<strong-random>
AUTH_JWT_SECRET=<strong-random>
```

Never commit `.env.secrets` to git. Verify it is in `.gitignore`.

`nself secrets set <KEY> [VALUE]` checks a value before it reaches the store. It refuses an empty value, an unexpanded `${VAR}` reference, a bracketed placeholder such as `<strong-random>`, and a value too short for the kind of key. `nself secrets rotate <KEY>` generates a new value only for passwords; API keys and tokens must be rotated at the provider, and a rotate that cannot generate one fails and leaves the old value in place.

## Check the Stack with `nself doctor`

Run `nself doctor` after every configuration change and before going live. It lists advisory backup hints too: a hand-rolled `pg_dump` upload script, or a backup identity that was never copied off this machine (see [[Guide-Backup-Restore]]). Advisory hints never change the exit code. `nself doctor --deep` adds the subsystem checks, including plugin migrations that have not finished and a bundle ledger that disagrees with the installed plugins.

## Disable Hasura Console in Production

The Hasura GraphQL console exposes schema information and must be disabled in production:

```env
# .env.prod
HASURA_GRAPHQL_ENABLE_CONSOLE=false
```

Rebuild after changing:
```bash
nself build && nself restart
```

## Enable Rate Limiting

Add rate limiting to Auth endpoints to prevent brute-force attacks. The default rate limit is 30 requests/minute.

To customise:
```env
AUTH_RATE_LIMIT=10r/m    # 10 requests per minute
```

For WAF-level protection, install the rate-limit plugin:
```bash
nself plugin install rate-limit
```

## Keep Everything Updated

```bash
nself update          # update nSelf CLI
nself build           # regenerate configs with latest service images
nself restart         # apply updates
```

## Enable Monitoring and Alerts

See [[Guide-Monitoring-Setup]] for step-by-step instructions. Set up CPU, memory, and error-rate alerts in Alertmanager.

## See Also

- [[Security-Hardening]], production checklist
- [[Security-Architecture]], how ɳSelf is secured by design
- [[Guide-Monitoring-Setup]], enable alerts

---
← [[Home]] | [[_Sidebar]]
