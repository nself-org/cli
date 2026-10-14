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

## MCP tool results (P7-SURF-03)

In v1.5 mode `nself mcp` registers generated tools only. Thirteen legacy tool names are the generated name of their command, so each is now one tool whose result is the command's `data` (`structuredContent`, with the full envelope as text) and whose output schema is the command's committed data schema. The hand tool's result shape is gone. Affected tools: `nself_status`, `nself_doctor`, `nself_logs`, `nself_config_get`, `nself_config_show`, `nself_config_set`, `nself_build`, `nself_start`, `nself_stop`, `nself_restart`, `nself_db_migrate_status`, `nself_backup_list`, `nself_deploy_status`. The old parameters still work for one minor (the call adds `meta.deprecations`, removal v1.6.0); read the new result from `structuredContent`. Details and the alias table: [[MCP-Server]].

## Gated behaviours

The table is generated. Never hand-edit it; run `compat-markers.sh --write-wiki`.

<!-- BEGIN GENERATED:gated -->
| Ticket | v1.4 behaviour (old) | v1.5 behaviour (new) | File |
|---|---|---|---|
| D-0216 | decrypt-on-deploy prints KEY=VALUE to any stdout and puts values in --json | plaintext needs a terminal stdout or --reveal; --json carries names only | `cmd/commands/secrets_decrypt.go` |
| D-0274 | installer prints 'run nself build' after every install | no hint when the caller reconciles | `internal/plugin/installer_hint.go` |
| D-0285 | bare data-loss command runs with a deprecation warning | refused (E403) unless --yes or a terminal yes | `cmd/commands/data_consent.go` |
| D-0287 | two plugins mapping to one nginx/sites file overwrite silently (warning) | E055 fails the build | `internal/build/plugin_nginx_collision.go` |
| D-0327 | no Host check on mcp sse/http | Host must be localhost/127.0.0.1/[::1] or an --allowed-host, with the listener port | `cmd/commands/mcp_auth.go` |
| D-0330 | remove <plugin> drops plugin data unless --keep-data | keeps it unless --delete-data, confirmed by --yes or a terminal | `cmd/commands/plugin_remove_data.go` |
| D-0430 | "Error: trust warnings present (exit code 2)" and exit 1 | silent exit 2, the canon's warning class | `cmd/commands/trust_status_detailed.go` |
| D-0478 | PLUGIN_DIR "~/..." used as written, no plugin snippets on default config | "~" expanded, installed plugins' snippets injected | `internal/build/plugin_configs.go` |
| D-0484 | pin store ancestor walk warns once and continues | refuses | `sdk/go/remote/knownhosts_compat.go` |
| D-0676 | bare stream path streams as path:// with a notice | refused | `internal/backup/stream_dest_compat.go` |
| P7-ADOPT-01 | writable absolute bind allowed | E502 | `internal/compose/custom_service_v2.go` |
| P7-ADOPT-03 | warn | E505 | `internal/build/shared_proxy.go` |
| P7-ADOPT-13 | --project-ref and --supabase-key required | optional | `cmd/commands/db_import_src_supabase.go` |
| P7-ADOPT-13 | artifact generator | import pipeline | `cmd/commands/migrate_supabase.go` |
| P7-ADOPT-14 | artifact generator | import pipeline | `cmd/commands/migrate_firebase.go` |
| P7-CANON-01 | same-owner breakout stays core in v1.4 | mounts from its plugin in v1.5 | `cmd/commands/plugin_mount.go` |
| P7-CANON-02 | registry view with moved entries at their v1.4 paths | view at canonical paths plus deprecated-shim entries | `internal/canon/canon.go` |
| P7-CANON-05 | help | next step + help | `cmd/commands/root.go` |
| P7-CANON-08 | no hint | Next: nself <command> --help | `cmd/commands/install_builtin.go` |
| P7-CANON-19 | account group | plugin commands group | `cmd/commands/builtin_families.go` |
| P7-CANON-21 | the argv names commands at their v1.4 paths | argv rewritten between old and canonical spellings, tree relocated | `cmd/commands/tree_prepare.go` |
| P7-DEPL-01 | legacy duplicate errors | E055 with both route IDs | `internal/build/orchestrator_build_config.go` |
| P7-DEPL-13 | accept-new | strict for secret shipping and prod targets. | `internal/deploy/ssh_hostkeys.go` |
| P7-DEPL-13 | accept-new | strict host key checking for access writes. | `internal/access/transport_ssh.go` |
| P7-DEPL-13 | quiet read-only first contact | print the observed fingerprint once. | `internal/controlplane/hostkeys.go` |
| P7-DEPL-13 | silent legacy host path | warn once until removal at v1.6.0. | `internal/controlplane/tiers.go` |
| P7-DEPL-14 | direct SSH deploy | canonical deploy pipeline delegation. | `cmd/commands/ops.go` |
| P7-DEPL-14 | direct SSH policy | caller-supplied inventory host-key policy. | `internal/access/transport_ssh.go` |
| P7-DEPL-14 | silent legacy host fallback | deprecation warning. | `cmd/commands/deploy_profile.go` |
| P7-DEPL-14 | unpinned admin SSH | pinned host-key policy. | `internal/admin/connect.go` |
| P7-DEPL-14 | unrestricted inventory writes | prod-class confirmation. | `cmd/commands/access_targets.go` |
| P7-DEPL-15 | deploy promote ignored args as a canary flip | two args promote environments | `cmd/commands/deploy_promote.go` |
| P7-DEPL-15 | local internal/promote implementation | deploy promote control-plane engine | `cmd/commands/promote.go` |
| P7-DEPL-15 | local promote snapshot | verified remote promote record | `cmd/commands/promote.go` |
| P7-DEPL-15 | uncertain restore repeated | explicit operator completion checkpoint | `cmd/commands/deploy_promote_confirm.go` |
| P7-DEPL-15 | unconfirmed promote rollback | require production database restore approval | `cmd/commands/deploy_promote_confirm.go` |
| P7-DEPL-24 | DeployViaSsh single-compose push | ApplyRemote artifact-set apply | `internal/controlplane/remote_apply.go` |
| P7-DEPL-24 | RunWithEnv single-compose push | target snapshot, scratch build, artifact-set apply | `cmd/commands/deploy_apply_v15.go` |
| P7-DEPL-24 | env snapshot written into the operator's project dir | written in the scratch dir at apply time | `cmd/commands/deploy_apply_v15.go` |
| P7-DEPL-24 | promote snapshot rollback | restore the remote pre-deploy tar (and the prod database backup) in place | `cmd/commands/deploy_lifecycle.go` |
| P7-DEPL-24 | remote build in the operator's project dir | scratch build after the target snapshot | `cmd/commands/deploy_scratch.go` |
| P7-DEPL-24 | remoteDeployPush single-compose rsync + pull + up | artifact-set apply | `cmd/commands/deploy_apply_v15.go` |
| P7-DEPL-29 | accept-new seeds ~/.ssh/known_hosts | accept-new seeds nself's TOFU file, operator file read-only | `internal/deploy/ssh_tofu.go` |
| P7-LIVE-03 | a lock held by another command does not stop a build (it takes the O_EXCL build.lock) | a lock held by another command refuses the build | `internal/build/build_lock.go` |
| P7-LIVE-03 | a prod-class or hand-edited change proceeds with a notice | refused with E403 without --yes or --force | `internal/reconcile/apply.go` |
| P7-LIVE-05 | --json accepted and ignored | v1 envelope {key, file, applied, plan} | `cmd/commands/config_set.go` |
| P7-LIVE-05 | env line written, regeneration left to the next build | plan, confirm (prod-class), write the line, regenerate | `cmd/commands/config_set_plan.go` |
| P7-LIVE-06 | plugin install leaves generated state for a later build | reconcile now | `cmd/commands/plugin_install.go` |
| P7-LIVE-06 | plugin removal leaves generated state for a later build | reconcile now | `cmd/commands/plugin_lifecycle.go` |
| P7-LIVE-06 | post-install build hint | automatic reconcile | `cmd/commands/plugin_install.go` |
| P7-LIVE-09 | the project lock is written and honoured only when nself.lock.yaml exists or --write-lock is given | always | `internal/build/lockfile.go` |
| P7-LIVE-10 | plan keeps the redis pin | plan moves it to valkey | `cmd/commands/update_plan_sources.go` |
| P7-LIVE-13 | a held lock is polled for 30 s, then a warning and the command runs unlocked | a held lock fails at once with E460 | `internal/oplock/guard.go` |
| P7-LIVE-17 | IMAGE_PINNING defaults to legacy | lock | `internal/compose/images_lock.go` |
| P7-LIVE-19 | the default cache engine is redis | valkey | `internal/compose/cache_service.go` |
| P7-LIVE-20 | auth runs nhost/auth:0.36.0 (auth-v14) | the current nhost/auth release (auth) when AUTH_ENCRYPTION_KEY is set | `internal/compose/auth_image.go` |
| P7-LIVE-22 | E151 for every ACME failure (exit of the failed process, else 2) | E470/E471 (exit 2) or E472 (exit 1) by failure class | `cmd/commands/ssl_renewal.go` |
| P7-LIVE-22 | certbot is the default certificate path | the ACME engine is the default | `cmd/commands/ssl_renewal.go` |
| P7-LIVE-22 | remediation uses trust ssl with --acme | canonical config trust ssl without --acme | `cmd/commands/ssl_acme.go` |
| P7-LIVE-23 | hasura runs graphql-engine v2.44.0 (hasura-v14) | v2.51.0 (hasura) | `internal/compose/core_services.go` |
| P7-PLUG-01 | v1 plugin.json read silently | one deprecation line per process on stderr | `internal/plugin/manifestv2/warn.go` |
| P7-PLUG-09 | generic "nself install <command>" hint | the catalog plugin that provides the command, with nself add | `internal/plugin/router.go` |
| P7-PLUG-09 | search over the registry (free tier, NDJSON) | search over the catalog (both tiers, v1 envelope) | `cmd/commands/plugin_search.go` |
| P7-PLUG-11 | tier pro | license licensed | `cmd/commands/plugin_marketplace_cmds.go` |
| P7-PLUG-11 | tier pro | license licensed | `cmd/commands/plugin_query.go` |
| P7-PLUG-11 | tier pro | license licensed | `cmd/commands/plugin_search.go` |
| P7-PLUG-17 | --json is ignored by plugin info | prints the info with its tier and why as JSON | `cmd/commands/plugin_info.go` |
| P7-PLUG-17 | a compose plugin without a fragment is skipped silently | the build fails with E128 (v1.4 warns) | `internal/build/plugins.go` |
| P7-PLUG-17 | a fragment may join any external network (warning) | E129 fails the build | `internal/build/plugins_network_normalize.go` |
| P7-PLUG-17 | a fragment that decodes as nodes but not as values is passed on unchecked | E129, the network policy could not be checked | `internal/build/plugins_network_normalize.go` |
| P7-PLUG-17 | a fragment with several YAML documents is checked on the first only (warning) | E129, the fragment must be one document | `internal/build/plugins_network_normalize.go` |
| P7-PLUG-17 | a service may set any network_mode (host, container:x, a network name) | E129 unless it is none or service:<a service of this fragment> | `internal/build/plugins_network_normalize.go` |
| P7-PLUG-17 | an update re-resolves a tier pair by entitlement and can switch the installed tier | the installed tier sticks unless --tier is given | `internal/plugin/tier_resolve.go` |
| P7-PLUG-17 | external network nself_network keeps its literal name | its definition is dropped and service references use ${DOCKER_NETWORK} | `internal/build/plugins_network_legacy.go` |
| P7-PLUG-17 | fragments get no PORT | PORT equals SERVICE_PORT (a fragment's own PORT always wins) | `internal/build/plugins_core_env.go` |
| P7-PLUG-17 | fragments keep their own networks and ${DOCKER_NETWORK:-x} | missing networks attached, ${DOCKER_NETWORK:-x} written as ${DOCKER_NETWORK} | `internal/build/plugins_network_normalize.go` |
| P7-PLUG-17 | info describes the first registry match | describes the installed tier's entry, or the one an install would pick | `cmd/commands/plugin_info.go` |
| P7-PLUG-17 | the Tier row shows the registry word | shows the licence word and why (Tier reason) | `cmd/commands/plugin_info.go` |
| P7-PLUG-19 | bundle install not app-ready exits 0 with a warning | E135 exit 2 | `internal/bundle/installer_appready.go` |
| P7-PLUG-19 | bundle install reconciles only | also starts the stack and waits for app-ready | `cmd/commands/bundle_appready.go` |
| P7-PLUG-19 | plugin install readiness failure warns | E118 exit 2 | `cmd/commands/plugin_install_readiness.go` |
| P7-PLUG-19 | plugin install readiness setup problems warn | returned error | `cmd/commands/plugin_install_readiness.go` |
| P7-PLUG-20 | --keep-data keeps plugin data | deprecated no-op (data is kept by default) | `cmd/commands/bundle_install_remove.go` |
| P7-PLUG-20 | bundle remove drops plugin data by default (paid bundle) | keeps it unless --delete-data | `internal/bundle/remover_plan.go` |
| P7-PLUG-20 | bundle remove of an unrecorded bundle removes its members | fails with E134 | `internal/bundle/remover_plan.go` |
| P7-PLUG-20 | remove <plugin> of a bundle member proceeds | refuses with E133 unless --force | `internal/bundle/remove_guard.go` |
| P7-PLUG-23 | a plugin with no verifiable release signature installs with a warning | refused with E107 | `internal/plugin/installer_signature.go` |
| P7-PLUG-24 | discovery needs no JWT | licensed runtime files refresh before build completes | `internal/build/plugins.go` |
| P7-PLUG-24 | no runtime entitlement diagnostic | warn on stale signed entitlement | `internal/doctor/entitlement_file.go` |
| P7-PLUG-24 | no runtime entitlement file | verified JWT publication and E125 on failure | `internal/license/entitlement/write.go` |
| P7-PLUG-24 | outside-project set saves only format | validates online for the next project build | `internal/license/entitlement/installed.go` |
| P7-PLUG-24 | overlapping fragments merge freely | licensed service collisions fail closed | `internal/build/plugins_entitlement_fragments.go` |
| P7-PLUG-24 | plugin compose has no entitlement mount | licensed services get the signed JWT file | `internal/build/plugins_entitlement_mount.go` |
| P7-PLUG-24 | start and license set ignore entitlement | refresh licensed installed plugins | `internal/license/entitlement/installed.go` |
| P7-PLUG-25 | clone template scaffolds without checking its plugins | refused (E110, E137, or E115 with no catalog) before any file is written | `internal/templates/clone/entitlement.go` |
| P7-PLUG-60 | MinIO endpoint | active core or plugin object store endpoint | `internal/compose/custom_services.go` |
| P7-PLUG-60 | S3 keys infer MinIO intent | core object storage keys do not enable MinIO | `internal/config/loader_parse_env_storage.go` |
| P7-PLUG-60 | S3 vars for every profile | none for profiles without object storage | `internal/compose/object_storage.go` |
| P7-PLUG-60 | core MinIO generation | missing plugin fails before writes | `internal/compose/object_storage.go` |
| P7-PLUG-60 | demo enables core MinIO | demo enables SeaweedFS object storage | `internal/setup/setup_env_files.go` |
| P7-PLUG-60 | media-hosting recommends MinIO | recommends core object storage and the MinIO plugin for existing data | `cmd/commands/init_presets.go` |
| P7-PLUG-60 | no object storage line | OBJECT_STORAGE_ENABLED=false listed with the optional services | `internal/setup/setup_env_files.go` |
| P7-PLUG-60 | optional core MinIO | core SeaweedFS or MinIO plugin | `internal/compose/object_storage.go` |
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
| P7-PLUG-78 | bundle install only verifies Hasura tracking | tracks the manifest tables first | `cmd/commands/bundle_appready.go` |
| P7-PLUG-78 | plugin install does not track manifest tables | tracks them in Hasura after readiness (E140) | `cmd/commands/plugin_install_readiness.go` |
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
| P7-SURF-03 | 26 hand tools primary | generated tools, merged same-name tools, deprecated aliases | `cmd/commands/mcp.go` |
| P7-SURF-07 | nself.yaml type errors and unknown keys warn | fail (E435/E436) | `internal/build/manifest_validate.go` |
| P7-SURF-11 | health history --json with no history prints a text line | the v1 envelope with an empty entries list | `cmd/commands/health_handlers.go` |
| P7-SURF-12 | an empty list is the JSON array [] (v1.4 and NSELF_JSON_LEGACY=1 print null) | v1 envelope with an array | `cmd/commands/config_envelope.go` |
| P7-SURF-14 | --json on the data commands is ignored or the pre-contract form (NSELF_JSON_LEGACY=1 then prints an empty list as bare []) | v1 envelope | `cmd/commands/db_envelope.go` |
| P7-SURF-15 | deploy, status, check-access, access list --json print a bare document (NSELF_JSON_LEGACY=1 keeps it) | v1 envelope | `cmd/commands/deploy_envelope.go` |
| P7-SURF-25 | no token = open loopback with a warning | token required (generated on first start) | `cmd/commands/mcp_auth.go` |
| P7-SURF-26 | --json ignored | envelope | `cmd/commands/registry_types_build.go` |
| P7-SURF-27 | logs --json per-line legacy JSON | envelope / json-stream | `cmd/commands/logs_json.go` |
<!-- END GENERATED:gated -->
