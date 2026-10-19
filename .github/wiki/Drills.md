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
| `drill-importers` | `scripts/drill/legs/importers/` | Reruns the six importer e2e scripts and the export round trip, one fixture at a time, and prints one evidence check per source. See [Importers Drill](Drill-Importers). |

## Add a drill

1. Create `scripts/drill/legs/<name>/`. The name starts with a letter or digit and uses only letters, digits, `.`, `_` and `-`.
2. Put an executable `run` in it (`chmod +x`, committed with the execute bit). Any language works; a shebang line picks it.
3. Commit. The next run has a job `drill-<name>`. You do not edit `.nself/ci.yaml` or the workflow.

A directory without a `run`, a `run` that is not executable, a symlink and loose files are ignored. A directory name that breaks the rule fails the pipeline with E640 rather than being skipped.

The job gets the default 10-minute timeout and only the allowlisted environment (`PATH`, `HOME`, `TMPDIR`, `CI`, the Go and Cargo variables, `LC_*`). `NSELF_BIN` and `NSELF_V15` are not passed, so a leg that needs the cli takes it from `.nself/bin/nself` (built from the checkout when missing) and sets what it needs itself, as `two-command/run` does. The nightly workflow raises it to 80 minutes with `--timeout 80m` (the `NSELF_CI_TIMEOUT` variable alone does not reach discovered drill jobs); a drill can also define its own `drill-<name>` job in `.nself/ci.yaml`.

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
4. uploads `evidence.json` as the artifact `drills-evidence` and the leg results as `drills-two-command-results`.

The job is named `drills` and allows 120 minutes. It has no secrets and a read-only token.

## Read overdue

A drill job is overdue when no passing, commit-bound run newer than `max_age` (36 hours) exists in the machine's run store. A throwaway runner keeps no store, so on GitHub a red run is the alert and a missing run (a schedule that stopped firing) is the staleness signal: check the Actions tab for `drill-nightly`. A runner-availability failure (image pull, runner start) is re-run, never counted as a pass. On a machine that keeps its run store, `nself ci evidence --pipeline drills` exits 10 and names the overdue drills.

To move the pinned `nself-ci`, change `CI_VERSION`, `CI_RELEASE_URL` and `CI_RELEASE_SHA256` together in the workflow.
