# Configuration Precedence

Where a configuration key gets its value, which source wins, and which command flags supply a key.

## Contents

- [Sources and precedence](#sources-and-precedence)
- [Asking which source wins](#asking-which-source-wins)
- [Flags that supply a key](#flags-that-supply-a-key)
- [What nself.yaml is not](#what-nselfyaml-is-not)

## Sources and precedence

The loader reads the cascade files for the active environment, lowest precedence first, and each file that sets a key replaces the value set before it:

1. `.env`
2. `.env.dev`, `.env.staging`, `.env.prod` or `.env.<name>` (exactly one, matching `ENV`; `local` reads `.env.dev`)
3. `.env.secrets`
4. `.env.local`

The process environment is the lowest source in practice: a file in the cascade replaces a value that the process environment carries, so the process environment wins only when no cascade file sets the key. A key that nothing sets falls back to its documented default. `NSELF_LEGACY_ENV_ORDER=1` restores the previous order for one minor version (see [[Configuration]]).

## Asking which source wins

```bash
nself config explain BASE_DOMAIN          # by key
nself config explain init --domain        # by command flag; resolves to the key it supplies
nself config explain BASE_DOMAIN --reveal # show values (redacted by default)
nself config explain BASE_DOMAIN --json   # one envelope
```

The answer lists the env name, the default, the flags that supply the key, the MCP tool and HTTP route parameter that carry each flag, the `nself.yaml` row, every cascade file that sets the key and the winner. A key that is not a known configuration key, and a flag that supplies no key, exit 1 with `E434`. `nself config env explain VAR` walks the same cascade and prints the same winner.

## Flags that supply a key

Each flag with a configuration meaning is bound to its key once, in `internal/config/bindings/flags.yaml`. A flag whose upper-snake name is a known key and which is not bound is listed with a reason in `exempt.yaml`; a test fails for any flag that is neither. The command registry reports the bound key as `env` on the flag (`nself help --json`).

<!-- BEGIN GENERATED: config-bindings (internal/config/bindings; do not hand edit) -->

### Bound flags

| Command | Flag | Key | Default |
|---|---|---|---|
| `nself build` | `--debug` | `DEBUG` | none |
| `nself config` | `--env` | `ENV` | dev |
| `nself config secrets` | `--env` | `ENV` | dev |
| `nself config services` | `--env` | `ENV` | dev |
| `nself db seed run` | `--env` | `ENV` | dev |
| `nself init` | `--domain` | `BASE_DOMAIN` | local.nself.org |
| `nself start` | `--skip-health-checks` | `NSELF_SKIP_HEALTH_CHECKS` | false |
| `nself status health` | `--env` | `ENV` | dev |
| `nself status urls` | `--env` | `ENV` | dev |

### Flags that look like a key and are not bound

| Command | Flag | Reason |
|---|---|---|
| `nself` (every command) | `--no-monorepo` | NSELF_NO_MONOREPO is listed as a known key but no code reads it; the flag is the only control |
| `nself backup list` | `--env` | filters backup rows by environment label; it does not select the local ENV cascade |
| `nself build` | `--no-monorepo` | NSELF_NO_MONOREPO is listed as a known key but no code reads it; the flag is the only control |
| `nself config plugins dev` | `--debug` | attaches the dlv debugger; unrelated to the DEBUG key |
| `nself db drift` | `--env` | carries a project directory (usage text: Project directory to read hasura/metadata/**), not an environment name |
| `nself db generate` | `--env` | names the target environment of the generated output (local, staging or prod), not the local ENV cascade |
| `nself db hasura apply-ref` | `--env` | selects a deploy inventory target (NSELF_DEPLOY_HOST_<ENV> or control-plane.yaml), not the local ENV cascade |
| `nself db hasura metadata apply` | `--env` | selects a deploy inventory target (NSELF_DEPLOY_HOST_<ENV> or control-plane.yaml), not the local ENV cascade |
| `nself db hasura sync` | `--env` | selects a deploy inventory target (NSELF_DEPLOY_HOST_<ENV> or control-plane.yaml), not the local ENV cascade |
| `nself db import firebase` | `--project-name` | names generated SQL files; it is not the PROJECT_NAME of the project |
| `nself db migrate status` | `--env` | selects a deploy inventory target (NSELF_DEPLOY_HOST_<ENV> or control-plane.yaml), not the local ENV cascade |
| `nself db migrate up` | `--env` | selects a deploy inventory target (NSELF_DEPLOY_HOST_<ENV> or control-plane.yaml), not the local ENV cascade |
| `nself db reconcile` | `--env` | carries a project directory (usage text: Project directory to read hasura/metadata/**), not an environment name |
| `nself deploy` | `--env` | selects the deploy inventory environment (staging, prod, qa), not the local ENV cascade |
| `nself deploy access grant` | `--env` | selects hosts in a deploy inventory environment, not the local ENV cascade |
| `nself deploy access list` | `--env` | selects hosts in a deploy inventory environment, not the local ENV cascade |
| `nself deploy access revoke` | `--env` | selects hosts in a deploy inventory environment, not the local ENV cascade |
| `nself deploy status` | `--env` | selects the deploy inventory environment to check, not the local ENV cascade |
| `nself exec` | `--env` | repeatable KEY=VALUE pairs for the child process, not the ENV key |
| `nself functions deploy` | `--env` | repeatable KEY=VALUE pairs for the function, not the ENV key |
| `nself logs` | `--no-color` | boolean inverse of NO_COLOR, which is presence based and read by internal/ui and internal/output; the flag does not set it |
| `nself reset` | `--no-monorepo` | NSELF_NO_MONOREPO is listed as a known key but no code reads it; the flag is the only control |
| `nself start` | `--debug` | shows debug output for this run only; it does not set DEBUG (build --debug does) |
| `nself stop` | `--no-monorepo` | NSELF_NO_MONOREPO is listed as a known key but no code reads it; the flag is the only control |

<!-- END GENERATED: config-bindings -->

## What nself.yaml is not

`nself.yaml` is not a configuration source. It carries `app`, `bundle`, `bundles` and `plugins`; it holds no configuration keys, so no key is read from it and `nself config explain` reports it as "not a source".
