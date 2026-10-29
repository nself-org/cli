# Plugin Licensing

Licensed plugins require a valid ɳSelf license key. Free plugins are MIT licensed and require no key.

## Key Format

License keys use the prefix format:

```
nself_pro_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

Accepted prefixes:
- `nself_pro_`, paid bundles and ɳSelf+
- `nself_max_`, ɳSelf+ and enterprise
- `nself_ent_`, Enterprise tier
- `nself_owner_`, Owner/self-hosted tier

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
- `401/403/404`, Invalid or insufficient tier

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

## Bundle Entitlements

| Plan | Monthly | Annual | Plugin Access |
|------|---------|--------|--------------|
| Free (incl. Task bundle) | $0 | $0 | Free plugins only |
| Single bundle (Chat, Claw, Family, Sentry, ClawDE) | $0.99 | $9.99 | That bundle's licensed plugins |
| ɳSelf+ | $3.99 | $39.99 | Every bundle + all apps + support |

There is no Basic, Pro, Elite or Business tier. The authoritative catalog is [nself.org/pricing](https://nself.org/pricing), fetched by the CLI from `https://plugins.nself.org/bundles.json`. See [[Licensing-Guide]].

## Related Pages

- [[Plugin-Overview]], Bundle comparison and plugin categories
- [[Plugin-Install]], How to install plugins
