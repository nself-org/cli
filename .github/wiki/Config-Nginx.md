# Nginx Configuration

Nginx is the only service in a ɳSelf stack that accepts external connections. Every other service, Postgres, Hasura, Auth, Storage, Redis, and all optional services, binds to `127.0.0.1` inside the Docker network and is reachable only through Nginx. This single-entry-point design keeps the attack surface small and means SSL termination, routing, and access control are all configured in one place.

This page covers every `NGINX_*` and `SSL_*` environment variable, how `BASE_DOMAIN` drives subdomain generation, the four SSL modes, and which Nginx files are generated (leave alone) versus which are safe to hand-edit.

---

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `NGINX_VERSION` | `alpine` | Docker image tag for the Nginx container. `alpine` tracks the latest stable Alpine-based release. Pin to a specific tag (e.g., `1.27-alpine`) if you need version stability. |
| `NGINX_HTTP_PORT` / `NGINX_PORT` | `80` | Port Nginx listens on for HTTP traffic. Both names are accepted , `NGINX_PORT` is the legacy alias. |
| `NGINX_HTTPS_PORT` / `NGINX_SSL_PORT` | `443` | Port Nginx listens on for HTTPS traffic. `NGINX_SSL_PORT` is the legacy alias. |
| `NGINX_BIND_IP` | `127.0.0.1` (dev) / `0.0.0.0` (prod) | IP address Nginx binds to on the host. The default changes automatically based on `ENV`: `127.0.0.1` in dev restricts access to localhost only; `0.0.0.0` in prod allows external traffic. Override this explicitly if you need non-default behaviour. |
| `NGINX_CLIENT_MAX_BODY_SIZE` | `100M` | Maximum allowed size for a single request body. Affects file uploads to Storage and any API endpoint. Use standard Docker memory notation: `50M`, `1G`, etc. |
| `NGINX_GZIP_ENABLED` | `true` | Enables gzip compression on responses. Set to `false` if a CDN or upstream proxy is handling compression. |
| `NGINX_MODE` | *(empty)* | Set to `shared` to enable multi-project mode, where a single Nginx instance routes traffic for multiple ɳSelf projects on the same host. Leave empty for standard single-project operation. |
| `NGINX_MEM_LIMIT` | `256m` | Docker memory limit for the Nginx container. |
| `NGINX_FRONTED_BY` | *(empty)* | Names another stack whose Nginx already fronts this project's domains, for a project that has no ingress Nginx of its own. See [Fronted by another stack](#fronted-by-another-stack) below. |
| `SSL_MODE` | `local` | SSL mode to use. One of `local`, `letsencrypt`, `custom`, or `none`. See SSL Modes below. |
| `EXTRA_SSL_DOMAINS` | *(empty)* | Additional domains to include in the SSL certificate as Subject Alternative Names (SANs). Comma-separated. Useful when your project is also reachable under an alias domain. |
| `BASE_DOMAIN` | `local.nself.org` | Root domain for your project. Nginx generates all service subdomains from this value automatically. See Domain Configuration below. |

---

## Domain Configuration

`BASE_DOMAIN` is the single variable that drives all subdomain routing for your stack. You set the root domain once and ɳSelf derives every service URL from it.

```bash
# .env.dev
BASE_DOMAIN=myproject.example.com
```

With that single setting, `nself build` generates Nginx virtual host entries for:

| Subdomain | Service |
|-----------|---------|
| `api.myproject.example.com` | Hasura GraphQL (controlled by `HASURA_ROUTE`) |
| `auth.myproject.example.com` | Authentication service (controlled by `AUTH_ROUTE`) |
| `storage.myproject.example.com` | Object storage (controlled by `STORAGE_ROUTE`) |

Each service's `*_ROUTE` variable lets you override the subdomain prefix if the default does not match your naming convention:

```bash
# Change the Hasura subdomain from "api" to "graphql"
HASURA_ROUTE=graphql
# Result: graphql.myproject.example.com
```

For local development the default `BASE_DOMAIN=local.nself.org` resolves to `127.0.0.1` via a public DNS wildcard, so you get working HTTPS URLs with no `/etc/hosts` changes required.

---

## SSL Modes

Set `SSL_MODE` in your env file to tell ɳSelf how to handle SSL certificates.

### `local` (default)

ɳSelf generates a self-signed certificate and key in `ssl/certificates/<domain-dir>/` at the project root, where `<domain-dir>` is `BASE_DOMAIN` with dots replaced by dashes (`local.nself.org` becomes `local-nself-org`). The certificate is scoped to `*.{BASE_DOMAIN}` so all subdomains are covered. Your browser will show a security warning that you must accept once.

Use this mode for local development. Do not use it in production.

### `letsencrypt`

ɳSelf provisions a certificate from Let's Encrypt automatically using the ACME challenge. The domain must be publicly reachable on port 80 at the time of provisioning. Certificates are stored in `ssl/certificates/<domain-dir>/` and renewed automatically before expiry. Until a certificate pair exists there, `nself build` emits no HTTPS server blocks for this mode.

Use this mode for staging and production servers with public domains. Requires that `BASE_DOMAIN` is a real domain pointing at your server.

```bash
# .env.prod
SSL_MODE=letsencrypt
BASE_DOMAIN=myproject.example.com
```

To cover additional domain aliases at the same time, add them as SANs:

```bash
EXTRA_SSL_DOMAINS=www.myproject.example.com,myproject.io
```

### `custom`

You provide your own certificate files. Place them at the paths Nginx expects (`<domain-dir>` is `BASE_DOMAIN` with dots replaced by dashes):

```
ssl/certificates/<domain-dir>/fullchain.pem   ← full chain certificate
ssl/certificates/<domain-dir>/privkey.pem     ← private key
```

`nself build` emits HTTPS server blocks only once both files exist. OCSP stapling is not configured: Let's Encrypt certificates carry no OCSP URL, so a `chain.pem` in the directory is ignored.

ɳSelf will not touch these files. Renewal and rotation are your responsibility.

Use this mode when your organisation manages certificates centrally (e.g., via Vault, Certbot in a separate process, or a corporate PKI).

### `none`

Nginx serves HTTP only, no SSL. All traffic is unencrypted.

This is only appropriate for internal services on isolated networks where encryption is handled at a higher layer (e.g., an outer load balancer or VPN). Do not use `none` for any internet-facing service.

---

## Nginx File Structure

ɳSelf writes and owns most Nginx files. Understanding which files are generated and which are yours to edit prevents lost customisations and config conflicts.

### Generated files, do not hand-edit

These files are regenerated every time `nself build` runs. Any manual changes will be overwritten.

```
nginx/
  nginx.conf          ← Main Nginx configuration
  sites/              ← Per-service virtual host configs (one file per service)
    hasura.conf
    auth.conf
    storage.conf
    ...
  conf.d/default.conf ← Default server (HTTP to HTTPS redirect, /health)
  includes/
    rate-limits.conf
ssl/                  ← project root, mounted read-only at /etc/nginx/ssl
  certificates/<domain-dir>/
    fullchain.pem     ← managed by nSelf, or provided by you in custom mode
    privkey.pem
```

If you want to change a value you see in `nginx/nginx.conf` or `nginx/sites/*.conf`, find the corresponding environment variable and set it in your `.env` file instead. The generated files are always the output, the env vars are the input.

### Safe to hand-edit, your customisation layer

```
nginx/
  conf.d/             ← Custom Nginx config snippets
    *.conf            ← Any .conf files here are included after generated config
```

Files in `nginx/conf.d/` are included by the main `nginx.conf` after all generated configuration, and they are **never touched by `nself build`**. The one exception is `conf.d/default.conf`, which build writes. If a hand-managed file serves a name that ɳSelf would also generate, your file wins: the generated `nginx/sites/` file is left out and `.nself/generated/routes.json` marks that route `shadowed_by` your file (see [[Nginx-Generation]]). This is the right place for:

- Custom rate limiting rules
- Additional proxy pass entries for services outside the ɳSelf stack
- Header modifications (`add_header`, `more_set_headers`)
- Redirect rules
- Custom log formats

Create as many `.conf` files as you like in `nginx/conf.d/`. They are included in alphabetical order. The directory itself is preserved across every `nself build` run.

Example, adding a custom header to all responses:

```nginx
# nginx/conf.d/security-headers.conf
add_header X-Content-Type-Options nosniff always;
add_header X-Frame-Options SAMEORIGIN always;
add_header Referrer-Policy strict-origin-when-cross-origin always;
```

---

## Internal Routing

For services that are not part of the standard ɳSelf stack, you can define additional Nginx upstreams using the `INTERNAL_ROUTE_N` variable group. These routes tell Nginx to proxy a subdomain to a custom upstream target, a Docker service, an internal IP, or any host reachable from within the Nginx container.

Up to 20 internal routes are supported (N = 1 through 20).

| Variable | Description |
|----------|-------------|
| `INTERNAL_ROUTE_N_NAME` | Human-readable identifier for the route (used in generated config comments) |
| `INTERNAL_ROUTE_N_SUBDOMAIN` | Subdomain prefix under `BASE_DOMAIN` that Nginx will route to this upstream |
| `INTERNAL_ROUTE_N_TARGET` | Upstream target as a full URL, including the scheme (e.g., `http://myservice:3000`, `http://hasura:8080`). A bare `host:port` is rejected by `nself build`. |

Example, route `metrics.myproject.example.com` to a Grafana container on port 3000:

```bash
# .env.dev
INTERNAL_ROUTE_1_NAME=grafana
INTERNAL_ROUTE_1_SUBDOMAIN=metrics
INTERNAL_ROUTE_1_TARGET=http://grafana:3000
```

After running `nself build`, Nginx will proxy all traffic for `metrics.{BASE_DOMAIN}` to the `grafana` container on port 3000. SSL is applied automatically based on the configured `SSL_MODE`.

Internal routes are generated into `nginx/sites/` and follow the same "do not hand-edit" rule as all other generated files. To modify a route, change its `INTERNAL_ROUTE_N_*` variables and rebuild.

---

## Fronted by Another Stack

Some hosts run more than one ɳSelf project, with a single project's Nginx serving the domains of the others. Ports 80 and 443 can only be held once, so every other project on that host must not generate an Nginx of its own.

Set `NGINX_FRONTED_BY` to the name of the stack that fronts this one:

```bash
# .env — this project is served by the nself-web stack's Nginx
NGINX_FRONTED_BY=nself-web
```

With this set, `nself build` omits the Nginx service from the generated `docker-compose.yml` entirely, and `nself status` stops counting an Nginx container for this project. Leaving it unset is the default and changes nothing for a normal single-project stack.

Without it, a fronted project still generates an Nginx that can never start: `nself start` fails its port check with 80/443 held by the fronting stack's `docker-proxy`, and `nself status` reports one permanently unhealthy service that nothing can clear.

**This flag removes the container that cannot run. It does not wire the two projects' networks together.** The fronting Nginx resolves upstreams through Docker's embedded DNS at `127.0.0.11`, which only answers for container names on a network that Nginx is attached to. Routing `auth.task.example.com` to this project's `auth` container still requires the two projects to share a Docker network — declared as `external` in both compose files, or attached by hand. Automatically rewriting a project's network topology from a single flag is a larger change and is deliberately not part of this one.

---

## Security Notes

**Nginx is the sole public entry point.** No other container publishes a port that is reachable from outside the host. This is enforced at the Docker Compose level, all services use `expose` (internal only) rather than `ports` (host binding). The only exception is Postgres in development when `POSTGRES_EXPOSE_PORT` is enabled for local database tools.

In production, verify your firewall allows inbound traffic only on ports 80 and 443. There is no need to open any other port to the internet.

**HTTP to HTTPS redirect** is enabled automatically when HTTPS server blocks are emitted: always in `local` mode, and in `letsencrypt` or `custom` mode once the certificate pair for `BASE_DOMAIN` exists. `none` never redirects. Requests arriving on port 80 receive a `301` redirect to the HTTPS equivalent.

---

← [[Config-Auth]] | [[Configuration]] | [[Config-Optional-Services]] →
