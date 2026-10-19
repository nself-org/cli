# Drills

A drill is a script that proves something about nSelf by doing it for real: install on a clean host, upgrade across a version, run an importer against a fixture. Drills are not a separate harness. They are jobs of the `drills` pipeline in `.nself/ci.yaml`, run by `nself ci` like any other job, on a schedule or on demand.

## The pipeline

`.nself/ci.yaml` is committed (the rest of `.nself/` stays ignored, see `.gitignore`):

```yaml
pipelines:
  drills:
    triggers: [schedule, manual]
    schedule: "30 2 * * *"
    max_age: 36h
    drills_dir: scripts/drill/legs
```

Every directory of `scripts/drill/legs/` that holds an executable file called `run` is one job of kind `drill`, named `drill-<directory>`. The file's exit code is the result: 0 passes, anything else fails. The working directory is the repository root.

| Drill | Directory | What it does |
|---|---|---|
| `drill-two-command` | `scripts/drill/legs/two-command/` | Runs the [Two-Command Drill](Two-Command-Drill) (`scripts/drill/two-command.sh --host`) once with `NSELF_V15` unset and once with `NSELF_V15=1`, and exits with that script's code. The drill, its legs and its budgets are not changed by the wrapper. |
| `drill-clean-host` | `scripts/drill/legs/clean-host/` | The lifecycle of a new user: install, `init`, `start`, `add` a plugin, `backup create`, `backup restore`, with row equality on a seeded table. See [Clean-host lifecycle](#clean-host-lifecycle). |
| `drill-upgrade-continuity` | `scripts/drill/legs/upgrade-continuity/` | A stack started by the latest released cli (N) is upgraded by `nself update stack` of the build from the ref (N+1) with the seeded rows unchanged. See [Upgrade continuity](#upgrade-continuity). |

## Clean-host lifecycle

`scripts/drill/legs/clean-host/run` is the path a new user takes, on a clean directory and an empty `HOME`. It prints one `ci.evidence/v1` document (one check per step) as its last stdout line and saves it as `evidence.json`. The first failed step marks the later steps `skip`.

| Step | What it proves |
|---|---|
| `install` | The cli under test (`NSELF_BIN`, or `.nself/bin/nself` built from the checkout) is copied into an empty bin directory that leads `PATH`, and `nself version` answers. |
| `init` | `nself init` in an empty directory with stdin closed: no prompt. |
| `start` | `nself start`, then `nself status` exit 0 and a GraphQL answer from Hasura. |
| `add-plugin` | `nself add feature-flags` installs the plugin bytes that plugins.nself.org serves (the cli checks the checksum), the plugin is listed `[installed]` and its `plugin.json` is under the throw-away `HOME`. Change the plugin with `DRILL_PLUGIN`. |
| `seed` | A Hasura-layout migration creates `public.drill_seed` with 50 rows and `nself db migrate up` applies it. The row count and an md5 of every row are recorded. |
| `backup` | `nself backup create --tag drill`, `nself backup verify latest`, and the backup is in `nself backup list`. |
| `restore` | The table is dropped (and checked gone), then `nself backup restore latest --yes` runs. |
| `rows-equal` | The restored table has the count and md5 recorded at `seed`. |

The plugin is installed, not started: building a plugin image is the job of the plugin checks, and this drill proves the install path and that the data survives a restore with a plugin present.

## Upgrade continuity

`scripts/drill/legs/upgrade-continuity/run` upgrades a running stack across versions. Version N is the latest GitHub release (downloaded from the `/releases/latest/download/checksums.txt` redirect and checked against its sha256, so it needs no API call or token; set `DRILL_PREV_BIN` to use a local binary). Version N+1 is the build from the ref. When the ref and the latest release are the same version the drill still runs and says so in the `fetch-n` excerpt.

| Step | What it proves |
|---|---|
| `fetch-n` | The release binary for this OS and CPU is downloaded and verified. Both versions are recorded in `tool_versions`. |
| `init-n`, `start-n` | N creates and starts a healthy project (no prompt, `status` exit 0, GraphQL answers). |
| `seed` | As in the clean-host drill: 50 rows through a migration, count and md5 recorded. |
| `upgrade` | N+1 runs `nself update stack --yes --json` in the project; the report status is `complete` (backup, pull, build, start, migrate, readiness and health all `ok`). |
| `healthy` | After the upgrade `nself status` exits 0 and Hasura answers again. |
| `rows-identical` | The seeded table has the count and md5 recorded before the upgrade. |

The report of `update stack` is kept as `update-stack.json` next to the step logs. Consecutive nights of green are counted by the ship drill (P7-SHIP-03).

Both drills stop the stack with `nself stop --volumes` (and `docker compose down -v`) on every exit path, and run with a throw-away `HOME`. They need Docker with the compose plugin, `jq` and `timeout`; they print the reason and exit 2 when one is missing. Results land in `.nself/drill-out/<leg>/` (`<step>.log`, `evidence.json`).

### One stack at a time

The stack's compose file publishes fixed host ports (5432, 8080, 4000, 80, 443) and `nself-ci` starts every drill job at once. The legs that start a stack (`two-command`, `clean-host`, `upgrade-continuity`) therefore take one host-wide lock, `scripts/drill/stack-lock.sh` (a directory under `.nself/`; a lock whose holder process is gone is taken over). They run one after another; a waiting leg's job timeout keeps running, so the three together need less than `NSELF_CI_TIMEOUT` (80 minutes in the nightly workflow, 120 for the job). A new leg that starts a stack sources the same file. `DRILL_LOCK_TIMEOUT` (default 3600 seconds) bounds the wait.

Each leg takes a few minutes on top of the image pulls. A run of the funnel without `NSELF_CI_TIMEOUT` needs it raised to at least 20 minutes per leg.

## Add a drill

1. Create `scripts/drill/legs/<name>/`. The name starts with a letter or digit and uses only letters, digits, `.`, `_` and `-`.
2. Put an executable `run` in it (`chmod +x`, committed with the execute bit). Any language works; a shebang line picks it.
3. Commit. The next run has a job `drill-<name>`. You do not edit `.nself/ci.yaml` or the workflow.

A directory without a `run`, a `run` that is not executable, a symlink and loose files are ignored. A directory name that breaks the rule fails the pipeline with E640 rather than being skipped.

The job gets the default 10-minute timeout and only the allowlisted environment (`PATH`, `HOME`, `TMPDIR`, `CI`, the Go and Cargo variables, `LC_*`). `NSELF_BIN` and `NSELF_V15` are not passed, so a leg that needs the cli takes it from `.nself/bin/nself` (built from the checkout when missing) and sets what it needs itself, as `two-command/run` does. The nightly workflow raises the timeout to 80 minutes through `NSELF_CI_TIMEOUT`.

A drill with several steps may print one `ci.evidence/v1` document as the last compact JSON line of its output; its failing checks are listed in the job's check.

## Run it

```
nself ci --pipeline drills              # all drills, in name order
nself ci --pipeline drills --json       # one check per drill in the evidence
nself ci evidence --pipeline drills     # overdue report (exit 10 when overdue)
```

The first two need Docker and take about as long as the Two-Command Drill (tens of minutes). Results of the two-command leg land in `.nself/drill-out/<mode>/` (`drill.json`, `perfbench.json`, step logs).

## The nightly workflow

`.github/workflows/drill-nightly.yml` runs the pipeline on a GitHub-hosted `ubuntu-latest` runner at 02:30 UTC, on `workflow_dispatch`, and on pull requests that touch `scripts/drill/legs/**`, `.nself/ci.yaml` or the workflow. It:

1. builds `nself` from the ref (`.nself/bin/nself`);
2. installs the `nself-ci` release pinned in the workflow `env` (tag and linux-amd64 sha256, checked with `sha256sum --check --strict`; the binary is never built from the checkout);
3. runs `nself-ci --no-status --pipeline drills --json`;
4. uploads `evidence.json` as the artifact `drills-evidence` and `.nself/drill-out/` (the results of every leg, not only two-command) as `drills-two-command-results`.

The job is named `drills` and allows 120 minutes. It has no secrets and a read-only token.

## Read overdue

A drill job is overdue when no passing, commit-bound run newer than `max_age` (36 hours) exists in the machine's run store. A throwaway runner keeps no store, so on GitHub a red run is the alert and a missing run (a schedule that stopped firing) is the staleness signal: check the Actions tab for `drill-nightly`. A runner-availability failure (image pull, runner start) is re-run, never counted as a pass. On a machine that keeps its run store, `nself ci evidence --pipeline drills` exits 10 and names the overdue drills.

To move the pinned `nself-ci`, change `CI_VERSION`, `CI_RELEASE_URL` and `CI_RELEASE_SHA256` together in the workflow.
