# License Verification (Offline Mode)

ɳSelf is offline-first. License verification works on disconnected, intermittent, or air-gapped networks. The CLI caches a signed license artifact and validates it locally on every command.

## Fail-Open Policy

The CLI prefers availability over strict revocation. Two independent clocks govern behavior, and they are easy to confuse:

- **Cache age** — how long since the CLI last reached `ping.nself.org`. This is the offline ladder below.
- **License expiry** — the server-reported end of the subscription. See [Post-Expiry Grace](#post-expiry-grace).

### Offline ladder (cache age)

Thresholds are `GraceSoftThreshold` and `GraceHardThreshold` in `internal/license/grace.go`.

| Cache age | State | Behavior | User signal |
|---|---|---|---|
| Under 72 hours | `valid` | Fail-open, silent | None |
| 72 hours to 7 days | `grace_soft` | Fail-open, full access, warning | `License validation is N old. Connect to the internet to refresh.` plus the remaining window |
| Over 7 days | `grace_hard` | **Read-only** — commands still run, writes are refused | `License validation expired (N offline). Paid plugins are in read-only mode.` |
| Bad signature, any age | — | Fail-closed, always | Signature verification failure |

Two properties of this table are load-bearing and are the ones most often misremembered:

**72 hours, not 7 days, is when the warning starts.** The silent window is deliberately sized to cover a Friday-evening-to-Monday-morning outage *on our side* with margin, so a blip on the license server never alarms a paying customer mid-weekend.

**Past 7 days the CLI degrades to read-only — it does not refuse to run.** `CanProceed` stays true and only `WriteAllowed` flips to false. You keep your stack readable and inspectable while offline; you cannot mutate it until you refresh.

The 7-day ceiling does **not** widen alongside the soft threshold. Validation sends only the license key over the wire, with no per-machine identifier, so a local cache is a bare copyable credential. Every extra day of ceiling multiplies that exposure, and 7 days is the accepted tradeoff between outage tolerance and copied-cache abuse.

A bad signature is never accepted. Cache age cannot bypass cryptographic verification — the cache carries an Ed25519 signature that is checked locally on every command.

### Post-expiry grace

Separately from the offline ladder, a license whose server-reported expiry has passed keeps working for **30 days** (`PostExpiryGraceWindow`), with a warning, before paid plugins go dormant. Reaching this state means the subscription lapsed, not that the network is down; refreshing the cache will not clear it.

## Configuration

| Variable | Default | Purpose |
|---|---|---|
| `LICENSE_CACHE_PATH` | `~/.cache/nself/license.json` | Cache file location. Override for shared CI runners or air-gapped hosts. |

The offline thresholds are compile-time constants, not configuration. There is no environment variable that widens or narrows the grace window — tightening posture is done by refreshing more often, not by lowering a cap.

## Manual Refresh

```bash
nself license refresh
```

Pulls a fresh signed artifact from `ping.nself.org/license/validate` for every configured key, replaces the cache, and resets the age clock. Run after extended offline periods, after key rotation, or when the warning fires.

## Air-Gapped Mode

For hosts that never reach the public internet, move a signed cache across the air gap with `export` / `import`. Run the export on a machine that *can* reach `ping.nself.org` and holds the same license key:

```bash
# On the connected machine
nself license refresh
nself license export > license-cache.json

# Transfer the file, then on the air-gapped host
nself license import license-cache.json
```

The imported entry keeps its Ed25519 signature and is verified locally, so an air-gapped host gets the same guarantees as a connected one. The transferred cache ages on the same ladder above, so repeat the transfer before the 7-day ceiling to stay out of read-only mode.

`NSELF_LICENSE_SKIP_VERIFY=1` exists for importing an unsigned entry and requires `--force` to be acknowledged explicitly. It bypasses signature verification — do not use it outside local testing.

## Verifying the ladder

`simulate-offline` backdates the cache so you can exercise each band without waiting. It requires `LICENSE_ALLOW_SIMULATION=true` and is disabled by default:

```bash
LICENSE_ALLOW_SIMULATION=true nself license simulate-offline 1    # silent
LICENSE_ALLOW_SIMULATION=true nself license simulate-offline 5    # warning
LICENSE_ALLOW_SIMULATION=true nself license simulate-offline 10   # read-only
nself license simulate-offline --clear                            # reset
```

## Troubleshooting

**Cache corruption.** Delete `~/.cache/nself/license.json` and run `nself license refresh`. The cache is regenerable.

**Signature invalid after CLI upgrade.** Run `nself license refresh`. Major version bumps may rotate signing keys.

**Manual signature verification.** There is no separate verify subcommand — every command validates the cached signature locally before it runs. To inspect the cache contents without triggering a network call, print it directly:

```bash
nself license show --json
```

**Stuck in read-only.** The cache is more than 7 days old. Connect to the internet and run `nself license refresh`. If the network is restricted, use the air-gap `export` / `import` flow above.

## See Also

- [License Management](license-management.md)
- [Plugin Installation](plugins.md)
- [nself license commands](commands/license.md)
