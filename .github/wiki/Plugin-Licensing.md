# Plugin Licensing

Licensed plugins require a valid ɳSelf license key. Free plugins are MIT licensed and require no key.

## Key Format

New keys are per-Bundle (`nself_claw_`, `nself_chat_` and so on) and are linked to your nSelf account. Older keys use the prefix format:

```
nself_pro_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

Legacy prefixes still accepted: `nself_pro_`, `nself_max_`, `nself_ent_`, `nself_owner_`. Link a legacy key to your account with `nself license migrate`.

Keys must be at least 32 characters total.

## Set Your License Key

```bash
nself license set nself_pro_xxxxx...
```

The key is stored at `~/.nself/license/key` (chmod 600) and also read from the `NSELF_PLUGIN_LICENSE_KEY` environment variable.

## Check License Status

```bash
nself license status
```

Shows your current tier, expiry, and which plugin categories are available.

## Remove Your Key

```bash
nself license remove
```

## Install Licensed Plugins

After setting your key:

```bash
nself plugin install ai
nself plugin install claw livekit chat
nself build
```

Tier entitlement is checked server-side at `ping.nself.org/license/validate`. The CLI never makes tier decisions locally.

## Validation

License validation sends:

```
POST https://ping.nself.org/license/validate
{"license_key": "...", "product": "plugins-pro"}
```

Response codes:
- `200`, Valid, allowed
- `401/403/404`, Invalid key or not entitled to the plugin

Validation results are cached locally for 24 hours at `~/.nself/license/cache`.

## Offline Mode

To skip remote validation (e.g., air-gapped environments), `NSELF_LICENSE_SKIP_VERIFY=1`
requires `--force` as explicit acknowledgment. Standalone skip without `--force` is rejected.

```bash
NSELF_LICENSE_SKIP_VERIFY=1 nself plugin install {name} --force
```

A warning is printed to stderr when `--force` is used. Without `--force`, the command
exits with a non-zero status and the message:

```
NSELF_LICENSE_SKIP_VERIFY requires --force flag; standalone skip is not permitted
```

The same requirement applies to `nself license import` when importing unsigned cache entries:

```bash
NSELF_LICENSE_SKIP_VERIFY=1 nself license import cache.json --force
```

## What a Key Unlocks

| Key | Monthly | Annual | Plugin Access |
|-----|---------|--------|--------------|
| None (free) | $0 | $0 | Free plugins and the Task Bundle |
| One Bundle (Chat, Claw, Family, Sentry or ClawDE) | $0.99 | $9.99 | The licensed plugins of that Bundle |
| ɳSelf+ | $3.99 | $39.99 | Every Bundle, every app, support |

Entitlement is decided server-side. Buy or manage a subscription at nself.org/pricing.

## Related Pages

- [[Plugin-Overview]], The offering and plugin categories
- [[Plugin-Install]], How to install plugins
