# Guide: SSL / TLS Setup

ɳSelf manages TLS certificates through nginx. Three certificate modes are supported.

## Self-Signed Certificates (Default)

ɳSelf auto-generates a self-signed certificate when you first run `nself build`. This is appropriate for local development and internal networks.

```bash
nself ssl status    # check certificate details and expiry
```

Browsers will show a security warning for self-signed certs, expected behaviour for local dev.

## Base Domain Certificates (DNS-01)

Use `nself ssl setup` to provision a wildcard or multi-domain Let's Encrypt certificate for your configured `BASE_DOMAIN` via DNS-01 challenge. This requires your DNS provider credentials to be in place before running.

```bash
# Wildcard certificate via Cloudflare
nself ssl setup --provider cloudflare --wildcard --email admin@example.com

# Per-subdomain certificate via Route53
nself ssl setup --provider route53 --email admin@example.com

# Enable automatic renewal via systemd timer (Linux)
nself ssl setup --provider cloudflare --wildcard --install-cron
```

Supported providers: `cloudflare`, `route53`, `digitalocean`. For other CAs, place credentials in `/etc/letsencrypt/{provider}.ini` and use `--provider custom`.

Certificates land in `ssl/{domain}/` inside the project directory. nginx reads them at `/etc/nginx/ssl/{domain}/` via the `./ssl:/etc/nginx/ssl:ro` mount.

## CLI-Owned ACME (DNS-01, no host certbot)

`nself ssl setup --acme` and `nself ssl renew --acme` issue and renew Let's Encrypt certificates from inside the stack (ADR 0026). A pinned `lego` container runs once per issuance; nothing is installed on the host and the Docker socket is never mounted. Certbot keeps working for projects that do not pass `--acme`.

### Prerequisites

- The served nginx container mounts the **whole** `ssl/` directory at `/etc/nginx/ssl`. Mounts of single certificates or `certificates/<dir>` are refused, because the install below uses symlinks that must resolve inside the container.
- `age` on `PATH` and an age key (`SECRETS_AGE_KEY_PATH`, default `~/.config/nself/age-key.txt`). DNS credentials live in the project secret store (`.secrets/<env>.age`), never in a file or on a command line.
- An ACME contact: `--email`, else `ACME_EMAIL`, else `ADMIN_EMAIL`.
- A DNS credential in the store under its name: `SSL_DNS_CLOUDFLARE_API_TOKEN` (Cloudflare, Zone:DNS:Edit token), `SSL_DNS_DIGITALOCEAN_TOKEN`, or `SSL_DNS_AWS_ACCESS_KEY_ID` + `SSL_DNS_AWS_SECRET_ACCESS_KEY` (Route53).

Always start with a dry run. It resolves and prints the served root, ssl dir, nginx dir, nginx container and its mount, and writes nothing:

```bash
nself ssl setup --acme --dry-run
nself ssl setup --acme --agree-tos                 # issue BASE_DOMAIN, api., auth. (or --wildcard)
nself ssl setup --acme --install-cron              # daily renewal timer (Linux)
nself ssl renew --acme --dry-run                   # which lineages are due
nself ssl renew --acme --staging --agree-tos       # rehearse against the staging CA; installs nothing
```

### Adopting certbot lineages

Production boxes already hold certbot lineages. Adoption reads `/etc/letsencrypt/renewal/*.conf` and `live/<name>/cert.pem`, maps each lineage to the served directory its names are served from, and records it as `dns-01`. It never issues, and it never edits or removes certbot state (renewal files, hooks, timers, credential files).

```bash
nself ssl setup --acme --adopt-certbot --dns-credential-file ./cloudflare.ini --dry-run
nself ssl setup --acme --adopt-certbot --dns-credential-file ./cloudflare.ini
```

- A `dns-cloudflare` / `dns-route53` / `dns-digitalocean` lineage keeps its provider.
- A `standalone`, `webroot` or `nginx` lineage is converted to dns-01 only when you pass a credential file; without one it is refused with that remediation.
- The credential file is a certbot DNS-plugin INI with exactly one provider: `dns_cloudflare_api_token`, `dns_digitalocean_token`, or `aws_access_key_id` + `aws_secret_access_key`. A Cloudflare global key is refused: create a scoped token.
- Served certificates are left byte for byte as they are unless a target is missing or older than certbot's live certificate.
- Adoption is saved lineage by lineage; if it fails part way, fix the cause and run it again to finish. Names that certbot also manages are flagged by `nself ssl status`.
- After adopting, remove the certbot timer yourself once `nself ssl renew --acme --dry-run` shows the lineages; the CLI never touches it.

### How a certificate is installed

Each target `ssl/certificates/<dir>` (always `certificates/<one plain name>`; anything else is refused) becomes a relative symlink to `.<dir>.gen-<n>/`, which holds `fullchain.pem`, `privkey.pem`, `cert.pem` and `chain.pem` (0600; the same files as certbot's `live/` directory, so confs that name `chain.pem` keep loading). A new generation is written and its key and certificate are checked against each other; then one rename of a temporary symlink over `<dir>` switches both files at once. nginx is then tested (`nginx -t`) and reloaded through `docker exec`, and the served certificate's fingerprint is checked. If any step fails the previous generation is restored and the command exits with `E151`. The previous generation stays until the next successful install. Every `--acme` run first repairs a missing link or a mismatched pair from the newest valid generation, so a crash between steps cannot leave a half-installed pair.

Only one mutating `--acme` run proceeds at a time: it holds `ssl/.acme/.lock`, and a second run waits up to 10 seconds, then exits with `E151` naming the holder's pid.

State lives in `ssl/.acme/` (0700, with a `.gitignore` of `*`): the ACME account, `certificates/`, `staging/` and `lineages.json` (names, challenge, provider, credential secret names, targets; never a value).

### Renewal timer

`--install-cron` writes `nself-acme-renew.service` and `.timer` (daily 03:30, up to 1 hour random delay, `Persistent=true`). The service sets `SECRETS_AGE_KEY_PATH` and `HOME` explicitly, because systemd's bare environment has neither, and runs `nself trust ssl renew --acme --quiet` from the project directory. The unit runs as root: run `setup --acme --install-cron` as the user who owns the age key (the install message prints the key path the unit will use), or `HOME` and the key path will point at the wrong user. Lineages with 30 days or fewer left are renewed.

### What the CLI never touches

Host certbot state (`/etc/letsencrypt`, certbot timers, hooks, credential files), nginx configuration, compose files and container definitions. It does not recreate or restart containers; only `nginx -s reload`.

## Custom Domain Certificates (HTTP-01)

Use `nself ssl add` to provision a certificate for an external custom domain (e.g., a white-labelled subdomain or a partner's domain). This uses HTTP-01 challenge, no DNS provider configuration needed.

```bash
# Add certificate and proxy traffic to a backend container
nself ssl add portal.example.com --upstream portal-app:8080

# Add certificate without configuring an upstream yet
nself ssl add portal.example.com
```

After certbot completes, the command:

1. Writes `nginx/conf.d/custom-{domain}.conf` with a full HTTPS server block (HTTP/2, security headers, HTTP-to-HTTPS redirect).
2. Tests the nginx config (`nginx -t`).
3. Reloads nginx.

The generated conf uses `proxy_pass http://{upstream}` when `--upstream` is provided, or returns a `200` response until an upstream is configured.

To update the upstream later, edit `nginx/conf.d/custom-{domain}.conf` and reload:

```bash
docker compose exec nginx nginx -s reload
```

## Custom Certificate Installation

If you have a certificate from a commercial CA (DigiCert, Sectigo, etc.):

1. Place your files in the project's `ssl/{domain}/` directory:
   ```
   ssl/{domain}/fullchain.pem
   ssl/{domain}/privkey.pem
   ```

2. Write an nginx server block to `nginx/conf.d/custom-{domain}.conf` following the same pattern that `nself ssl add` generates.

3. Test and reload nginx:
   ```bash
   docker compose exec nginx nginx -t
   docker compose exec nginx nginx -s reload
   ```

## SSL Commands

| Command | Description |
|---------|-------------|
| `nself ssl status` | Show certificate path, issuer, and expiry date; with `--acme` lineages, also a Lineages table and a certbot-overlap warning |
| `nself ssl renew` | Trigger manual certificate renewal prompt |
| `nself ssl setup` | Provision a wildcard/multi-domain cert via DNS-01 |
| `nself ssl add <domain>` | Provision a cert for a single custom domain via HTTP-01 |
| `nself ssl setup --acme` | Issue or adopt certbot lineages with the CLI's own ACME client (DNS-01) |
| `nself ssl renew --acme` | Renew the lineages the CLI manages (30 days or fewer left, or `--force`) |

## See Also

- [[cmd-ssl]], ssl command reference
- [[Guide-Production-Deployment]], full server setup
- [[Guide-Security-Hardening]], ensure TLS is properly configured

---
← [[Home]] | [[_Sidebar]]
