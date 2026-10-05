# Compat: NSELF_V15

← [[Config-Env-Vars]]

---

`NSELF_V15` is the one switch for behaviour changes that break an existing user-visible contract (JSON shapes, exit codes, command moves, tool names). Patch releases keep shipping from `main`, so a breaking change lands dormant: the old behaviour stays the default until v1.5.0 flips it (ADR 0021).

## What it does

| Value | Mode | Meaning |
|---|---|---|
| `1`, `true`, `TRUE`, `True` (any case of `true`) | v1.5 | New behaviour on. `1` is the ADR 0021 spelling. |
| unset, empty, `0`, `false`, `yes`, anything else | v1.4 | Old behaviour. |

Default: off in 1.4.x. On from v1.5.0, where the CLI stops reading the variable. New additive surfaces (a new flag, a new subcommand) are not gated.

The Go side is `internal/compat`: `compat.V15()` reads the environment on every call (never cached), and `compat.Mode()` returns `v1.4` or `v1.5`.

## Opt in early

```bash
NSELF_V15=1 nself status --json    # one command
export NSELF_V15=1                 # a whole shell, agent session or CI job
```

Agents and CI contract jobs run in both modes.

## For contributors

A gated branch carries this marker on the line above (or the same line as) the `compat.V15()` call:

```go
// compat.V15(P7-REG-09): old behaviour -> new behaviour
if compat.V15() {
```

`scripts/ci/compat-markers.sh` lists every gated branch as `<file>:<line> <ticket-id>`, fails when a call has no marker, and regenerates the table below:

```bash
bash scripts/ci/compat-markers.sh               # list markers
bash scripts/ci/compat-markers.sh --write-wiki  # regenerate the table (after rebase)
bash scripts/ci/compat-markers.sh --check       # exit 1 when the table is stale
```

Test both modes with `compattest.Both` from `internal/compat/compattest`; it runs the body as subtests `v1.4` and `v1.5`. Use `compattest.Set(t, on)` for one mode. Tests never branch on a bare `compat.V15()`.

## Gated behaviours

The table is generated. Never hand-edit it; run `compat-markers.sh --write-wiki`.

<!-- BEGIN GENERATED:gated -->
| Ticket | v1.4 behaviour (old) | v1.5 behaviour (new) | File |
|---|---|---|---|
| P7-PLUG-63 | bundle reply trusted for any key and bundle | only a signed reply naming this key, this bundle and a live window | `internal/license/cache_entry.go` |
| P7-PLUG-63 | cache age from the unsigned fetched_at | the older of fetched_at and the signed jwt iat | `internal/license/cache_entry.go` |
| P7-PLUG-63 | cache trusted whatever the clock says | refused when the clock is behind the signed iat or the highest time seen | `internal/license/cache.go` |
| P7-PLUG-63 | cache trusted without a signature check | only a cache whose server-signed body verifies | `internal/license/cache_entry.go` |
| P7-PLUG-63 | fail-open past the licence expiry | refused once the signed expiry plus the post-expiry grace has passed | `internal/license/cache.go` |
| P7-PLUG-63 | no trusted-time mark | mark raised to now after each trusted decision | `internal/license/cache.go` |
| P7-PLUG-63 | no trusted-time mark | mark reset to the signed iat of each verified reply | `internal/license/cache.go` |
| P7-PLUG-63 | plugins field ping never sends (always empty) | plugins_allowed | `internal/license/cache_entry.go` |
| P7-PLUG-63 | post-expiry grace ignores cache age | post-expiry write access also needs a cache younger than the 7-day offline ceiling | `internal/license/cache.go` |
| P7-PLUG-63 | signature over a locally built payload | server signature over the raw body | `internal/license/cache_entry.go` |
| P7-PLUG-63 | signed reply trusted for any licence | only a reply whose signed jwt names this key and is in window | `internal/license/cache_entry.go` |
| P7-PLUG-63 | unsigned bundle response accepted | unsigned bundle response refused | `internal/license/checker.go` |
| P7-REG-02 | bare pre-contract JSON | v1 envelope (NSELF_JSON_LEGACY=1 or true keeps bare JSON for one minor) | `internal/output/legacy.go` |
| P7-REG-05 | --json accepted and ignored on a command whose own json flag predates P7-REG | refused with E402 | `cmd/commands/invocation.go` |
| P7-REG-05 | flag and argument errors keep cobra's text | wrapped as E401 | `cmd/commands/invocation.go` |
| P7-REG-05 | registry reports v1.4 exit codes and hides v1.5-only envelopes | v1.5 exit codes and envelopes | `cmd/commands/registry.go` |
<!-- END GENERATED:gated -->
