# JWT Key Rotation

JWT keys for Hasura are long-lived credentials. Rotating them periodically
limits the blast radius if a key is ever leaked and satisfies common compliance
requirements (SOC 2, PCI-DSS, ISO 27001).

---

## When to rotate

- Immediately after a suspected credential leak.
- On a regular schedule, at minimum every 90 days.
- Before or after a major infrastructure change (new host, provider migration).

The `nself doctor --deep` check **JWT-ROT-01** warns when the last recorded
rotation is older than the configured window (default: 90 days) and fails when
it exceeds the hard limit (default: 2x the window, i.e. 180 days). Fix it with:

```bash
nself self-heal --jwt
```

---

## How rotation works

1. `nself self-heal --jwt` generates a new 256-bit random key and writes a
   timestamped entry to the rotation log (`/var/lib/nself/jwt-rotation.log`
   by default, with XDG fallback for non-root environments).
2. The new key is printed to stdout by default. Use `--to-file <path>` to write
   it to a file at mode 0600 instead. Use `--no-print` to suppress stdout output
   entirely (useful in automated pipelines combined with `--to-file`).
   With `--to-file`, the key is written and synced in a private temporary file
   before the rotation log is updated, then renamed over the target. A write
   or sync failure leaves the log unchanged. If the final rename fails, the
   command reports that rotation was logged but delivery failed; retry it.
3. The old key is NOT overwritten automatically. You must apply the new key
   yourself (see steps below).
4. A 12-hour grace period begins. During this window you should keep both
   keys configured so running tokens signed by the old key remain valid.
5. After the grace period, remove the old key reference.

---

## Cross-service restart cascade

After applying a new JWT key, every service that holds a cached copy of the old
key must restart. This includes:

- **Hasura**: reads `HASURA_GRAPHQL_JWT_SECRET` at startup. Must restart to pick
  up the new key.
- **auth-server**: validates tokens against the same key. Must restart.
- **Every plugin with JWT verification** (ai, claw, mux, browser, and any
  plugin that calls `auth.VerifyToken`): must restart to clear the in-process
  key cache.

**WebSocket clients with active subscriptions will be disconnected** when
Hasura restarts. Inform users before rotating in production or rotate during a
low-traffic window.

### Recommended restart sequence

```bash
# 1. Apply new key to .env.secrets (see Step-by-step below)

# 2. Rebuild the generated config
nself build

# 3. Restart in dependency order: auth first, then Hasura, then plugins
nself restart auth
nself restart hasura
nself restart plugins

# 4. Verify all services are healthy
nself doctor --deep --only security
```

To restart everything at once instead of one service at a time:

```bash
nself restart
```

---

## Hasura version compatibility

Dual-key JWKS support (serving both old and new keys simultaneously) requires
Hasura v2.10+. Earlier versions support only a single `HASURA_GRAPHQL_JWT_SECRET`.

If your Hasura version is below v2.10, you cannot use the dual-key grace period.
Instead, you must schedule a maintenance window and do a hard cutover:

1. Rotate the key with `nself self-heal --jwt`.
2. Update `.env.secrets` immediately.
3. Run `nself build && nself restart` in one step.
4. Accept that active WebSocket sessions will be disconnected.

---

## Step-by-step

```bash
# 1. Run the rotation routine
nself self-heal --jwt

# 2. Copy the printed new key, then open .env.secrets:
nano .env.secrets

# 3. Update the JWT secret line. Use the JSON format Hasura requires:
HASURA_GRAPHQL_JWT_SECRET={"type":"HS256","key":"<NEW_KEY>"}

# 4. During the grace period, add the old key as a secondary entry so
#    Hasura accepts tokens signed by the old key until they expire.
#    See https://hasura.io/docs/latest/auth/authentication/jwt/ for JWKS config.

# 5. Restart services in order (see Cross-service restart cascade above)
nself build && nself restart

# 6. After grace period (~12h): remove the old key reference from .env.secrets.
```

---

## Incident response runbook

Use this sequence when a JWT key is suspected to have leaked.

```bash
# Step 1. Rotate immediately (no grace period window applies for a leak).
nself self-heal --jwt

# Step 2. Apply the new key to .env.secrets right away.
#         Do NOT wait for the grace period.

# Step 3. Restart all services immediately (expect active session disconnects).
nself build && nself restart

# Step 4. Audit recent auth logs for signs of abuse (unusual token issuers,
#         unexpected service-to-service calls).
#         Hasura logs: docker logs nself-hasura --since 24h
#         auth-server: docker logs nself-auth --since 24h

# Step 5. Rotate once more after 24h to ensure any tokens issued with the
#         leaked key have fully expired.

# Step 6. File an incident report and review how the key was leaked.
#         Common causes: .env.secrets committed to git, key logged in plain text,
#         key included in an HTTP response body.
```

---

## Configuration

| Environment variable | Default | Purpose |
|---|---|---|
| `NSELF_JWT_ROTATION_LOG` | `/var/lib/nself/jwt-rotation.log` (XDG fallback when not writable) | Where rotation events are recorded |
| `NSELF_JWT_ROTATION_WINDOW_DAYS` | `90` | Maximum days between rotations before JWT-ROT-01 warns |
| `NSELF_JWT_ROTATION_HARD_DAYS` | `2 * NSELF_JWT_ROTATION_WINDOW_DAYS` | Age at which JWT-ROT-01 escalates from warn to fail |

### Log path fallback

When `/var/lib/nself/` is not writable (common on developer workstations),
the rotation log falls back to:

```
${XDG_STATE_HOME:-$HOME/.local/state}/nself/jwt-rotation.log
```

Override with `NSELF_JWT_ROTATION_LOG` to specify an explicit path.

---

## Rotation log format

Each line in the rotation log is an RFC3339 timestamp followed by the event:

```
2026-04-30T14:22:00Z rotated HASURA_GRAPHQL_JWT_SECRET (grace period ends 2026-05-01T02:22:00Z)
```

Comments (lines starting with `#`) are ignored. Future timestamps are rejected
(clock-skew guard). The log file is append-only and protected by an exclusive
advisory lock (`flock(2)`) so concurrent rotation calls cannot produce torn writes.

### Log integrity

Each line in the rotation log is written atomically under an exclusive flock.
To detect tampering (lines deleted or modified after the fact), compute a
running HMAC chain: each entry's hash incorporates the hash of the previous
entry. Tooling to verify the chain is planned for a future release. For now,
store the log on a filesystem with integrity protection (e.g. dm-verity, ZFS
with checksums) or ship it to an append-only audit log service.

---

## Verifying with doctor

```bash
# Standard check included in SecurityChecks
nself doctor --deep --only security

# Check JWT-ROT-01 output specifically
nself doctor --deep --only security --json | jq '.checks[] | select(.name | contains("JWT-ROT-01"))'
```

Expected pass output:

```
JWT-ROT-01: last rotation 14 days ago (window=90d)
```

Expected warn output (rotation overdue):

```
JWT-ROT-01: last rotation was 95 days ago (window=90d) — rotate with 'nself self-heal --jwt'
```

Expected fail output (hard limit exceeded):

```
JWT-ROT-01: last rotation was 185 days ago — exceeds hard limit of 180d (NSELF_JWT_ROTATION_HARD_DAYS). Rotate immediately with 'nself self-heal --jwt'
```

---

## Dry run

To see what would happen without making any changes:

```bash
nself self-heal --jwt --dry-run
```

## Writing key to file

To write the new key to a file instead of stdout (useful in scripts):

```bash
nself self-heal --jwt --to-file /run/secrets/jwt-new-key
# File is created with mode 0600.

# Suppress stdout entirely (combined with --to-file in automation):
nself self-heal --jwt --to-file /run/secrets/jwt-new-key --no-print
```

---

## See also

- [[operations/self-healing]] — overview of all self-healing routines
- [[Config-Auth]] — JWT algorithm and key configuration for Hasura
- [[Home]]
