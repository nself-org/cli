# Two-Command Drill

The promise: `nself init && nself start` gives a working backend on a clean Linux host, with no questions asked. The drill proves it every night and on every change to its own files.

## What it proves

In an empty temp directory, with `CI`, `GITHUB_ACTIONS` and `NSELF_NONINTERACTIVE` unset for the drilled commands:

| Leg | How the commands run | Pass means |
|---|---|---|
| A | stdin `/dev/null`, output to a file (no TTY) | `init` (120 s) and `start` (600 s) exit 0 |
| B | a pseudo-TTY (`script -qec`) that never receives input | `init` and `start` finish inside the same timeouts and exit 0 |

A prompt cannot be answered in either leg. In leg A it hits end of file and fails. In leg B it blocks until the timeout kills it, and the drill reports `prompt_detected: true`.

Health is mode agnostic: poll `nself status` until it exits 0 (within 600 s), then send one `{ __typename }` query to `http://localhost:8080/v1/graphql` with the project's admin secret. Leg B always runs after leg A's stack is stopped (`nself stop`, then `docker compose down -v --remove-orphans` in the drill directory).

## Time to healthy

Seconds from the start of `nself init` (leg A) to the first healthy `nself status`. It is written as a `perfbench/v1` result and is informational. It never gates a merge or a release.

## Workflow

`.github/workflows/two-command-drill.yml` runs on GitHub-hosted `ubuntu-latest` (no secrets, `contents: read`, actions pinned by SHA):

- nightly at 03:17 UTC, on `workflow_dispatch`, and on pull requests that touch `scripts/drill/**` or the workflow;
- a matrix of two legs: `NSELF_V15` unset (v1.4 mode) and `NSELF_V15=1` (v1.5 mode);
- it builds `nself` from the ref, runs the drill, and uploads one artifact per matrix leg.

A failure caused by runner availability (image pulls, runner start) is re-run, not counted.

## Artifacts

| File | Content |
|---|---|
| `drill.json` | `{schema: "two-command-drill/v1", mode, time_to_healthy_s, graphql_ok, prompt_detected, leg_a, leg_b}` |
| `perfbench.json` | `perfbench/v1`, scenario `two-command-drill`, one metric `time_to_healthy` in `s`, runs 1 |
| `*.log` | output of each leg's `init` and `start`, and of the cleanup |

If the stack never got healthy, `time_to_healthy_s` is `null` and no `perfbench.json` is written.

## Run it locally

On a Linux host with Docker, `jq` and coreutils `timeout`:

```bash
go build -mod=vendor -o /tmp/nself ./cmd/nself
NSELF_BIN=/tmp/nself bash scripts/drill/two-command.sh --host                # v1.4 mode
NSELF_V15=1 NSELF_BIN=/tmp/nself bash scripts/drill/two-command.sh --host    # v1.5 mode
```

Results go to `./drill-out` (override with `DRILL_OUT`). The drill starts the stack on ports 80, 443, 4000, 5432 and 8080 and removes it afterwards, so stop other stacks first. Never run it against a server.

Check the detector itself, with no Docker:

```bash
bash scripts/drill/two-command.sh --self-test
```

It runs `scripts/drill/fixtures/prompt.sh` (a command that does `read -r x`) and must report a prompt, runs `fixtures/ok.sh` and must not, and validates a synthetic `drill.json` and `perfbench.json` with `jq`.

Tunables: `DRILL_INIT_TIMEOUT` (120), `DRILL_START_TIMEOUT` (600), `DRILL_HEALTH_TIMEOUT` (600), `DRILL_GRAPHQL_URL`.
