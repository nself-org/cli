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
| P7-ADOPT-01 | writable absolute bind allowed | E502 | `internal/compose/custom_service_v2.go` |
| P7-CANON-01 | same-owner breakout stays core in v1.4 | mounts from its plugin in v1.5 | `cmd/commands/plugin_mount.go` |
| P7-CANON-02 | registry view with moved entries at their v1.4 paths | view at canonical paths plus deprecated-shim entries | `internal/canon/canon.go` |
| P7-CANON-05 | help | next step + help | `cmd/commands/root.go` |
| P7-CANON-08 | no hint | Next: nself <command> --help | `cmd/commands/install_builtin.go` |
| P7-CANON-19 | account group | plugin commands group | `cmd/commands/builtin_families.go` |
| P7-CANON-21 | the argv names commands at their v1.4 paths | argv rewritten between old and canonical spellings, tree relocated | `cmd/commands/tree_prepare.go` |
| P7-DEPL-01 | legacy duplicate errors | E055 with both route IDs | `internal/build/orchestrator_build_config.go` |
| P7-DEPL-13 | accept-new | strict for secret shipping and prod targets. | `internal/deploy/ssh.go` |
| P7-DEPL-13 | accept-new | strict host key checking for access writes. | `internal/access/transport_ssh.go` |
| P7-DEPL-13 | quiet read-only first contact | print the observed fingerprint once. | `internal/controlplane/hostkeys.go` |
| P7-DEPL-13 | silent legacy host path | warn once until removal at v1.6.0. | `internal/controlplane/tiers.go` |
| P7-DEPL-14 | direct SSH deploy | canonical deploy pipeline delegation. | `cmd/commands/ops.go` |
| P7-DEPL-14 | direct SSH policy | caller-supplied inventory host-key policy. | `internal/access/transport_ssh.go` |
| P7-DEPL-14 | silent legacy host fallback | deprecation warning. | `cmd/commands/deploy_profile.go` |
| P7-DEPL-14 | unpinned admin SSH | pinned host-key policy. | `internal/admin/connect.go` |
| P7-DEPL-14 | unrestricted inventory writes | prod-class confirmation. | `cmd/commands/access_targets.go` |
| P7-LIVE-03 | a lock held by another command does not stop a build (it takes the O_EXCL build.lock) | a lock held by another command refuses the build | `internal/build/build_lock.go` |
| P7-LIVE-03 | a prod-class or hand-edited change proceeds with a notice | refused with E403 without --yes or --force | `internal/reconcile/apply.go` |
| P7-LIVE-06 | plugin install leaves generated state for a later build | reconcile now | `cmd/commands/plugin_install.go` |
| P7-LIVE-06 | plugin removal leaves generated state for a later build | reconcile now | `cmd/commands/plugin_lifecycle.go` |
| P7-LIVE-06 | post-install build hint | automatic reconcile | `cmd/commands/plugin_install.go` |
| P7-LIVE-13 | a held lock is polled for 30 s, then a warning and the command runs unlocked | a held lock fails at once with E460 | `internal/oplock/guard.go` |
| P7-LIVE-17 | IMAGE_PINNING defaults to legacy | lock | `internal/compose/images_lock.go` |
| P7-PLUG-01 | v1 plugin.json read silently | one deprecation line per process on stderr | `internal/plugin/manifestv2/warn.go` |
| P7-PLUG-11 | tier pro | license licensed | `cmd/commands/plugin_marketplace_cmds.go` |
| P7-PLUG-11 | tier pro | license licensed | `cmd/commands/plugin_query.go` |
| P7-PLUG-11 | tier pro | license licensed | `cmd/commands/plugin_search.go` |
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
| P7-PROD-08 | age-key.txt only | shared identity search, E223 when none | `internal/backup/restore.go` |
| P7-PROD-08 | age-key.txt only | shared identity search, E223 when none | `internal/backup/stream_restore_remote.go` |
| P7-PROD-08 | only FATAL or "could not" fails | any unlisted pg_restore error: line fails | `internal/backup/drill_support.go` |
| P7-PROD-08 | refuse | auto identity | `internal/backup/autokey.go` |
| P7-REG-02 | bare pre-contract JSON | v1 envelope (NSELF_JSON_LEGACY=1 or true keeps bare JSON for one minor) | `internal/output/legacy.go` |
| P7-REG-05 | --json accepted and ignored on a command whose own json flag predates P7-REG | refused with E402 | `cmd/commands/invocation.go` |
| P7-REG-05 | flag and argument errors keep cobra's text | wrapped as E401 | `cmd/commands/invocation.go` |
| P7-REG-05 | registry reports v1.4 exit codes and hides v1.5-only envelopes | v1.5 exit codes and envelopes | `cmd/commands/registry.go` |
| P7-REG-06 | a panic crashes the process with the raw Go runtime trace and exit 2 | the panic is reported as an "internal error" (error envelope in JSON mode, stack trace on stderr only), still exit 2 | `cmd/nself/safe.go` |
| P7-REG-06 | exit status 1 and a plain "Error: msg" line for every failure | exit classes 2/3/4, "Error: [Exxx]" block on stderr and a JSON error envelope on stdout | `cmd/nself/report.go` |
| P7-REG-09 | --json ignored | envelope | `cmd/commands/config_list_validate.go` |
| P7-REG-09 | --json ignored | envelope | `cmd/commands/config_show_get.go` |
| P7-REG-09 | bare pre-contract JSON | v1 envelope whose data adds `state` | `cmd/commands/config_json_types.go` |
| P7-REG-09 | doctor --json exits 0 | 10 (failed) or 12 (warnings only) | `cmd/commands/doctor_health_report.go` |
| P7-REG-09 | doctor --json prints the passed-check lines on stdout before the JSON | on stderr, so stdout is one document | `cmd/commands/doctor_health_report.go` |
| P7-REG-09 | doctor exits 1 (failed) or 2 (warnings only) | 10 (failed) or 12 (warnings only) | `cmd/commands/doctor_health_report.go` |
| P7-REG-09 | human status exits 2 | 10 (unhealthy) or 11 (starting) | `cmd/commands/status.go` |
| P7-REG-09 | status exits 2 (unhealthy) or 1 (starting), 0 with --json | 10 (unhealthy) or 11 (starting) in both modes | `cmd/commands/status_print.go` |
| P7-SURF-07 | nself.yaml type errors and unknown keys warn | fail (E435/E436) | `internal/build/manifest_validate.go` |
<!-- END GENERATED:gated -->
