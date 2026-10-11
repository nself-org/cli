# Secret Rotation Runbook

Extends the S64 secrets-rotation baseline with automated schedule management and event logging.

## Manual Rotation

```bash
# Rotate a single secret immediately
nself secrets rotate HASURA_GRAPHQL_ADMIN_SECRET

# Rotate with dual-key overlap window (keep old key as _PREVIOUS)
nself secrets rotate JWT_SIGNING_KEY --dual-window

# Retire the old key after the overlap window (e.g. after 7 days)
nself secrets retire JWT_SIGNING_KEY
```

## Scheduling Automated Rotation

```bash
# Set a 90-day rotation schedule for the JWT signing key
nself secrets schedule --secret JWT_SIGNING_KEY --every 90d

# Set a 180-day schedule for the Hasura admin secret
nself secrets schedule --secret HASURA_GRAPHQL_ADMIN_SECRET --every 180d

# View all schedules and their status
nself secrets list-schedules
```

The Hasura admin secret is tracked as `HASURA_GRAPHQL_ADMIN_SECRET`, the variable the stack reads. A schedule saved under the older name `HASURA_ADMIN_SECRET` is renamed automatically the next time the rotation state loads, keeping its cadence and history.

Automated rotation fires when the `rotation` plugin is installed and
`NSELF_SECRET_ROTATION=true` is set. The plugin registers a cron job (via
the `cron` plugin) that calls its `/rotate/tick` endpoint every minute.

## Verifying a Secret Is Present

```bash
nself secrets verify JWT_SIGNING_KEY
# Secret JWT_SIGNING_KEY: PRESENT in dev environment.
```

## Viewing the Rotation Event Log

```bash
# All events
nself secrets rotation-log

# Filter to one secret
nself secrets rotation-log --secret JWT_SIGNING_KEY
```

## Schedule Status Reference

| Status | Meaning |
|--------|---------|
| `ok` | Rotation is not due yet (>7 days away) |
| `warning` | Rotation due within 7 days |
| `overdue` | Rotation is past due |
| `missing` | No NextDue timestamp set , run `nself secrets schedule` |

## Rollback on Failure

If the rotation plugin cannot verify service health after rotation, it:
1. Rolls back the secret to its previous value.
2. Writes a `rolled_back` event to the log.
3. Sends an alert to `NSELF_SECRET_ROTATION_NOTIFY_EMAIL`.

## Compliance References

- SOC 2 CC6.1, Logical access controls
- PCI-DSS 8.3.9, 90-day rotation for service accounts
- NIST SP 800-63B, Authentication credential management

## Related

- [[cmd-secrets]], full secrets command reference
- [[security/Supply-Chain]], supply-chain security baseline
- [[Home]]
