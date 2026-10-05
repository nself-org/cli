# Plugin Development Guide

Build a new ɳSelf plugin. Plugins extend the ɳSelf stack with additional Docker services, Nginx routes, and CLI commands.

## Plugin Structure

A plugin is a directory (or Git repository) containing:

```
my-plugin/
├── manifest.json       # plugin metadata and declarations
├── compose.yml         # Docker Compose overlay
├── nginx.conf          # Nginx location blocks (optional)
└── README.md
```

## Manifest Format

```json
{
  "name": "my-plugin",
  "version": "1.0.0",
  "description": "Short description of what the plugin does.",
  "tier": "free",
  "compose": "compose.yml",
  "nginx": "nginx.conf",
  "env_vars": [
    {
      "key": "MY_PLUGIN_PORT",
      "description": "Port the plugin service listens on",
      "default": "3100",
      "required": true
    },
    {
      "key": "MY_PLUGIN_SECRET",
      "description": "Shared secret for internal auth",
      "secret": true
    }
  ]
}
```

**Tier values:** `free` | `basic` | `pro` | `elite` | `business` | `enterprise`

## Compose Overlay

The `compose.yml` defines Docker services to add to the stack. ɳSelf merges this into the generated `docker-compose.yml` at build time:

```yaml
services:
  my-plugin:
    image: myorg/my-plugin:1.0.0
    restart: unless-stopped
    environment:
      MY_PLUGIN_SECRET: "${MY_PLUGIN_SECRET}"
    ports:
      - "127.0.0.1:${MY_PLUGIN_PORT}:3100"
    networks:
      - default
```

All services must:
- Bind to `127.0.0.1` on the host (not `0.0.0.0`)
- Use `restart: unless-stopped`
- Connect to the `default` network

## Connection URL Variables

Docker Compose substitutes a variable into a URL verbatim and cannot percent-encode it. A password containing `/`, `@`, `:`, `?`, `#` or `%` turns `postgresql://user:${POSTGRES_PASSWORD}@postgres:5432/db` into a URL that parses with the wrong host, and the client fails with "Invalid URL" or an authentication error.

`nself build` therefore writes two extra variables into `.nself/compose.env`, with the password encoded for the userinfo part of a URL (RFC 3986, the same encoding as `config.URLUserInfo`):

| Variable | Raw twin | Written when |
|---|---|---|
| `POSTGRES_PASSWORD_URLENC` | `POSTGRES_PASSWORD` | the Postgres password is set |
| `REDIS_PASSWORD_URLENC` | `REDIS_PASSWORD` | the Redis password is set |

Use the nested-default form wherever a password sits inside a URL:

```yaml
environment:
  - DATABASE_URL=postgresql://${POSTGRES_USER}:${POSTGRES_PASSWORD_URLENC:-${POSTGRES_PASSWORD}}@postgres:5432/${POSTGRES_DB}
  - REDIS_URL=redis://:${REDIS_PASSWORD_URLENC:-${REDIS_PASSWORD}}@redis:6379
```

- Substitute the encoded variable exactly once. Never wrap it in another encoding step: `%40` becomes `%2540`.
- The fallback keeps a `compose.env` written by an older CLI working: without the encoded variable, compose renders the raw password, exactly as before.
- Use the raw variables (`POSTGRES_PASSWORD`, `REDIS_PASSWORD`) for anything that is not a URL, such as `POSTGRES_PASSWORD` for the Postgres container itself or a `--requirepass` argument.
- Only hand an encoded URL to something that percent-decodes the password (database drivers and client libraries do). `redis-cli -u` does not: it answers `WRONGPASS` to the encoded form. For `redis-cli` in a healthcheck or script, use `-a` with the raw `REDIS_PASSWORD` instead.
- Paid fragments that read `${DATABASE_URL}` need no change: `DATABASE_URL` is already encoded.
- `nself doctor` warns (`url-reserved-password`) when a password holds a reserved character and an installed fragment still embeds the raw variable in a URL. The warning names the variable, never the value.

## Nginx Injection

Add Nginx location blocks to route external traffic to your plugin:

```nginx
location /my-plugin/ {
    proxy_pass http://my-plugin:3100/;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
}
```

ɳSelf appends this block to the generated Nginx server config for your domain.

## Env Var Declaration

All env vars your plugin needs must be declared in the manifest. ɳSelf:
1. Validates required vars are present before build
2. Auto-generates secret vars (if `"secret": true` and not already set)
3. Injects declared vars into the compose overlay

## Testing Locally

```bash
# Link a local directory as a shadow override — nself build prefers it
# over the registry version until you `nself plugin unlink`
nself plugin link ./my-plugin/

# Rebuild and start
nself build && nself restart

# Check it's running
nself status
nself urls
```

## Publishing Checklist

Before submitting a PR to the plugins repository:

- [ ] `manifest.json` validates (run `nself plugin validate ./my-plugin/`)
- [ ] Plugin installs cleanly on a fresh `nself init`
- [ ] All declared env vars are documented
- [ ] Compose overlay uses `127.0.0.1` port binding
- [ ] Nginx config tested (no syntax errors)
- [ ] `README.md` covers: what it does, env vars, usage

**Free plugins:** submit a PR to [nself-org/plugins](https://github.com/nself-org/plugins).
**Pro plugins:** contact the ɳSelf team.

## See Also

- [[Plugin-Architecture]], how plugins integrate with the core
- [[Plugin-Overview]], existing plugin catalogue
- [[Contributing]], general contribution guidelines

---
← [[Home]] | [[_Sidebar]]
