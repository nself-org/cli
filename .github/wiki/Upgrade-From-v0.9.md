# Upgrade From v0.9

> Step-by-step guide for upgrading an ɳSelf v0.9.x project to v1.0.9.

## Contents

- [Before you start](#before-you-start)
- [Step 1: Upgrade the CLI](#step-1-upgrade-the-cli)
- [Step 2: Run the migration](#step-2-run-the-migration)
- [Step 3: Re-install plugins](#step-3-re-install-plugins)
- [Step 4: Verify the stack](#step-4-verify-the-stack)
- [Command changes](#command-changes)
- [Environment file changes](#environment-file-changes)
- [Plugin changes](#plugin-changes)
- [Troubleshooting](#troubleshooting)
- [Rollback](#rollback)

---

## Before you start

**What changed in v1.0.0:**

- The CLI was rewritten from Bash to Go. It is now a single binary.
- Plugin format changed from shell scripts to signed Go bundles. All v0.9 plugins must be re-installed.
- The environment file layout changed from a single `.env` to a cascade: `.env.dev` → `.env.local` → `.env.staging`/`.env.prod` → `.env.secrets` → `.env.computed`.
- The nginx config moved from `nginx/` (flat) to `nginx/sites/` (structured).
- `.nself/config` changed from a plain file to a directory.
- `nself.sh` (the v0.9 bootstrap script) is no longer used.

**What `nself migrate` does automatically:**

1. Stops running containers.
2. Backs up your current project state to `.nself/backup/{timestamp}/`.
3. Moves nginx configs to `nginx/sites/`.
4. Regenerates `docker-compose.yml`, nginx, and SSL certificates for v1.

**What you must do manually after migration:**

- Re-enter your license key: `nself license set <key>`.
- Re-install each plugin: `nself plugin install <name>`.

---

## Step 1: Upgrade the CLI

### macOS (Homebrew)

```bash
brew upgrade nself-org/nself/nself
nself --version
# Expected: nself vX.Y.Z (darwin/arm64), the latest release
```

### Linux

```bash
curl -fsSL https://install.nself.org | bash
nself --version
```

### Verify

```bash
nself --version
# Must show: nself v1.0.9 or later
```

---

## Step 2: Run the migration

Navigate to your v0.9 project directory and run:

```bash
cd ~/my-nself-project

# First, scan to see what will be migrated
nself migrate detect

# Example output:
# Found 3 v0.9 artifact(s):
#   docker-compose.yml — v1-generated compose file
#   .env              — v1 environment file (NSELF_VERSION=0.x)
#   nginx/            — v1 nginx directory structure (missing nginx/sites/)
# Run: nself migrate run

# Run the full migration
nself migrate run
```

The migration is **idempotent**: running it on an already-migrated project exits cleanly with no changes.

**Verify the migration completed:**

```bash
# Should show: No v1 artifacts detected. This project is ready for nSelf v2.
nself migrate detect

# Check backup was created
ls .nself/backup/
```

---

## Step 3: Re-install plugins

Plugin signatures changed in v1.0.0. Every v0.9 plugin must be re-installed.

```bash
# Re-enter your license key
nself license set nself_pro_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx

# Re-install plugins (example — use your actual plugin list)
nself plugin install ai mux claw voice browser google notify cron

# Verify plugins are installed
nself plugin list
```

To find your original plugin list, check the v0.9 `.env` backup:

```bash
grep "PLUGIN_\|NSELF_PLUGIN" .nself/backup/*/  .env 2>/dev/null | grep "=true"
```

---

## Step 4: Verify the stack

```bash
# Build the v1 configuration
nself build

# Start the stack
nself start

# Run diagnostics
nself doctor
```

---

## Command changes

Commands that changed from v0.9 to v1.0.9:

| v0.9 command | v1.0.9 equivalent | Notes |
|---|---|---|
| `./nself.sh start` | `nself start` | Go binary, no bootstrap script |
| `./nself.sh stop` | `nself stop` | |
| `./nself.sh status` | `nself status` | |
| `./nself.sh build` | `nself build` | |
| `./nself.sh logs` | `nself logs` | |
| `./nself.sh update` | `nself update` | |
| `./nself.sh doctor` | `nself doctor` | |
| `./nself.sh plugin install <name>` | `nself plugin install <name>` | New signed format |
| `./nself.sh plugin list` | `nself plugin list` | |
| `./nself.sh plugin remove <name>` | `nself plugin remove <name>` | |
| `./nself.sh license set <key>` | `nself license set <key>` | |
| `./nself.sh license info` | `nself license info` | |
| `./nself.sh migrate` | `nself migrate` | New in v1.0.0 |
| `./nself.sh backup` | `nself backup` | |
| `./nself.sh restore` | `nself restore` | |
| `./nself.sh db migrate` | `nself db migrate` | |
| `./nself.sh db reset` | `nself db reset` | |
| `./nself.sh ssl renew` | `nself ssl renew` | |
| `./nself.sh config get <key>` | `nself config get <key>` | |
| `./nself.sh config set <key> <val>` | `nself config set <key> <val>` | |
| `./nself.sh env show` | `nself env show` | |
| `./nself.sh admin start` | `nself admin start` | |
| `nself up` | `nself start` or `nself up` | `up` is now an alias |

---

## Environment file changes

**v0.9:** Single `.env` file with all configuration including secrets.

**v1.0.9:** Cascade of env files, each for a specific purpose:

```
.env.dev        ← Team-shared defaults (tracked in git)
.env.local      ← Your machine-specific overrides (gitignored)
.env.staging    ← Staging overrides (tracked)
.env.prod       ← Production overrides (tracked, no real secrets)
.env.secrets    ← Real secrets — API keys, passwords (gitignored)
.env.computed   ← Generated by nself build (never edit manually)
```

**Migration handles this automatically.** After running `nself migrate run`, your old `.env` is backed up and `nself build` generates a compatible v1 configuration.

**v0.9 env variables that changed:**

| v0.9 variable | v1.0.9 variable | Notes |
|---|---|---|
| `NSELF_VERSION=0.9.x` | `NSELF_VERSION=1.0.9` | Updated by migrate |
| `NGINX_CONF_DIR=nginx/` | `NGINX_CONF_DIR=nginx/sites/` | Updated by migrate |
| `PLUGIN_<NAME>=true` | `NSELF_PLUGIN_<NAME>=true` | Prefix changed |

---

## Plugin changes

All v0.9 plugins used shell scripts. v1.0.9 uses signed Go bundles. Plugins are incompatible across versions.

**Plugin names that changed:**

| v0.9 plugin name | v1.0.9 plugin name |
|---|---|
| `nself-ai` | `ai` |
| `nself-claw` | `claw` |
| `nself-mux` | `mux` |
| `nself-voice` | `voice` |
| `nself-browser` | `browser` |
| `nself-google` | `google` |
| `nself-notify` | `notify` |
| `nself-cron` | `cron` |
| `nself-chat` | `chat` |
| `nself-livekit` | `livekit` |
| `nself-recording` | `recording` |
| `nself-moderation` | `moderation` |
| `nself-bots` | `bots` |
| `nself-realtime` | `realtime` |
| `nself-media-processing` | `media-processing` |
| `nself-streaming` | `streaming` |
| `nself-epg` | `epg` |
| `nself-tmdb` | `tmdb` |
| `nself-podcast` | `podcast` |
| `nself-social` | `social` |

After migration, the CLI prints a warning showing which plugins were enabled in v0.9 and the exact commands to re-install them.

---

## Troubleshooting

### Docker version too old

```
Error: Docker 24 or later is required (found: 20.10.x)
```

Upgrade Docker:

```bash
# macOS
brew upgrade docker

# Ubuntu/Debian
sudo apt-get update && sudo apt-get install docker-ce
```

### Port conflicts

```
Error: port 5432 is already in use by process: postgresql (pid 1234)
```

Stop the conflicting service:

```bash
# macOS: stop local postgres
brew services stop postgresql

# Linux: stop local postgres
sudo systemctl stop postgresql
```

### License key rejected after migration

Your license key format is valid but the key must be re-entered after migration because the v0.9 key storage location changed:

```bash
# Re-enter the key explicitly
nself license set nself_pro_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx

# Verify acceptance
nself license info
```

### Plugin name not found

v0.9 plugin names used the `nself-` prefix; v1.0.9 uses short names:

```bash
# v0.9 (wrong)
nself plugin install nself-ai

# v1.0.9 (correct)
nself plugin install ai
```

See the plugin name table above for the full mapping.

### Migration fails midway

If `nself migrate run` fails partway through:

```bash
# Check what was backed up
nself migrate rollback --list

# Restore the last backup
nself migrate rollback --yes
```

The rollback restores all files that were backed up before the failure.

### nself doctor reports issues after migration

Run with `--full` to get detailed diagnostics:

```bash
nself doctor --full

# Check for v0.9 global artifacts (stale paths)
nself doctor --check-legacy
```

---

## Rollback

If you need to revert to v0.9 after migrating:

```bash
# List available backups
nself migrate rollback --list

# Restore the most recent backup
nself migrate rollback --yes

# Or restore a specific backup by timestamp
nself migrate rollback --backup 20260420-143022 --yes
```

`--yes` confirms the restore without a prompt. From v1.5 a rollback without `--yes` asks on a terminal and is refused with E403 anywhere else (scripts, CI); in v1.4 it still runs but prints a deprecation warning (under `--json`, in `meta.deprecations`).

After rollback, reinstall the v0.9 CLI (use the Homebrew formula version pin or download from GitHub releases).

---

## Automated testing

The migration path is covered by a CI fixture that runs on every PR:
see `cli/internal/migration/testdata/v0.9-fixture/` and `.github/workflows/migration-fixture.yml`.
The fixture plants all 5 v0.9 artifact types and verifies the post-migration state matches
the expected output in `testdata/v0.9-fixture-expected/`.

---

See also:

- [[cmd-migrate]], full `nself migrate` command reference
- [[Installation]], fresh install guide
- [[Plugin-Install]], plugin management reference

← [[Commands]] | [[Home]] →
