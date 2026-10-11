# Nginx Generation

Nginx is the single entry point for all external traffic in a ɳSelf stack. Rather than asking you to write or maintain nginx configuration by hand, `nself build` generates the full nginx configuration automatically based on which services are enabled, what SSL mode you have selected, and which plugins are installed. The generated files are written to an `nginx/` directory in your project root and are consumed by the nginx container at startup.

---

## Generated Directory Structure

```
nginx/
├── nginx.conf              ← Main config (auto-generated, do not edit)
├── conf.d/
│   └── default.conf        ← HTTP→HTTPS redirect, /health endpoint
├── conf.d-dev/             ← Dev-specific overrides (auto-generated)
├── conf.d-prod/            ← Prod-specific overrides (auto-generated)
├── sites/                  ← Auto-generated service routes (one .conf per service)
│   ├── hasura.conf
│   ├── auth.conf
│   ├── storage.conf
│   └── ...
├── includes/
│   └── rate-limits.conf    ← Rate limiting zones
└── routes/                 ← Plugin-generated routes
```

`nginx.conf` is the top-level entry point. It includes everything under `conf.d/`, `conf.d-dev/` or `conf.d-prod/` (depending on environment), `sites/`, `includes/`, and `routes/`. Never edit `nginx.conf` or anything under `sites/` directly, those files are overwritten on every `nself build`.

## Proxy route contract

Each build writes `.nself/generated/routes.json`, version 1 of the proxy route contract. Its `_generated` key identifies the file as build output. The file describes the project and environment, HTTP defaults, the default server, rate zones, and every generated core, optional, custom service, frontend, and internal route. Route entries include their source, server names, TLS settings, security headers, blocked paths, upstream locations, rate limits, and any `shadowed_by` hand-managed file. Routes and zones have stable ordering. Hostnames, ports, and paths are included; credentials are excluded.

The nginx renderer reads this model. A route suppressed by a hand-managed `conf.d` file remains in the JSON with `shadowed_by` set, while its generated `nginx/sites/` file is omitted. Plugin snippets and hand-managed server blocks are in the same file; see [Plugin and hand-managed routes](#plugin-and-hand-managed-routes).

Duplicate generated domains use the compatibility gate: v1.4 keeps the existing build preflight refusal text for duplicates it already detects and warns once for additional duplicates found by the renderer. With `NSELF_V15=1`, every duplicate is refused as E055 with both route IDs. Give each service a distinct route before rebuilding.

## Plugin and hand-managed routes

`routes.json` also describes the nginx files nself does not render itself: every `nginx/*.conf` a plugin ships (rendered into `nginx/sites/<plugin>-<file>`, see [Plugin Route Injection](#plugin-route-injection)) and every hand-managed `nginx/conf.d/*.conf` and `nginx/conf.d-<env>/*.conf` (not `default.conf`). Each `server { }` block becomes one route:

| Route | `source` | `id` | `file` |
|---|---|---|---|
| Plugin snippet | `plugin` | `plugin:<plugin>/<file>#<k>` | `nginx/sites/<plugin>-<file>` |
| Hand-managed file | `hand_managed` | `hand:<file>#<k>` (`hand:<conf.d-env>/<file>#<k>` for an env directory) | `nginx/conf.d/<file>` |

Two plugins whose `<plugin>-<file>` names resolve to the same `nginx/sites` file (compared case-insensitively, for example plugin `a-b` with `c.conf` and plugin `a` with `b-c.conf`) are refused as E055 with `NSELF_V15=1`. In v1.4 mode it is a warning and the later file replaces the earlier one.

`<k>` is the 1-based index of the server block in the file. A usual snippet has two blocks for one name (port 80 redirect, port 443 proxy), so one name can appear in two routes. nginx behaviour does not change: plugin files are still copied byte for byte and hand-managed files are never touched.

Each directive is read with a closed vocabulary. It is mapped to a model field, judged to have no observable effect, or copied word for word into the route's `unmodelled` list. Nothing is evaluated: no `include`, `map` or `if`, and a variable is substituted only for `set $v <url>;` followed by `proxy_pass $v;` in the same location or server. `deny all` and `limit_except` map only in a closed scope: the location has no `allow` of its own (also inside `if` or `limit_except`), no `satisfy` other than `all` is in effect (from the location, the server or the top of the file), and neither the location nor its server holds an `if`, `rewrite`, `break`, `return`, `include` or `error_page`. An `error_page` at the top of the file counts too, because nginx inherits it into every location. Any of these can let a request past the access rule or answer it first.

| Directive | Result |
|---|---|
| `listen 80`, `listen 443 ssl [http2]` (also `[::]:` and `0.0.0.0:` forms) | `listen.http`, `listen.https` |
| `server_name` (plain FQDNs, lower-cased: nginx matches names without case) | `server_names`; a wildcard, regex, `_` or variable name keeps the directive in `unmodelled` |
| `ssl_certificate` and `ssl_certificate_key` both under `/etc/nginx/ssl/<dir>/` with names `fullchain.pem` and `privkey.pem`, where `<dir>` starts with a letter or digit | `tls.ssl_dir`; a different path or file name, mismatched directory names, a missing certificate or key, or a directory named `.` or `..` is `unmodelled` |
| `ssl_protocols`, `ssl_ciphers` | `tls.protocols`, `tls.ciphers` (empty and `null` mean the http-level default) |
| `return 30x https://$host$request_uri;` | `return` on a `/` location and `http_to_https_redirect: true`; another redirect target with a variable is mapped and also `unmodelled`; `return CODE;` maps to `return`; a body text stays `unmodelled` |
| `location /p` and `location = /p` | `locations` with `match` `prefix` or `exact`; regex, `^~`, named, nested and repeated locations stay `unmodelled` whole |
| `proxy_pass http://host:port` or an `upstream` name with one `server` | `upstream`; a URI part (`http://h:1/v1/`) is mapped AND kept in `unmodelled`, because v1 cannot carry the path |
| the four standard forwarded headers (`Host`, `X-Real-IP`, `X-Forwarded-For`, `X-Forwarded-Proto`, all present) | `forwarded_headers`; a partial set stays `unmodelled` |
| `Upgrade $http_upgrade` with `Connection "upgrade"` (or `"Upgrade"`) and `proxy_http_version 1.1` in scope | `websocket`; without `proxy_http_version 1.1`, the headers are recorded in `unmodelled` |
| other `proxy_set_header` with a literal value | `headers_set`; a value with a variable stays `unmodelled`. An empty value (`proxy_set_header Authorization "";`) is kept as `""` and means "do not pass this header to the upstream", as in nginx |
| `proxy_connect_timeout`, `proxy_read_timeout`, `proxy_send_timeout` | `timeouts` in whole seconds; a sub-second or zero value stays `unmodelled` |
| `client_max_body_size` with `k`, `m`, `g` | `max_body_bytes`; `0` (unlimited) stays `unmodelled` |
| `access_log off` | `access_log: false` |
| `deny all` in a closed scope (defined above the table) | `deny_all`; otherwise `unmodelled` |
| `limit_except M... { deny all; }` in a closed scope | `methods` as written (nginx also allows HEAD when GET is listed); otherwise `unmodelled` |
| `keepalive`, `proxy_http_version` (except when it enables websocket), `proxy_buffer_size`, `proxy_buffers`, `proxy_busy_buffers_size`, `http2`, `resolver`, a `set` used only for `proxy_pass` | no observable effect; not recorded |
| anything else | `unmodelled`, verbatim |

Timeouts, `client_max_body_size`, `access_log` and `proxy_set_header` written at server level apply to every location that does not set its own, as in nginx. A server block with no `listen` listens on port 80, as in nginx. A location with `return` plus `proxy_pass`, or `return` plus `deny all`, maps both and also keeps the `return` in `unmodelled` (nginx always answers the `return` first, before the proxy and before the access rules; v1 does not say which wins). The reader splits words only at space, tab, CR and LF, as nginx does: form feed and vertical tab are ordinary word bytes. Variable names are case-insensitive, as in nginx. A `set` variable is not substituted into `proxy_pass` when it is set twice, when the location or server holds an `if`, `rewrite`, `break`, `return`, `include` or `error_page`, when it is a server-level `set` and some location of that server sets the same variable, or when another directive can write it: `auth_request_set`, `set_by_lua` and the like, or any variable named outside the server blocks (`map`, `geo`, `js_set` ...). Every top-level `upstream` is kept verbatim in `unmodelled_global`, even when its name resolves inside the file, because nginx upstreams are visible to every file; a `proxy_pass` to an upstream name defined in another file is `unmodelled`, never resolved as a DNS host.

`unmodelled` is the fail-closed list for other providers: a route whose `unmodelled` is not empty has behaviour the model cannot say, so a provider that is not nginx must refuse it, never serve a partial route. The same holds for the top-level `unmodelled_global` list, which holds the directives outside any server block: `map`, `limit_req_zone`, an `upstream` with several servers, and bare `location` blocks in hand-managed files. A hand-managed file that does not parse is recorded there too, and so is each plugin file the build left out of `nginx/sites` (`skipped plugin <plugin>/<file>: <reason>`, see [Plugin Route Injection](#plugin-route-injection)). Snippets are third-party input; the reader is fuzzed and a test checks that every directive is mapped, ignored or recorded.

---

## What You CAN Safely Edit

Files in `nginx/conf.d/` are hand-managed and safe to customize. The build system checks for conflicts and skips auto-generating a `sites/` config if the same domain already exists in `conf.d/`. This lets you override a service route with a fully custom configuration without the build clobbering your changes.

For example, if you want to serve `api.yourdomain.com` with a custom caching policy or non-standard proxy settings, create a file in `nginx/conf.d/` targeting that server name. On the next `nself build`, the generator will detect the conflict, skip writing `nginx/sites/hasura.conf` for that domain, and print a notice so you know the override is in effect.

---

## Route Structure

Each enabled service gets its own subdomain. The base domain is controlled by the `BASE_DOMAIN` environment variable.

| Service | Subdomain | Notes |
|---|---|---|
| Hasura GraphQL | `api.{BASE_DOMAIN}` | WebSocket support, 86400s read timeout |
| Auth | `auth.{BASE_DOMAIN}` | Strict rate limiting (10 req/min) |
| MinIO API | `storage.{BASE_DOMAIN}` | 1000M max body size |
| MinIO Console | `storage-console.{BASE_DOMAIN}` | |
| Admin dashboard | `admin.{BASE_DOMAIN}` | Dev mode: proxies to host machine |
| Search | `search.{BASE_DOMAIN}` | |
| Email UI | `mail.{BASE_DOMAIN}` | |
| Grafana | `grafana.{BASE_DOMAIN}` | |
| Prometheus | `prometheus.{BASE_DOMAIN}` | |
| Alertmanager | `alertmanager.{BASE_DOMAIN}` | |

Custom services get routes based on `CS_N_ROUTE`. Frontend apps get routes based on `FRONTEND_APP_N_ROUTE`. Both support full subdomain paths or path-based routing depending on your configuration.

---

## SSL Termination

SSL mode is controlled by the `SSL_MODE` environment variable. All TLS termination happens at the nginx layer, upstream services always receive plain HTTP.

| Mode | Description |
|---|---|
| `local` (default) | Self-signed certificates generated by mkcert (preferred) or OpenSSL fallback |
| `custom` | Provide your own cert and key via `SSL_CERT_PATH` and `SSL_KEY_PATH` |
| `letsencrypt` | Automated Let's Encrypt certificates (production) |
| `none` | HTTP only , not recommended, disables all redirect logic |

For local development, mkcert is strongly preferred because browsers trust the resulting certificate without warnings or bypass prompts. When mkcert is available, `nself build` automatically collects all service subdomains as Subject Alternative Names (SANs) and issues a single cert covering the entire stack. If mkcert is not installed, the build falls back to OpenSSL for a self-signed cert, functional but not browser-trusted.

For production, `letsencrypt` mode handles certificate issuance and renewal automatically using the ACME HTTP-01 challenge. All subdomains must be publicly reachable before running `nself start` in this mode.

---

## Security Headers

The following headers are included by default on all generated routes:

```nginx
add_header X-Content-Type-Options nosniff;
add_header X-Frame-Options DENY;
add_header X-XSS-Protection "1; mode=block";
```

A `Content-Security-Policy` header is intentionally not set at the global nginx level. Plugins and custom services often need to customize CSP per route (for example, a chat plugin embedding media from external sources). Set CSP in the relevant `conf.d/` override or within the plugin's injected route configuration.

---

## Rate Limiting

Rate limiting zones are defined in `nginx/includes/rate-limits.conf` and referenced by each generated service config. The defaults are:

| Service | Rate | Burst |
|---|---|---|
| GraphQL API | 100 req/min | 20 |
| Auth | 10 req/min | 5 |
| Storage uploads | 5 req/min | 2 |
| Functions | 50 req/min | 15 |
| Plugin webhooks | 30 req/min | 10 |
| Admin dashboard | 10 req/sec | 10 |
| Custom services | 10 req/sec | 10 |

To adjust limits for a specific service, override its route in `nginx/conf.d/` with your own `limit_req_zone` and `limit_req` directives.

There is no `NGINX_RATE_LIMIT` variable — it does not exist anywhere in
this codebase, is not declared in any env template, and no generator reads
it. Each zone above is configured via its own dedicated var instead:
`RATE_LIMIT_API_RPS`, `RATE_LIMIT_AUTH_RPS`, `RATE_LIMIT_AI_RPS`
(`internal/nginx/ratelimit.go`) and `AUTH_RATE_LIMIT` for the whole-server
Auth zone.

### Path-Scoped Rate Limits (SEC-HARDENING-06)

On top of the one server-wide zone above, every generated service conf
(`nginx/sites/*.conf`) and every proxying `nself ssl add <domain>
--upstream ...` custom-domain conf (`nginx/conf.d/custom-*.conf`) also
carries two path-scoped `location` blocks, applied unconditionally
regardless of which zone the server-wide one uses:

| Path | Match | Zone | Rate | Burst |
|---|---|---|---|---|
| `/auth/login` | exact (`location =`) | `auth_strict` | `RATE_LIMIT_AUTH_RPS` (default 5r/s) | 5 |
| `/api/` | prefix | `api` | `RATE_LIMIT_API_RPS` (default 30r/s) | 20 |

nginx resolves the longest-matching prefix `location` regardless of
declaration order in the file, so a request to either path always hits the
stricter zone even on a server otherwise routed through a looser one (for
example a `CS_N` custom service on the `Custom services` zone above) — a
request that never touches those paths is unaffected. Applied
unconditionally because the generator has no reliable way to know whether
a given `CS_N` or internal-route service's upstream actually serves either
path; the blocks are harmless when it doesn't. The custom-domain
placeholder conf (`nself ssl add <domain>` with no `--upstream`) never
proxies anywhere, so it carries neither block.

Source: `internal/nginx/generator.go`'s `defaultSecurityPathZones()`
(default set) and `ServiceRouteData.PathZones` (template field), rendered
by `internal/nginx/templates/service.conf.tmpl`; custom-domain equivalent
in `cmd/commands/ssl_install.go`'s `writeCustomDomainConf()`. Verified by
`nself doctor --deep`'s `SEC-HARDENING-06` check
(`internal/doctor/hardening_check_nginx_zones.go`), which accepts either of
two signals per generated conf file — it does not require both:

1. **Service identity** (cli#379): the file is split into its `server {}`
   blocks, and a block whose `server_name` first label is a known
   auth/API hostname (`auth` for the auth service; `api`, `hasura`,
   `graphql`, `ping`, or `ping-api` for the API/Hasura/ping-api surface —
   see `nginx.Route` defaults in `internal/nginx/routes_core.go` and the
   shipped `CS_1_ROUTE=ping` example in
   `internal/setup/setup_env_files.go`) contains a `limit_req` directive
   anywhere in its body, including the server-wide `location /` zone from
   the table above — not only the path-scoped blocks.
2. **Literal path fallback** (original behavior): the file contains
   `limit_req` co-occurring with the literal string `/auth/login` or
   `/api/`, for hand-written gateway configs that route by path instead
   of by `server_name`.

A conf with no `limit_req` anywhere fails both signals, and a `limit_req`
confined to an unrelated service's block (e.g. only a frontend app's
`location /`) satisfies neither.

---

## Plugin Route Injection

An installed plugin can ship nginx server blocks as `nginx/*.conf` in its plugin directory (`NSELF_PLUGIN_DIR`, default `~/.nself/plugins`). During `nself build` each file is rendered and written to `nginx/sites/<plugin>-<file>` (for a fronted project, the fronting stack's `nginx/sites`), which `nginx.conf` includes at the `http` level.

Rendering replaces these variables and nothing else:

| Variable | Value |
|---|---|
| `${BASE_DOMAIN}` | `BASE_DOMAIN` |
| `${SSL_DIR}` | the base domain's certificate dir under the nginx ssl mount, `certificates/<domain-dir>` (`local.nself.org` gives `certificates/local-nself-org`), so `/etc/nginx/ssl/${SSL_DIR}/fullchain.pem` is the certificate ɳSelf serves |
| `${PROJECT_NAME}`, `${COMPOSE_PROJECT_NAME}` | `PROJECT_NAME` |
| `${DOCKER_NETWORK}` | the stack's Docker network |
| `${POSTGRES_USER}`, `${POSTGRES_DB}`, `${POSTGRES_PASSWORD}`, `${HASURA_PORT}`, `${AUTH_PORT}`, `${NSELF_PLUGIN_DIR}` | the matching project values |

A file nginx would refuse to load would stop nginx, and with it every route, so the build leaves it out and prints a warning naming the plugin, the file and the reason:

- it still holds a `${NAME}` variable from outside this table (nginx's own lower-case variables such as `${host}` are fine);
- it does not parse, or it has a directive at the top level that nginx accepts only inside a `server` block (a bare `location` block, for example);
- it loads the `${SSL_DIR}` certificate while the build serves no TLS (`SSL_MODE=none`, or `letsencrypt`/`custom` before the certificate exists).

A skipped file is also listed in `routes.json` under `unmodelled_global`. Run `nself build` again after updating the plugin or issuing the certificate.

A plugin file whose `server_name` is already claimed on the same port by another file in `nginx/sites` fails the build, naming both files, because nginx would silently serve only one of them.

---

## Lazy Resolver Pattern

Optional services, those that may not be running at nginx startup, use a DNS-based lazy resolution pattern to prevent nginx from failing to start with an upstream lookup error:

```nginx
resolver 127.0.0.11 valid=10s;
set $upstream http://admin:3021;
proxy_pass $upstream;
```

Setting the upstream via a variable defers DNS resolution to request time rather than startup time. Docker's embedded DNS (`127.0.0.11`) resolves container names dynamically, so if an optional container comes up after nginx is already running, traffic routes to it without requiring a reload.

Core required services, Hasura and Auth, use direct `proxy_pass` without the variable pattern because they are always running when nginx starts.

---

**See also:** [[Architecture]] | [[Compose-Generation]] | [[Service-Graph]] | [[Home]]
