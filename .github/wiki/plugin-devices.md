# Devices Plugin

> IoT device enrollment, trust management, and command dispatch with telemetry ingestion. **Pro plugin.**

> **Requires:** Pro license tier or higher. `nself license set nself_pro_...`

## Install

```bash
nself license set nself_pro_xxxxx...
nself plugin install devices
```

The service listens on port `3603`.

## What It Does

The Devices plugin manages a fleet of IoT devices through three stages: enrollment, trust, and dispatch.

1. **Enrollment.** A device requests registration using a bootstrap token (`POST /devices`). The plugin issues a short-lived enrollment token, scoped by `DEV_ENROLLMENT_TOKEN_TTL` (default `3600` seconds), and records the pending device in `np_dev_devices`.
2. **Trust lifecycle.** Once enrolled, a device holds a trust state stored against its row. Operators can `suspend` a misbehaving device or `revoke` trust entirely, which invalidates its tokens and blocks further command delivery. A challenge step (`DEV_CHALLENGE_TTL`) and heartbeat tracking (`DEV_HEARTBEAT_INTERVAL`, `DEV_HEARTBEAT_TIMEOUT`) keep the trust state current; a device that stops sending heartbeats transitions to an offline state.
3. **Command dispatch.** Operators queue a command for a device (`POST /devices/{id}/commands`). Commands are retried per `DEV_COMMAND_MAX_RETRIES` with a timeout of `DEV_COMMAND_DEFAULT_TIMEOUT`, and the device acknowledges over its dispatch channel. Telemetry flows back via `POST /telemetry` (single) or `POST /telemetry/batch`, retained for `DEV_TELEMETRY_RETENTION_DAYS` (default `90`).

All lifecycle events (enrollment, command acknowledgement, suspension, revocation, disconnect) are written to a structured audit log.

## CLI Commands

```bash
nself plugin run devices devices enroll     # Start enrollment for a device
nself plugin run devices devices revoke     # Revoke device trust
nself plugin run devices devices suspend    # Suspend a device
nself plugin run devices commands send      # Send a command to a device
nself plugin run devices health             # Device health summary
nself plugin run devices stats              # Fleet-wide statistics
```

## API

Health checks live at `GET /health` and `GET /ready`. Device CRUD is under `/devices` (`GET`, `POST`, `GET/PUT/DELETE /devices/{id}`). Command dispatch is `GET/POST /devices/{id}/commands` and `GET /commands/{id}`. Telemetry ingestion is `POST /telemetry`, `POST /telemetry/batch`, and `GET /devices/{id}/telemetry`. The audit trail is `GET /audit`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `DEVICES_PORT` | `3603` | Port the Devices plugin service listens on |
| `DEVICES_API_KEY` | — | API key guarding the plugin's HTTP surface |
| `DEVICES_RATE_LIMIT_MAX` | — | Max requests per rate-limit window |
| `DEV_ENROLLMENT_TOKEN_TTL` | `3600` | Enrollment token lifetime in seconds |
| `DEV_CHALLENGE_TTL` | — | Trust challenge lifetime in seconds |
| `DEV_HEARTBEAT_INTERVAL` | — | Expected device heartbeat interval |
| `DEV_HEARTBEAT_TIMEOUT` | — | Heartbeat silence before a device is marked offline |
| `DEV_COMMAND_DEFAULT_TIMEOUT` | — | Default command acknowledgement timeout |
| `DEV_COMMAND_MAX_RETRIES` | — | Max command delivery retries |
| `DEV_TELEMETRY_RETENTION_DAYS` | `90` | Days to retain telemetry data |

## Database Tables

5 tables added to your Postgres database (multi-app isolated via `source_account_id`):

- `np_dev_devices`, enrolled device records, trust state, and heartbeat status
- `np_dev_commands`, queued and acknowledged commands per device
- `np_dev_telemetry`, ingested telemetry data points
- `np_dev_ingest_sessions`, active and historical ingest sessions
- `np_dev_audit_log`, device lifecycle and connection event log

## Ports

| Port | Purpose |
|------|---------|
| `3603` | Devices plugin HTTP service |

## Nginx Routes

| Route | Description |
|-------|-------------|
| `/devices/` | Proxied to Devices plugin service on port 3603 |

## Licensing

Devices is a Pro, license-gated plugin (`requires_license: true`). Under the Security-Always-Free doctrine, core deployment hardening is free and automatic; device fleet management (enrollment, trust lifecycle, command dispatch, telemetry retention) is an operator-facing product, so it ships as a Pro feature.

## Source

Source-available (license required to run): [`bundles/paid/devices/`](https://github.com/nself-org/bundles/tree/main/paid/devices)

Note: `plugins-pro` is a private repository. Source access is granted to ɳSelf+ subscribers and Enterprise customers.

## See Also

- [Plugin-Install](Plugin-Install) — general plugin install flow
- [Plugin-Catalog](Plugin-Catalog) — full plugin list
- [Plugin-Licensing](Plugin-Licensing) — tier and license model

← [[Plugin-Catalog]] | [[Home]] →
