# Error Codes Reference

Every user-facing error in the ɳSelf CLI includes a stable code, a plain-language explanation, and a suggested fix. This page is the catalog for all registered error codes.

---

## How errors appear in the CLI

An error that carries a code (a structured error, or a known failure such as Docker not running) prints a block on stderr:

```
Error: [E001] Docker not installed
  Why: The docker binary was not found in PATH.
  Fix: Install Docker: https://docs.docker.com/get-docker/
  Docs: https://nself.org/docs/reference/error-codes#e001
```

The `Why`, `Fix` and `Docs` lines appear only when they have content. Every other error still prints one line, `Error: <message>`, and exits 1.

The code in brackets (e.g., `[E001]`) is stable across CLI versions. You can search this page by code to find the cause and fix. The exit status is the **Exit** column of the code's row; see [[Exit-Codes]] for the classes.

> The coded block and the exit classes (2, 3, 4) are v1.5 behaviour. They are on from v1.5.0 and opt-in before that with `NSELF_V15=1`; see [[Compat-V15]]. Without it, errors print as `Error: <message>` and exit 1.

---

## Looking up an error

- Copy the bracketed code from the terminal output (e.g., `E102`).
- Search this page for that code (Ctrl+F or Cmd+F).
- The fix column gives you the fastest path to resolution.

---

## Docker (E001-E049)

These errors occur when the CLI cannot find or communicate with Docker.

| Code | Exit | Summary | Why | Fix |
|------|------|---------|-----|-----|
| <a id="e001"></a>E001 | 2 | Docker not installed | The `docker` binary was not found in `PATH`. | Install Docker: [https://docs.docker.com/get-docker/](https://docs.docker.com/get-docker/) |
| <a id="e002"></a>E002 | 2 | Docker daemon not running | The Docker daemon is not responding to commands. | Start Docker Desktop or run: `sudo systemctl start docker` |
| <a id="e003"></a>E003 | 2 | Docker Compose not available | `docker compose` v2 plugin is not installed. | Update Docker Desktop or install the compose plugin manually. |
| <a id="e004"></a>E004 | 2 | docker-compose.yml not found | No `docker-compose.yml` exists in the project directory. | Run `nself build` to generate the compose file. |
| <a id="e005"></a>E005 | 2 | Port conflict | A required port is already in use by another process. | Run `nself doctor` to identify the conflict, then stop the conflicting process or change the port in `.env`. |

---

## Config (E050-E099)

These errors occur during configuration validation or when reading `.env` files.

| Code | Exit | Summary | Why | Fix |
|------|------|---------|-----|-----|
| <a id="e050"></a>E050 | 1 | No .env file found | No `.env` or `.env.dev` file exists in the project directory. | Run `nself init` to generate a configuration file. |
| <a id="e051"></a>E051 | 1 | Invalid config key | The configuration key contains invalid characters. | Config keys must contain only A-Z, 0-9, and underscores. |
| <a id="e052"></a>E052 | 1 | Config validation failed | One or more configuration values are invalid. | Run `nself config validate` to see all issues, then fix the reported values. |
| <a id="e053"></a>E053 | 1 | Weak password detected | A password field does not meet minimum length or contains insecure patterns. | Use a strong, randomly generated password of at least 16 characters. |
| <a id="e054"></a>E054 | 1 | Invalid project name | Project name contains characters not allowed in Docker and DNS contexts. | Use only lowercase letters, numbers, and hyphens. Must start with a letter. |
| <a id="e055"></a>E055 | 1 | Duplicate route detected | Two or more services are configured with the same nginx route. | Check `ROUTE` values in `.env` and ensure each service has a unique route. |
| <a id="e056"></a>E056 | 1 | Unknown config key | The key is not recognized by nself. | Check for typos. Run `nself config list` to see all valid keys. |
| <a id="e057"></a>E057 | 1 | Insecure password pattern | A password matches a known insecure pattern such as a dictionary word, a repeated character or a default value. | Replace it with a randomly generated password of at least 16 characters. |
| <a id="e058"></a>E058 | 1 | CORS wildcard not allowed in production | The CORS origin list contains a wildcard while the environment is production. | List the exact origins in HASURA_GRAPHQL_CORS_DOMAIN, or use a development environment for wildcards. |
| <a id="e059"></a>E059 | 1 | Secret is empty or a placeholder | A required secret is empty or still holds a placeholder value from the template. | Generate a real value and set it in .env, then run the command again. |

---

## Plugin and License (E100-E149)

These errors occur when installing plugins or validating license keys.

| Code | Exit | Summary | Why | Fix |
|------|------|---------|-----|-----|
| <a id="e100"></a>E100 | 1 | Plugin not found | The requested plugin does not exist in the registry. | Run `nself plugin list` to see available plugins. |
| <a id="e101"></a>E101 | 3 | Invalid license key | The license key format is invalid. | License keys start with `nself_pro_` followed by 32+ characters. Check your key. |
| <a id="e102"></a>E102 | 3 | License tier insufficient | Your license tier does not include this plugin. | Upgrade your plan at [https://nself.org/pricing](https://nself.org/pricing) |
| <a id="e103"></a>E103 | 3 | License expired | The license key has expired. | Renew your license at [https://nself.org/account](https://nself.org/account) |
| <a id="e104"></a>E104 | 3 | License validation failed (network) | Cannot reach the license server and no valid local cache exists. | Check your internet connection. Previously validated licenses work offline for 7 days. |
| <a id="e105"></a>E105 | 1 | Circular plugin dependency | Plugin dependency graph contains a cycle. | Report this as a bug at [https://github.com/nself-org/cli/issues](https://github.com/nself-org/cli/issues) |
| <a id="e106"></a>E106 | 1 | Invalid plugin manifest | The plugin manifest is missing a required field or contains an invalid value. | Fix the manifest fields named in the message, or reinstall the plugin: nself add <plugin> |
| <a id="e107"></a>E107 | 1 | Plugin is not signed | A stable plugin release has no signature, so the install was refused. | Use a release that carries a signature, or report the missing signature to the plugin publisher. |
| <a id="e108"></a>E108 | 1 | Plugin checksum missing | A stable plugin release has no checksum, so the install was refused. | Use a release that carries a checksum, or report the missing checksum to the plugin publisher. |
| <a id="e109"></a>E109 | 1 | Duplicate plugin slug | The same plugin slug is served by more than one unrelated registry entry. | Report this to the registry maintainers. Install the plugin by its full registry name if one is shown. |
| <a id="e110"></a>E110 | 3 | License does not entitle this plugin tier | The installed license does not include the pro tier of this plugin. | Check your plan with: nself license status. Upgrade at https://nself.org/pricing if the tier is missing. |
| <a id="e111"></a>E111 | 1 | Forbidden key in a v2 plugin manifest | A manifest_version 2 plugin.json carries a key that belongs to the registry or to a legacy format (bundles, tier_pair, author_public_key, signature, checksum). | Remove the key from plugin.json. Bundle membership lives in bundles.json and signatures are added by the release pipeline. |
| <a id="e112"></a>E112 | 1 | Plugin manifest compatibility keys drifted | The generated compatibility keys (pluginType, binaryName, cliCommands, minNselfVersion, status, isCommercial, licenseType, requires_license, tier) no longer match the v2 fields they are derived from. | Regenerate them: go run github.com/nself-org/cli/tools/manifestv2migrate -in plugin.json -compat -write |
| <a id="e113"></a>E113 | 1 | Plugin command collides with a core command | commands.command is one of the core command verbs, so the plugin cannot be mounted under it. | Rename commands.command in plugin.json to a name that is not a core verb. |
| <a id="e114"></a>E114 | 1 | Unsupported plugin manifest version | plugin.json declares a manifest_version this nself release does not read (supported: 1 or absent, and 2). | Set manifest_version to 2, or upgrade nself if the manifest was written for a newer release. |
| <a id="e118"></a>E118 | 2 | Plugin migrations not applied in time | The bounded wait ended before the plugin's /health reported every migration applied: it reported fewer applied than expected, or it never answered HTTP 200 (hang, error status, redirect). The plugin applies its own SQL at boot; the CLI never applies it. | Read the plugin logs (docker logs nself_<plugin>) for the failing migration. A slow first boot needs a longer wait: set NSELF_PLUGIN_READY_TIMEOUT (seconds). |
| <a id="e119"></a>E119 | 2 | Plugin health lacks migrations status | The plugin declares migrations but its /health response has no valid "migrations" {applied, expected} field (absent, wrong type, negative, applied above expected, or a body over 1 MiB), so readiness cannot be proven. | Update the plugin to a release built on sdk/go/migrate, which serves the migrations field in /health. |
| <a id="e120"></a>E120 | 1 | Plugin ships migrations without boot apply | The plugin has a migrations/ directory but its manifest does not declare migrations.apply: boot, so nothing applies the SQL when the plugin starts. | Add "migrations": {"dir": "migrations", "apply": "boot"} to plugin.json and apply the files at boot with sdk/go/migrate. |

---

## SSL and Network (E150-E199)

These errors occur during SSL certificate generation or network operations.

| Code | Exit | Summary | Why | Fix |
|------|------|---------|-----|-----|
| <a id="e150"></a>E150 | 2 | mkcert not installed | `mkcert` is not installed; falling back to OpenSSL self-signed certs. | Install mkcert: `brew install mkcert` (macOS) or see [https://github.com/FiloSottile/mkcert](https://github.com/FiloSottile/mkcert) |
| <a id="e151"></a>E151 | 2 | SSL certificate generation failed | Could not generate SSL certificates for the configured domain. | Check domain configuration and ensure `openssl` is available. |

---

## Database, Backup and Recovery (E200-E249)

These errors occur when the CLI interacts with the PostgreSQL container, backups, restores and disaster recovery.

| Code | Exit | Summary | Why | Fix |
|------|------|---------|-----|-----|
| <a id="e200"></a>E200 | 2 | Database not running | PostgreSQL container is not running or not accepting connections. | Run `nself start` to start all services, or `nself doctor` to diagnose. |
| <a id="e201"></a>E201 | 2 | Migration failed | A database migration could not be applied. | Check the migration SQL for errors. Run `nself db migrate status` to see pending migrations. |
| <a id="e202"></a>E202 | 2 | Backup failed | Database backup operation failed. | Ensure sufficient disk space and that the database is running. |
| <a id="e203"></a>E203 | 1 | Migration validation failed | The migration dry run found an error in the migration SQL before anything was applied. | Fix the SQL named in the message and run the migration again. No changes were applied. |
| <a id="e204"></a>E204 | 2 | Migration prerequisite missing | An object the migration depends on does not exist in the live schema. | Apply the earlier migrations first: nself db migrate. Run nself doctor if the schema looks wrong. |
| <a id="e205"></a>E205 | 2 | Backup not found | The requested backup file or backup ID does not exist. | List available backups with: nself backup list |
| <a id="e206"></a>E206 | 2 | Backup verification failed | The backup failed its integrity check, so it may be corrupt or incomplete. | Create a fresh backup: nself backup create. Do not restore from the failed file. |
| <a id="e207"></a>E207 | 2 | Backup restore failed | The backup could not be restored into the database. | Check that the database is running and has enough disk space, then retry. Run nself doctor for details. |
| <a id="e208"></a>E208 | 2 | Backup encryption failed | The backup could not be encrypted. | Check the backup encryption key setting and that the key file is readable, then retry. |
| <a id="e209"></a>E209 | 2 | Backup decryption failed | The backup could not be decrypted with the configured key. | Check that the key matches the one used to create the backup, then retry. |
| <a id="e210"></a>E210 | 2 | Remote backup operation failed | Copying a backup to or from the remote destination failed. | Check the remote destination settings and network access, then retry. |
| <a id="e211"></a>E211 | 2 | Backup prune failed | Old backups could not be removed according to the retention policy. | Check file permissions on the backup directory and the remote destination, then retry. |
| <a id="e212"></a>E212 | 2 | WAL archive failed | PostgreSQL could not archive a write-ahead log segment. | Check free disk space and the archive destination. Run nself doctor for details. |
| <a id="e213"></a>E213 | 2 | Disaster recovery drill failed | The disaster recovery drill did not complete successfully. | Read the drill report for the failing step, fix it and run the drill again. |
| <a id="e214"></a>E214 | 2 | Standby promotion failed | The standby database could not be promoted to primary. | Check the standby status and replication lag, then retry the promotion. |
| <a id="e215"></a>E215 | 2 | Disaster recovery rollback failed | Rolling back a disaster recovery step failed. | Check the current database role on each node before taking any further action. |
| <a id="e216"></a>E216 | 2 | Split-brain fence failed | The old primary could not be fenced, so two nodes might accept writes. | Stop the old primary by hand before continuing, then retry the fence step. |
| <a id="e217"></a>E217 | 2 | Off-box backup is stale | The newest off-box backup heartbeat is older than --max-age, is missing, or reports a failed backup. | Check the backup schedule on the production host (nself backup schedule, then its systemd journal) and run nself backup stream once by hand. |
| <a id="e218"></a>E218 | 2 | Restore drill is stale or failed | The newest restore drill is older than --max-drill-age, never ran, or its restore did not match the backup. | Run nself backup drill --from <remote> --identity <age key> --heartbeat-to <remote> from the owner machine and read the mismatches it lists. |
| <a id="e219"></a>E219 | 2 | Backup heartbeat unreadable | The heartbeat object could not be fetched or parsed, so freshness is unknown. Unknown is treated as not OK. | Check the heartbeat remote and its credentials (--heartbeat-to or NSELF_BACKUP_HEARTBEAT_REMOTE), then read the object with rclone cat. |
| <a id="e220"></a>E220 | 2 | Restore drill cannot start | The drill needs a running Docker daemon, the age binary for encrypted backups, and free disk of twice the download size. | Start Docker, install age, or free disk space in the temp directory (TMPDIR), then run the drill again. |

---

## Health (E250-E299)

These errors occur during health checks run by `nself doctor`, `nself health`, `nself status` and startup probes.

| Code | Exit | Summary | Why | Fix |
|------|------|---------|-----|-----|
| <a id="e250"></a>E250 | 2 | Service unhealthy | A service health check returned an unhealthy status. | Run `nself doctor --verbose` for detailed diagnostics. |
| <a id="e251"></a>E251 | 2 | Health check timeout | The health check did not complete within the timeout period. | The service may be starting slowly. Wait and retry, or check logs with `nself logs`. |
| <a id="e252"></a>E252 | 2 | Service not found | The named service is not part of this project stack. | Run nself status to list the services in this project, and check the spelling. |

---

## Init and Project (E300-E349)

These errors occur during `nself init` or when the CLI validates the working directory.

| Code | Exit | Summary | Why | Fix |
|------|------|---------|-----|-----|
| <a id="e300"></a>E300 | 1 | Project already initialized | A `.env` file already exists in this directory. | Use `nself config set` to modify existing config, or delete `.env` to reinitialize. |
| <a id="e301"></a>E301 | 1 | Source directory detected | You are running nself inside the CLI source repository. | Change to your project directory first: `cd /path/to/your/project` |

---

## Domain (E350-E399)

These errors occur when the CLI validates domain names or port numbers from config.

| Code | Exit | Summary | Why | Fix |
|------|------|---------|-----|-----|
| <a id="e350"></a>E350 | 1 | Invalid domain name | The configured domain name is not valid. | Use a valid domain like `example.com` or `localhost`. |
| <a id="e351"></a>E351 | 1 | Invalid port number | Port number is outside the valid range (1-65535). | Use a port number between 1024 and 65535 for non-privileged ports. |

---

## CLI (E400-E449)

These errors come from the command line itself: usage, JSON support, destructive-action gates and moved commands.

| Code | Exit | Summary | Why | Fix |
|------|------|---------|-----|-----|
| <a id="e400"></a>E400 | classified | Unclassified error | The command failed with an error that has no specific code. | Read the message. If it looks like a bug, report it at https://github.com/nself-org/cli/issues |
| <a id="e401"></a>E401 | 1 | Invalid usage | A flag or argument was missing, unknown or had an invalid value. | Run the command with --help to see the accepted flags and arguments. |
| <a id="e402"></a>E402 | 1 | JSON output not supported | This command or flag combination cannot produce JSON output. | Run the command without --json, or use nself help --json to see which commands support it. |
| <a id="e403"></a>E403 | 4 | Destructive action blocked | A safety gate refused a destructive action. | Read the message for the missing confirmation or flag, check the target environment, then confirm explicitly. |
| <a id="e404"></a>E404 | 1 | Command moved to a plugin | This command now lives in a plugin that is not installed. | Run the nself add command printed in the message, then run the command again. |
| <a id="e410"></a>E410 | 1 | Command removed in v1.5 | This command was removed in nSelf v1.5 and has no replacement. | Read the message for what to use instead, or run nself --help to see the current commands. |
| <a id="e435"></a>E435 | 1 | nself.yaml has a type or syntax error | nself.yaml cannot be read as YAML, or a key holds a value of the wrong type (for example plugins as a number, or a plugin entry that is a map instead of a name). | Fix the value at the reported line so it matches schemas/nself-yaml.v1.schema.json (see [[nself-yaml]]). |
| <a id="e436"></a>E436 | 1 | Unknown key in nself.yaml | nself.yaml has a key that nself does not read. Silent keys hide typos and drift. | Rename the key to x-<key> if it is app metadata, or remove it. nself reads app, bundle, bundles and plugins only. |

---

## Reconcile and locks (E450-E479)

Change-plan (E450) and operation-lock (E460) codes. The other codes in this block are documented here when their Tickets register them.

| Code | Exit | Summary | Why | Fix |
|------|------|---------|-----|-----|
| <a id="e450"></a>E450 | 1 | Plan id does not match | The plan you confirmed no longer matches what this command would change, because an input changed or the plan id is not one this project produces. | Re-run nself build --plan and pass the new plan_id. |
| <a id="e451"></a>E451 | 1 | Plan id cannot bind generated secrets | This build generates secrets, and random values cannot be reproduced from a plan id. | Set the secrets in .env.secrets first, or run nself build and confirm at the prompt (or with --yes, without --plan-id). |
| <a id="e460"></a>E460 | 1 | Project operation lock held | Another nself command is changing this project and holds its operation lock. | Wait for the running nself command to finish, or stop it, then run this command again. |

---

## Adopt (E500-E529)

These errors come from adopting an existing stack: custom services that name dependencies, networks, build contexts and host binds. Only the codes registered so far are listed.

| Code | Exit | Summary | Why | Fix |
|------|------|---------|-----|-----|
| <a id="e500"></a>E500 | 1 | Custom service dependency is not valid | CS_N_DEPENDS_ON names a service that does not exist in this project, uses a condition other than started, healthy or completed, or forms a dependency cycle. | Use name[:started\|healthy\|completed], comma-separated, name a core service, another custom service or a service from an installed plugin, and remove circular dependencies. |
| <a id="e501"></a>E501 | 1 | Custom service network is not allowed | CS_N_NETWORKS names a network that does not start with <PROJECT_NAME>_. A custom service never joins another project's network. | Rename the network to <PROJECT_NAME>_<name> (lowercase letters, digits, - and _), or remove it from CS_N_NETWORKS. |
| <a id="e502"></a>E502 | 1 | Custom service host bind is not allowed | CS_N_VOLUMES binds a writable absolute host path outside the project, or the Docker socket, or the host root. These give the container control of the host. | Use a named volume or a project-relative path, mount an outside path read-only (:ro), and never mount /var/run/docker.sock or /. |
| <a id="e507"></a>E507 | 1 | Required Postgres extension unavailable | The plugin declares requires.postgres_extensions and the project's Postgres does not provide one: the running cluster is asked through pg_available_extensions, and when it is stopped the configured image decides. Nothing was downloaded or installed. | Run `nself db image switch --to pgvector` for vector, or set POSTGRES_IMAGE to an image that ships the extension. See [[pgvector]]. |
| <a id="e508"></a>E508 | 1 | Postgres extension availability unknown for a custom image (warning) | The postgres image is neither postgres:* nor pgvector/pgvector:* and the database is not running, so nself cannot tell what it ships. A warning, not a refusal: the install continues. | Start the stack so the cluster can be checked, or use an image that ships the extension. See [[pgvector]]. |
| <a id="e515"></a>E515 | 1 | Unknown portable bundle format or major version | The bundle manifest is missing, is not JSON, is not an nself portable export, or its schema_version is not major 1. | Re-export the bundle with a matching nself release, or upgrade nself if the bundle comes from a newer one. |
| <a id="e516"></a>E516 | 1 | Portable bundle failed its integrity check | A bundle file is missing, changed, truncated, longer than listed, unlisted, duplicated, a link, or has an unsafe name, or a size limit was exceeded. The message names the file. | Export the bundle again from the source and keep it unmodified, then re-run the command. See [[Import-Export-Format]]. |
| <a id="e528"></a>E528 | 1 | Custom service build context is not allowed | CS_N_PATH reaches above the repository root, or an ancestor build context has no ignore file that excludes **/.env* and **/.secrets (a bare .env* matches only the context root). The whole directory is sent to the image builder. | Point CS_N_PATH at the project or at an ancestor no higher than the directory that holds .git, and add **/.env* and **/.secrets to that directory's .dockerignore. |

See [[Config-Custom-Services]] for the keys and [[Import-Export-Format]] for the bundle codes.

---

## JSON error object

With `--json` (v1.5 mode), a failure writes exactly one error envelope to stdout and the human block above still goes to stderr:

```json
{
  "schema_version": "1",
  "command": "start",
  "error": {
    "code": "E002",
    "message": "docker info failed: docker daemon is not running",
    "cause": "The Docker daemon is not responding to commands.",
    "remediation": "Start Docker Desktop or run: sudo systemctl start docker",
    "docs_url": "https://nself.org/docs/reference/error-codes#e002",
    "exit_code": 2,
    "class": "infra"
  }
}
```

| Field | Rule |
|-------|------|
| `code` | `^E[0-9]{3}$`. The error's own code; else the code of the matching known failure (when several match, the highest class wins: auth, then destructive, then infra, then user); else `E400`. |
| `message` | What happened. |
| `cause` | Omitted when empty. The `Why` line. |
| `remediation` | Omitted when empty. The `Fix` line. |
| `docs_url` | Omitted when empty. The `Docs` line. |
| `exit_code` | The process exit status. |
| `class` | `user` (1), `infra` (2), `auth` (3), `destructive_blocked` (4), `other` (any other status). |

`command` is the command path without `nself ` (`""` when the failure came before a command was resolved). Message, cause and remediation are redacted: URL credentials, tokens, e-mail addresses and IPs are masked, and raw arguments are never included. The schema is `schemas/error.v1.schema.json`.

---

## Code allocation

Codes are grouped in blocks. A block has exactly one category, so the category of a code follows from its number. A new block or range needs an entry in the plan first; the registry (`internal/errs`) rejects a code outside its owner's range, a mismatched category, and any code in the reserved range.

| Block | Category | Use |
|-------|----------|-----|
| E001-E049 | docker | Docker and Compose |
| E050-E099 | config | Configuration and secrets |
| E100-E149 | plugin | Plugins, licenses, entitlements |
| E150-E199 | ssl | Certificates |
| E200-E249 | database | Database, migrations, backups, recovery |
| E250-E299 | health | Service health |
| E300-E349 | init | Project setup |
| E350-E399 | domain | Domains and ports |
| E400-E449 | cli | Usage, JSON output, command moves |
| E450-E479 | reconcile | Reconcile, locks, ACME |
| E480-E499 | deploy | Deploy and release verification |
| E500-E529 | adopt | Adopting existing stacks |
| E540-E549 | secret | Secret resolution |
| E600-E719 | reserved | The ci plugin's own registry; never allocated in the CLI |

Codes are never reused or renumbered. Several blocks have no registered codes yet; a block appears in the tables above once it has one.

---

## Error codes in scripts

If you are scripting around the CLI, branch on the exit status (see [[Exit-Codes]]) and parse the bracketed code from stderr, or use `--json` and read `error.code` from stdout:

```bash
output=$(nself start 2>&1)
if [ $? -ne 0 ]; then
  code=$(printf '%s' "$output" | grep -oE '\[E[0-9]+\]' | head -1)
  printf 'nself failed with %s\n' "$code"
fi
```

---

## Related pages

- [[Exit-Codes]], exit status classes and what changes in v1.5.0
- [[cmd-doctor]], run diagnostics to identify and resolve common errors
- [[cmd-config]], view and edit configuration values
- [[Plugin-Licensing]], license key format and tier details
- [[Guide-SSL-Setup]], SSL certificate setup
- [[Config-Env-Vars]], all environment variable reference

---

← [[Home]] | [[_Sidebar]]
