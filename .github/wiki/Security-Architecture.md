# Security Architecture

ɳSelf is designed with security as a core concern, not an afterthought. This document describes the security model and key protections built into the CLI and generated infrastructure.

## Container Security

All generated containers include hardened defaults:

```yaml
security_opt:
  - no-new-privileges:true
cap_drop:
  - ALL
```

- **`no-new-privileges`**, prevents privilege escalation via setuid binaries
- **`cap_drop: ALL`**, removes all Linux capabilities from containers by default

Nginx is the only service that retains `NET_BIND_SERVICE` (required for ports 80/443).

## Network Isolation

All internal services bind to `127.0.0.1` only. The Docker bridge network (`{project}_default`) is internal, services communicate via Docker DNS names (e.g., `hasura:8080`).

Only Nginx exposes external ports (80 and 443). All external traffic flows through Nginx, which handles TLS termination and proxying to internal services.

| Service | Binding | External Access |
|---------|---------|----------------|
| PostgreSQL | `127.0.0.1:5432` | Nginx only (not exposed) |
| Hasura | `127.0.0.1:8080` | Via Nginx proxy |
| Auth | `127.0.0.1:4000` | Via Nginx proxy |
| Nginx | `0.0.0.0:80,443` | Direct |

## Secret Management

Secrets (passwords, API keys, JWT secrets) are managed through the `.env` cascade:

- `.env.secrets`, sensitive values, **never committed to git**
- Secrets are passed to containers as environment variables, not files
- `.env.secrets` must be in `.gitignore` (verified by `nself doctor`)
- Secret values are **redacted** from all CLI output (`***REDACTED***`)

Secret env vars matching these patterns are automatically redacted:
`*_PASSWORD`, `*_SECRET`, `*_KEY`, `*_TOKEN`

## Security Headers

ɳSelf's Nginx configuration includes security headers on all server blocks:

```nginx
add_header X-Frame-Options "SAMEORIGIN" always;
add_header X-Content-Type-Options "nosniff" always;
add_header X-XSS-Protection "1; mode=block" always;
add_header Referrer-Policy "strict-origin-when-cross-origin" always;
add_header Content-Security-Policy "default-src 'self'" always;
server_tokens off;
```

## Auth Rate Limiting

Auth endpoints are rate-limited to prevent brute-force attacks:

- Default: **30 requests/minute** per IP
- Burst: 5 requests
- Configurable via `AUTH_RATE_LIMIT` env var

## Input Sanitisation

All user inputs are validated via `internal/sanitize` before being used in generated configs:

| Input | Constraint |
|-------|-----------|
| Project name | Lowercase, 2–30 chars, alphanumeric + hyphens |
| Base domain | Valid FQDN |
| Port numbers | 1024–65535, not in reserved list |
| Custom service names | Lowercase, alphanumeric + hyphens/underscores |
| Plugin versions | Anchored semver regex |
| Backup file paths | No path traversal (`../`) |

## Plugin Integrity

Plugin manifests are decoded with `DisallowUnknownFields` to prevent field injection attacks. All fields are validated after decode:
- Version strings must match anchored semver
- Plugin download URLs must use HTTPS from allowed domains
- Checksums are verified (SHA-256) before installation
- Ed25519 signatures are verified against the publisher key pinned in the registry when present

### Plugin Signature Verification (S47-T09)

When the registry entry for a plugin includes `author_public_key` and `signature` fields, the CLI performs Ed25519 signature verification after the SHA-256 checksum passes:

1. Decode the hex-encoded 32-byte public key from the registry.
2. Compute SHA-256 of the downloaded tarball.
3. Verify the Ed25519 signature over that digest using `crypto/ed25519`.
4. If verification fails, install is aborted with: `plugin signature verification failed: tarball does not match registry signature (possible tampering)`.

This protects against supply-chain attacks where an attacker replaces a tarball on the distribution server but cannot forge a valid signature without the plugin author's private key.

The verification step is skippable only in offline/development mode via `NSELF_LICENSE_SKIP_VERIFY=1` (requires `--force` flag). It is never skipped in production installs. Per the Security-Always-Free Doctrine, this verification is free, automatic, and applies to all plugin tiers.

## File Permissions

ɳSelf applies least-privilege file permissions:

| File/Dir | Permission |
|----------|-----------|
| `.env`, `.env.*` | `0600` (owner read/write only) |
| Backup directories | `0700` |
| SSL certificate directory | `0750` |
| Plugin cache files | `0600` |

## TLS

The ɳSelf CLI itself enforces TLS 1.2 minimum for all outbound HTTPS connections (registry, license validation, update checks). Weaker protocol versions are rejected.

## Authentication Model

ɳSelf uses a layered authentication system:

### JWT Authentication
The Auth service (nHost) issues JWTs on login. Tokens carry user ID, roles, and tenant claims. Hasura validates JWTs on every GraphQL request using a shared JWT secret (`HASURA_JWT_KEY`, minimum 32 characters).

Token lifecycle:
- Access tokens: short-lived (configurable, default 15 minutes)
- Refresh tokens: long-lived, stored server-side
- Token rotation: refresh tokens are single-use; each refresh issues a new refresh token

### Passkey Authentication (WebAuthn)
ɳClaw supports passkey-based authentication via `ASWebAuthenticationSession` on Apple platforms and WebAuthn on web. The pairing flow uses:
- Pair codes with TTL (410 Gone on expiry)
- Direct pairing via HMAC-SHA256 4-word phrases
- Pair attempt rate limiting: 10 failures per 15 minutes triggers 429 + Telegram alert
- `np_claw.pair_attempts` audit table tracks all attempts

### OAuth Providers
Auth supports 13+ OAuth providers (GitHub, Google, Apple, Microsoft, etc.). OAuth tokens for AI providers (Claude, GPT-4o) are stored with AES-256-GCM encryption in the database. Token refresh happens proactively (30-minute cycle).

## Rate Limiting

### Auth Rate Limiting
Auth endpoints: 30 requests/minute per IP (burst: 5). Configurable via `AUTH_RATE_LIMIT`.

### Plugin Rate Limiting
- ɳClaw pairing: 10 failed attempts per 15 minutes per IP, then 429 + Telegram alert
- AI caller tokens: per-namespace rate limiting with configurable limits per token
- Mux stall detector: exponential backoff on repeated failures (3 same-error cycles triggers stop + Telegram alert)

### Priority Queue
The AI plugin uses a 4-level priority queue for LLM requests. Higher-priority requests (user-facing chat) preempt lower-priority ones (background classification).

## ACL System

Access control operates at multiple levels:

### Hasura Permissions
Row-level security (RLS) enforced at the GraphQL layer. Each table defines insert/select/update/delete permissions per role. Tenant isolation uses RLS policies on `tenant_id` columns.

### Plugin Schema Isolation
Each plugin gets its own PostgreSQL schema (`np_{plugin_name}`) with a dedicated role. Row-level security applies per schema. Zero cross-plugin data leakage by design.

### Tool Gating
ɳClaw persona `tools_allowed` whitelists restrict which tools each persona can invoke. Shell access requires explicit `NCLAW_ALLOW_SHELL=true`. Browser automation requires user consent (`NClaw_BrowserEnabled` user default).

### Telegram User Gating
`CLAW_TG_ALLOWED_USERS` restricts which Telegram user IDs can interact with the claw bot. Messages from unauthorized users are silently dropped.

## Plugin-to-Plugin Authentication

The trust boundary between plugins is the deployment. Plugins in one project authenticate to each other with shared secrets over the Docker bridge network:

| Secret | Protects |
|--------|----------|
| `PLUGIN_INTERNAL_SECRET` | Generic inter-plugin X-Internal-Token header |
| `MUX_CLAW_SHARED_SECRET` | Mux-to-claw RPC calls (POST /internal/classify) |
| `NOTIFY_INTERNAL_SECRET` | Notify plugin internal API |
| `CLAW_WEB_SECRET` | Claw-web dashboard authentication |

`PLUGIN_INTERNAL_SECRET` is generated per project by the CLI at `nself init` and in the config defaults, and stored in the project's env files. `nself build` injects it into every plugin container server-side. The secret is never sent to any nSelf service and never placed in a browser bundle. It travels only with the project's own env files to that project's deploy target.

`ping.nself.org` holds no plugin identity. It registers no plugin keys and issues no plugin-to-plugin tokens. The per-plugin identity client that earlier 1.4.x CLIs shipped (it posted the secret to ping on the first install of each plugin) is removed, and a repository test keeps it from returning.

Limits: one secret per deployment authenticates any component of that deployment, so it does not separate one plugin or app from another. App isolation stays in the data layer (`source_account_id`). Per-plugin caller identity is deferred and will be local to the deployment, with no network call.

All internal endpoints validate the token before processing. Missing or invalid tokens return 401 (not 500).

## Browser Automation Security

The browser plugin (Playwright/CDP) includes multiple safety layers:

- **SSRF blocking.** All RFC-1918 addresses, private IPs, and localhost are blocked by default.
- **URL allowlist.** `np_claw.browser_allowlist` table controls which domains are accessible.
- **Audit log.** Every browser action is logged to `np_claw.browser_audit_log`.
- **Consent flow.** Browser automation requires explicit user opt-in (`NClaw_BrowserEnabled` user default on macOS).
- **Stealth mode.** `playwright-extra` prevents bot detection on allowed sites.

## Shell Execution Security

The `shell_dispatch_vps` tool (when enabled via `NCLAW_ALLOW_SHELL`) logs every command to `np_claw.shell_audit_log`. Dangerous commands require confirmation via `np_claw.pending_confirmations` with a 2-minute expiry window.

## Email Security

- `CLAW_BLOCKED_RECIPIENTS`: hard blocklist checked before any email pipeline processing.
- Auto-reply guard rails: blocked senders, low-confidence replies, and sensitive topics are escalated to Telegram for manual approval.
- `np_mux_auto_reply_audit` tracks every auto-reply decision (sent, blocked_sender, blocked_confidence, blocked_topic).

## Deferred to v1.1

The following security improvements are planned for a future release:

- Non-root container users for Postgres, Auth, Functions
- Docker socket proxy for Admin service
- Binary signature verification (GPG)
- Automatic ACME certificate renewal

## See Also

- [[Security-Policy]] -- how to report vulnerabilities
- [[Security-Hardening]] -- production checklist
- [[Guide-Security-Hardening]] -- step-by-step hardening guide
- [[API-Reference]] -- plugin endpoint authentication
- [[Configuration]] -- environment variable reference for secrets

---
← [[Home]] | [[_Sidebar]]
