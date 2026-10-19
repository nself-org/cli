# Importers Drill

Source platforms change under us. This [drill](Drills) reruns every importer end to end, plus the export round trip, so a drift in an upstream image, a changed API or a regression in the importer shows up the night it happens instead of the day a user comes home.

It is the job `drill-importers` of the `drills` pipeline: `scripts/drill/legs/importers/run`.

## What it proves

One check per source. Each runs the same Linux e2e script a developer runs by hand, against a local, pinned fixture. No credential is involved and nothing leaves the runner.

| Check id | Script | Guide |
|---|---|---|
| `importers/postgres` | `scripts/e2e/import-postgres.sh` | [Come-Home-Postgres](Come-Home-Postgres) |
| `importers/nhost` | `scripts/e2e/import-nhost.sh` | [Come-Home-nhost](Come-Home-nhost) |
| `importers/hasura-cloud` | `scripts/e2e/import-hasura-cloud.sh` | [Come-Home-Hasura-Cloud](Come-Home-Hasura-Cloud) |
| `importers/supabase` | `scripts/e2e/import-supabase.sh` | [Come-Home-Supabase](Come-Home-Supabase) |
| `importers/firebase` | `scripts/e2e/import-firebase.sh` | [Come-Home-Firebase](Come-Home-Firebase) |
| `importers/appwrite` | `scripts/e2e/import-appwrite.sh` | [Come-Home-Appwrite](Come-Home-Appwrite) |
| `importers/round-trip` | `scripts/e2e/export-round-trip.sh` | [Import-Export-Format](Import-Export-Format) |

`bash scripts/drill/legs/importers/run --list` prints the seven ids and runs nothing.

## How it runs

- One fixture at a time, in the order above. A source that fails does not stop the next one, and the leg exits 1 when any check did not pass.
- Each script runs the way the e2e line of the importers plan runs it: a `docker:27-cli` container on the host network with the docker socket, the checkout and a work dir mounted at the same path as on the host, and a static Linux `nself` built from this checkout on `PATH`. The script tears its own fixture down (`compose down -v`) in a trap; the leg removes the container and the work dir after each source, and on CI it also removes any container, volume or network left behind that carries one of the scripts' own project names (`a<NN>…-`, `e2e-adopt-rt-`; the pipeline runs the other drill legs on the same Docker daemon at the same time, so nothing else is touched) and removes unused images when the runner is short of disk.
- Time: 15 minutes per source, 75 minutes for the whole leg. A source that overruns, or that has no time left, is an `error` check with failure class `timeout`. A skipped source is never reported as a pass.
- The nightly workflow passes `--timeout 80m` to `nself-ci`, so every drill job may run 80 minutes and the 75-minute budget fits. (A drill job has a 10-minute default; the `NSELF_CI_TIMEOUT` variable alone does not raise it for discovered drill jobs.)
- It runs on a GitHub-hosted `ubuntu-latest` runner. A run on a Mac proves nothing for the record.

## The evidence

The leg prints progress lines on stderr and, as the last line of stdout, one compact `ci.evidence/v1` document with one check per source (`importers/<source>`, kind `test`, required). `nself ci` reads that line, validates it, and folds the failing checks into the one check of the `drill-importers` job. The pipeline evidence of the nightly run is the artifact `drills-evidence`; per-source logs are in `~/.nself/drill-out/importers/<source>.log`, uploaded as the artifact `drills-importers-results`.

A failed check carries the exit code, `failure_class` `code`, and the last lines of the script output as its excerpt. An `error` check means the script did not get an answer: the budget ran out, Docker or Go was missing, or the image could not be pulled.

## Run it

```
nself-ci --pipeline drills              # all drills, including this one
bash scripts/drill/legs/importers/run   # this leg alone (needs Docker and Go)
```

A full run needs Docker with about 15 GiB of free disk for the fixture images and takes tens of minutes.

Test the leg itself without Docker by pointing it at stub scripts named `<source>.sh`:

```
D=$(mktemp -d)
for s in postgres nhost hasura-cloud supabase firebase appwrite round-trip; do
  printf '#!/bin/sh\nexit %s\n' "$([ "$s" = firebase ] && echo 1 || echo 0)" > "$D/$s.sh"
done
DRILL_SOURCES_DIR="$D" bash scripts/drill/legs/importers/run   # firebase fails, the rest run, exit 1
```

| Variable | Meaning | Default |
|---|---|---|
| `DRILL_SOURCES_DIR` | run `<dir>/<source>.sh` directly with bash, no container | unset |
| `DRILL_OUT_ROOT` | result root; logs go to `<root>/importers/` | `~/.nself/drill-out` |
| `DRILL_SOURCE_TIMEOUT` | seconds per source | 900 |
| `DRILL_BUDGET` | seconds for the whole leg | 4500 |
| `NSELF_LINUX_BIN` | a static Linux `nself` to mount instead of building one | built from the checkout |

## Staleness and the pass count

On GitHub a throwaway runner keeps no run store, so a red nightly run is the alert and a missing run is the staleness signal (see [Drills](Drills)). The drill must pass once on the pull request that adds it. Three consecutive green nights are counted once, in the ship gate, not here.

## When a source turns red

1. Open the `drills-evidence` artifact and read the excerpt of the failing check, then its full log in the artifact `drills-importers-results` (`<source>.log`).
2. Fixtures are pinned by digest in `test/fixtures/import/<source>/`. If an upstream image moved, bump the digest deliberately in the fixture and rerun.
3. If the script drifted from the importer's behaviour, fix the script in the same change that changed the behaviour, and rerun this leg.

[[Home]] · [Drills](Drills) · [Two-Command Drill](Two-Command-Drill)
