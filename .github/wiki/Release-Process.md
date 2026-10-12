# Release Process

How an ɳSelf CLI release is cut. One script runs the whole chain from the owner's machine: `scripts/release/release.sh`. It uses your `gh` login and nothing else. It stores no token and asks for none.

## Before you start

- `gh auth login` as the repo owner. The script uses that session for every call.
- `git`, `gh`, `jq`, `curl` and `node` on the PATH (S1 checks them). `go` too, once `tools/facts` exists in cli (S3 runs it). `node` is for the web lock PR in S7.
- The registry secrets are set for the cli repo (P7-PROD-61): `DOCKERHUB_USERNAME`, `DOCKERHUB_TOKEN` and the npm token. S1 warns if Docker Hub's is missing. S8 to S10 fail without them.
- Run it in a terminal you are sitting at. A real run refuses to start when stdin is not a TTY (exit 2), because you answer every merge prompt yourself.
- `main` of cli, admin and homebrew-nself is green and nothing you want released is waiting in an open PR.

## Always plan first

```bash
scripts/release/release.sh --plan --version 1.5.0
```

Plan mode prints one row per command, `S<n> <step-id> <repo> <command>`, and makes only read-only `gh api` calls. It writes nothing, in the repo or anywhere else. Read the rows, then run for real:

```bash
scripts/release/release.sh --version 1.5.0
```

Flags:

| Flag | Meaning |
|---|---|
| `--version X.Y.Z` | Required. No leading `v`. |
| `--plan` | Print the steps. Change nothing. |
| `--from S<n>` | Resume at step `n`. Earlier steps are skipped. |
| `--yes` | Skip the script's own `Run S<n>? [y/N]` question. It never answers a merge prompt. |

Exit codes: `0` done, `1` a step failed (the message names the step and how to resume), `2` refused input. Refusals are: no or bad `--version`, stdin not a TTY, a version older than the latest release, a fresh start (or `--from S2`, `S3`) when the version is already the latest or its tag exists, and `--from S4` when the version is already the latest. From S5 on, the tag and a published release are expected. `--from S4` is allowed with the tag present: S4 only confirms the tag sits on the merged commit.

## The ten steps

Every step first checks whether GitHub already shows it finished. If so it prints `S<n> <id> done` and changes nothing. That is what makes a re-run safe. Work happens in temporary clones under `$TMPDIR/nself-release-<version>/`, removed when the run finishes. Resume data is in `$TMPDIR/nself-release-<version>.state`, and every step can also rebuild it from GitHub.

| Step | What it does | If it fails |
|---|---|---|
| S1 `preflight` | Checks tools and `gh` login, clones cli, admin and homebrew-nself, warns about missing Docker Hub secrets. | Fix the tool or login. Re-run from S1. |
| S2 `admin-lockstep` | Bumps admin (`package.json`, `src/lib/cli-version.ts`, `Dockerfile`) with cli's `bump-version.sh`, opens a PR on admin, waits for required checks, merges with `admin-merge.sh`. | `--from S2`. A closed PR must be deleted with its branch first. |
| S3 `cli-version` | Bumps the cli version files. When `tools/facts` exists it also runs `go run -mod=vendor ./tools/facts` and the `-readme README.md` form, so the facts file and README match the tag. Opens the PR, waits, merges. | `--from S3`. |
| S4 `tag` | Checks the merged commit equals `origin/main`, runs the tag-time denylist rules (see below), then creates an annotated tag on that commit and pushes it. The tag push starts `release.yml`, which creates the release with its assets. An existing tag on a different commit stops the run; a tag is never moved. | `--from S4`. Close the milestone or fix `license-audit.yml` first. If main moved after the merge it stops; inspect before tagging by hand. |
| S5 `build-wait` | Waits for the `build` job of `release.yml`, then downloads the `release-checksums` artifact. | A red build: fix, re-run the failed jobs on GitHub, `--from S5`. |
| S6 `homebrew-formula` | In homebrew-nself runs its own `update-formula.sh --checksums-file` on the build's checksums. That writes the darwin arm64 hash into `on_arm` and the amd64 hash into `on_intel`. Opens the PR, waits, merges, then re-runs the failed `verify-homebrew-lockstep` job. A re-run of S6 after the formula merged still does that re-run if the job failed. | `--from S6`. A closed unmerged PR: delete its branch with `gh api -X DELETE repos/nself-org/homebrew-nself/git/refs/heads/release/nself-vX.Y.Z`, then re-run. |
| S7 `release-wait` | Waits for `release.yml` to succeed and lists the assets. Then opens a PR on the web repo that pins the new cli tag (`update-lock.mjs`). The owner merges that PR; nothing deploys. | If the run failed, re-run its failed jobs, then `--from S7`. |
| S8 `docker-verify` | Checks `nself/nself:X.Y.Z` on Docker Hub for linux/amd64 and linux/arm64. If absent, waits for a `docker-publish.yml` run that is in flight or finished since the release; dispatches it once only when there is none. | `--from S8`. Check the Docker Hub secrets. |
| S9 `admin-image` | Same wait-then-dispatch rule for admin's `docker-publish.yml`; checks `nself/nself-admin:X.Y.Z` the same way. | `--from S9`. |
| S10 `sdk-publish` | For each `sdk-*publish*.yml` in cli: done if a run on the tag succeeded, otherwise dispatches it with the version. A file without `workflow_dispatch` is printed for a manual run. | `--from S10`. Check the npm and PyPI secrets. |

## The denylist runs in two places

`.github/release-denylist.json` has five rules. S4 evaluates the three that can be decided at tag time:

- DENY-003: at least one commit since the prior tag.
- DENY-004: the latest `license-audit.yml` run on `main` succeeded.
- DENY-005: a closed milestone titled `vX.Y.Z` or `X.Y.Z` (either is accepted).

DENY-001 (cosign bundles) and DENY-002 (SBOMs) need the build output, so they are evaluated after the build by the Pre-Release Gate, the `denylist` job in `release.yml`. That job re-checks all five before anything is published. `scripts/release-create.sh` is not used here: it evaluates the post-build rules before the build exists, so it would always refuse at tag time. Recovery for a refusal: close the milestone or fix `license-audit.yml`, then `--from S4`.

## Merges are yours

S2, S3 and S6 merge PRs on protected repos. For each one the script waits until the PR's required checks are green (red stops the run, nothing merges), then runs `scripts/admin-merge.sh --repo nself-org/<repo> --pr <n>` with your terminal attached. If a PR shows no checks yet, the script asks branch protection what is required: none means it goes on to your prompt, otherwise it retries six times and then stops without merging. That script snapshots branch protection, relaxes it for under a minute, merges, and restores it. You answer its `[y/N]` prompt. `--yes` does not cover it, and the script never pipes an answer in.

If `admin-merge.sh` exits non-zero, protection may not be restored. Read its message and its audit log (`~/.nself/admin-merge-audit.log`) before you do anything else.

## Recovery

- Any step: fix the cause, then `release.sh --version X.Y.Z --from S<n>`.
- Guard rules are in the exit-code paragraph above. A stale branch on the remote from an earlier attempt stops the run with the exact `gh api -X DELETE …/git/refs/heads/<branch>` command to clear it. A work directory that is not a clone is named in the message; delete it and re-run.
- Wrong tag or broken release: use `scripts/release-retract.sh`, then start again with a new patch version.
- Artifacts expire after a day. If S6 runs later, it falls back to the release's `checksums.txt` asset.
- If the script cannot be used, P7-PROD-23 keeps a fallback token path for the old dispatch workflows. It is a fallback only; do not use it for a normal release.

## Version files

`scripts/bump-version.sh` edits every lockstep file in one go: `.github/VERSION`, `internal/version/version.go`, the SDK manifests, and in admin `package.json`, `src/lib/cli-version.ts` and `Dockerfile`. Do not hand-edit them. Tags are `vX.Y.Z`.

## See Also

- [[Contributing]], how to contribute code
- [[Dev-Setup]], local development setup
- [[Changelog]], version history

---
← [[Home]] | [[_Sidebar]]
