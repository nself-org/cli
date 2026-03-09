# CLI Test Coverage — v1.0.0

Last updated: 2026-03-08

## Summary

| Metric | Count |
| --- | --- |
| Total bats test files | 52 |
| Total bats test cases | ~750 (estimated) |
| Top-level commands covered | 30/30 |
| Integration test files | 12 |
| Commands with no test coverage | 0 |

## File Inventory

### Root test files (bats)

| File | Domain | Cases (approx) | Docker required |
| --- | --- | --- | --- |
| `admin_tests.bats` | Admin command | 18 | Yes |
| `auth_user_tests.bats` | Auth / user management | 24 | Yes |
| `auto_fix_tests.bats` | Auto-fix on start | 12 | No |
| `backup_restore_test.bats` | Backup + restore cycle | 16 | Partial |
| `backup_tests.bats` | Backup extended | 20 | Yes |
| `billing_stripe_tests.bats` | Stripe billing plugin | 14 | Yes |
| `build_security_test.bats` | Build secret validation | 10 | No |
| `build_tests.bats` | nself build | 22 | Partial |
| `command_tree_test.bats` | All 30 top-level --help | 52 | No |
| `commands_test.bats` | Frontend + plugin + license | 22 | No |
| `compatibility_tests.bats` | Bash 3.2 compat | 16 | No |
| `compliance_tests.bats` | Compliance tooling | 10 | No |
| `config_tests.bats` | Config management | 18 | No |
| `database_tests.bats` | DB operations | 20 | Yes |
| `db_commands_test.bats` | nself db subcommands | 14 | Partial |
| `deploy_tests.bats` | Deploy workflows | 16 | Yes |
| `dev_tests.bats` | Dev mode | 10 | No |
| `email_tests.bats` | Email service | 12 | Yes |
| `env_tests.bats` | Env var handling | 14 | No |
| `helm_tests.bats` | Helm charts | 8 | No |
| `hooks_tests.bats` | CLI hooks | 10 | No |
| `init_tests.bats` | nself init | 18 | No |
| `install_tests.bats` | Installer | 8 | No |
| `k8s_tests.bats` | Kubernetes support | 8 | No |
| `migrate_tests.bats` | DB migrations | 14 | Yes |
| `monitoring_tests.bats` | Monitoring bundle | 16 | Yes |
| `nself_tests.bats` | Core CLI | 12 | No |
| `oauth_tests.bats` | OAuth flows | 14 | Yes |
| `observability_tests.bats` | Observability plugin | 10 | Yes |
| `org_tests.bats` | Org management | 8 | No |
| `plugins_tests.bats` | Plugin system | 22 | No |
| `providers_tests.bats` | Providers | 10 | No |
| `rate_limit_tests.bats` | Rate limiting | 12 | No |
| `realtime_tests.bats` | Realtime plugin | 14 | Yes |
| `recovery_tests.bats` | Recovery flows | 10 | Yes |
| `redis_tests.bats` | Redis operations | 12 | Yes |
| `resilience_tests.bats` | Resilience / auto-heal | 10 | Yes |
| `secrets_encryption_tests.bats` | Secrets encryption | 14 | No |
| `secrets_vault_tests.bats` | Secrets vault | 12 | No |
| `security_audit_test.bats` | Security audit command | 10 | No |
| `security_tests.bats` | Security hardening | 16 | No |
| `service_init_tests.bats` | Service init | 10 | No |
| `services_functions.bats` | Service functions | 12 | No |
| `ssl_tests.bats` | SSL certificates | 14 | Partial |
| `start_tests.bats` | nself start | 14 | Yes |
| `storage_tests.bats` | MinIO storage | 16 | Yes |
| `tenant_tests.bats` | Multi-tenancy | 20 | Yes |
| `upgrade_tests.bats` | Self-update | 10 | No |
| `utils_tests.bats` | Utility functions | 14 | No |
| `webhooks_tests.bats` | Webhooks | 12 | Yes |
| `whitelabel_tests.bats` | White-label | 10 | No |
| `wizard_tests.bats` | Setup wizard | 8 | No |

### Integration test files

Located in `integration/`:

| File | Domain | Docker required |
| --- | --- | --- |
| `build_start_stop_test.bats` | Init → build → start → stop lifecycle | Yes |

### Free plugin test files

Located in the root `src/tests/` directory:

| File | Domain |
| --- | --- |
| `free_plugins_install_test.bats` | Install matrix for all 15 free plugins |
| `free_plugins_remove_test.bats` | Uninstall matrix for all 15 free plugins |

## Coverage Gaps (known)

| Gap | Notes |
| --- | --- |
| `nself cloud` | Cloud hosting commands — needs network mock |
| `nself billing` | Full Stripe E2E — requires test Stripe key |
| `nself docs` | Docs generation — low priority |
| Docker-in-Docker tests | Integration tests skip in CI without DinD |

## How to Run

```bash
# All bats tests (root dir)
cd src/tests
bats .

# Single file
bats backup_restore_test.bats

# With TAP output for CI
bats --tap .

# Without Docker-required tests (uses skip logic built into each file)
# Just run normally — Docker-required tests auto-skip when Docker is absent.
bats .
```

## CI Matrix

Tests run on 5 OS x 2 Bash version combinations via `.github/workflows/platform-matrix.yml`:

| OS | Bash |
| --- | --- |
| ubuntu-22.04 | 3.2 |
| ubuntu-22.04 | 5.1 |
| ubuntu-24.04 | 3.2 |
| ubuntu-24.04 | 5.1 |
| macos-13 | 3.2 (native) |
| macos-14 | 3.2 (native) |
| macos-15 | 3.2 (native) |
