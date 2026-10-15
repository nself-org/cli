# Release Process

How an ɳSelf CLI release is cut. One script runs the whole chain from the owner's machine: `scripts/release/release.sh`. It uses your `gh` login and nothing else. It stores no token and asks for none.

## Before you start

- `gh auth login` as the repo owner. The script uses that session for every call.
- `git`, `gh`, `jq`, `curl` and `node` on the PATH (S1 checks them). `go` too, once `tools/facts` exists in cli (S3 runs it). `node` is for the web lock PR in S7.
- The registry secrets are set for the cli repo (P7-PROD-61): `DOCKERHUB_USERNAME`, `DOCKERHUB_TOKEN` and the npm token. S1 warns if Docker Hub's is missing. S8 to S10 fail without them.
- Run it in a terminal you are sitting at. A real run refuses to start when stdin is not a TTY (exit 2). What the code guarantees is that `release.sh` never pre-answers a prompt and never feeds `admin-merge.sh` any input. A TTY check cannot prove a human is typing (a `script` or `expect` wrapper has a TTY too), so the discipline of answering each merge prompt yourself is yours.
- `main` of cli, admin and homebrew-nself is green and nothing you want released is waiting in an open PR.

## Always plan first

```bash
scripts/release/release.sh --plan --version 1.5.0
```

Plan mode prints one row per command, `S<n> <step-id> <repo> <command>`, and makes only read-only `gh api` calls. It writes nothing to any repo, and no state file; the only file it touches is a short-lived temp file in `$TMPDIR` that holds a `gh` error message and is removed at once. Read the rows, then run for real:

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
| S2 `admin-lockstep` | Bumps admin (`package.json`, `src/lib/cli-version.ts`, `Dockerfile`) with cli's `bump-version.sh --no-goldens` (nothing from cli is committed here, so the goldens are left alone), opens a PR on admin, waits for required checks, merges with `admin-merge.sh`. | `--from S2`. A closed PR must be deleted with its branch first. |
| S3 `cli-version` | Bumps the cli version files. The bump also regenerates the four version-hash goldens (`UPDATE_GOLDEN=1` through `TestWriteModeGolden` and `TestBuildPlanDevGolden`, so the release PR carries them), refuses any golden change beyond the hash on a `.nself/build-version` line, and re-runs both tests plain. When `tools/facts` exists it also runs `go run -mod=vendor ./tools/facts` and the `-readme README.md` form, so the facts file and README match the tag. Opens the PR, waits, merges. | `--from S3`. |
| S4 `tag` | Finds the merge commit of the S3 PR. Tags `origin/main` if it is that commit; if main has moved on but still contains it, tags the merge commit you reviewed (a commit outside main's history stops the run). Reads `.github/VERSION` at that commit, runs the tag-time denylist rules (see below), then creates an annotated tag there and pushes it. The tag push starts `release.yml`, which creates the release with its assets. An existing tag on a different commit stops the run; a tag is never moved. | `--from S4`. Close the milestone or get `license-audit.yml` green first. |
| S5 `build-wait` | Waits for the build job of the `release.yml` run whose head is the tag commit, then downloads the `release-checksums` artifact (fetched fresh each run, never trusted from a cache). | A red build: fix, re-run the failed jobs on GitHub, `--from S5`. Artifact expired (it lives one day) and the release is not published: re-run the WHOLE run with `gh run rerun <id> -R nself-org/cli` (not `--failed`), wait for the build, then `--from S5`. |
| S6 `homebrew-formula` | Waits until every `release.yml` job except the Homebrew lockstep job and the publish job has succeeded (a failed `Pre-Release Gate`, SBOM or provenance job stops it: the formula is never merged for a release that cannot publish). Then runs homebrew-nself's own `update-formula.sh --checksums-file` on the release checksums (the published `checksums.txt` once the release exists, otherwise the build artifact). That writes the darwin arm64 hash into `on_arm` and the amd64 hash into `on_intel`. Opens the PR, waits, merges, then re-runs the failed `verify-homebrew-lockstep` job. S6 counts as done only when the formula's version and both hashes match the checksums. | `--from S6`. A closed unmerged PR: delete its branch with `gh api -X DELETE repos/nself-org/homebrew-nself/git/refs/heads/<branch>` (the message prints the exact command), then re-run. |
| S7 `release-wait` | Waits for `release.yml` to succeed and lists the assets. Downloads the published `checksums.txt` and compares its darwin lines with `on_arm` and `on_intel` in `Formula/nself.rb` on homebrew-nself main; a rebuilt release has new hashes, and a mismatch stops the run. Then opens a PR on the web repo that pins the new cli tag (`update-lock.mjs`). The owner merges that PR; nothing deploys. | A failed run: the message names the failed jobs. Fix, re-run the failed jobs, then `--from S7`. Hash mismatch: `--from S6` re-writes the formula from the published checksums. |
| S8 `docker-verify` | Checks `nself/nself:X.Y.Z` on Docker Hub for linux/amd64 and linux/arm64. If absent, waits about 3 minutes for the `docker-publish.yml` run that the Release starts, then for that run to finish (up to 60 minutes: 180 polls of 20 seconds; the multi-arch build takes about 40 minutes under QEMU); dispatches it once only when none exists. | `--from S8`. Check the Docker Hub secrets. |
| S9 `admin-image` | Tags admin: an annotated `vX.Y.Z` on the S2 merge commit, pushed from the admin clone. Admin's `docker-publish.yml` starts on a `v*` tag push, and admin's release-tag-drift check fails 6 hours after the bump without the tag. The tag push also starts admin's other tag-triggered workflows (`release.yml` and `docker-publish.yml`). The tag goes on the S2 merge commit; if no merged lockstep PR is found, on the commit that set `package.json` to the version, never the tip of main. S9 then waits for that run for up to 60 minutes (it does not dispatch a second publish) and checks `nself/nself-admin:X.Y.Z`. Only if no run publishes the image does it dispatch the workflow once. An admin tag on another commit stops the run. | `--from S9`. |
| S10 `sdk-publish` | For each `sdk-*publish*.yml` in cli: done if a run on the tag succeeded, otherwise dispatches it on the tag's ref. Only `sdk-publish-go.yml` gets `-f version`; the py, ts and ts-sdk workflows would overwrite their own manifest version with it, so they publish the version committed at the tag. A file without `workflow_dispatch` is printed for a manual run. An API error stops the run. | `--from S10`. Check the npm and PyPI secrets. |

## Completion checks

For a stable release, finish these checks after S10 and after all tag-triggered SDK publishes are green. S2 must merge admin's three version files before S4 tags cli; S6 must merge the Homebrew formula PR with both darwin hashes before rerunning the failed `verify-homebrew-lockstep` job. The tap does not update itself just because the cli tag exists.

The Go SDK workflow creates `sdk/go/v2.M.P` on the same commit as CLI `v1.M.P`. Use the shared mapping script rather than copying the CLI version into a Go module tag. From the cli checkout, replace the example with the release being verified:

```bash
CLI_TAG=v1.5.0
SDK_TAG=$(bash sdk/go/scripts/sdk-tag.sh "$CLI_TAG")
SDK_VERSION=$(bash sdk/go/scripts/sdk-tag.sh --version "$CLI_TAG")
git fetch origin --tags
test "$(git rev-parse "$SDK_TAG^{commit}")" = "$(git rev-parse "$CLI_TAG^{commit}")"
GOPROXY=https://proxy.golang.org GOFLAGS=-mod=mod go list -m "github.com/nself-org/cli/sdk/go/v2@$SDK_VERSION"
```

If the tag or proxy resolution is missing, inspect `sdk-publish-go.yml` on the CLI tag and resume S10 after fixing its failure. Never move an existing SDK tag: the module proxy caches immutable versions. Pre-release CLI tags do not publish a Go SDK version.

Check the GitHub Latest slot again after the SDK publishes. SDK releases must use `--latest=false`; they must not displace the CLI release used by `install.sh`:

```bash
gh api repos/nself-org/cli/releases/latest \
  --jq '.tag_name, (.assets | length)'
gh release view "$CLI_TAG" -R nself-org/cli --json assets
```

The Latest tag must equal `$CLI_TAG`, and its assets must include the CLI platform tarballs and `checksums.txt`. Compare the published `checksums.txt` byte for byte with the Build job's `release-checksums` artifact; confirm the formula's two darwin hashes still match. If an SDK publish displaced Latest, restore the CLI release with `gh release edit "$CLI_TAG" -R nself-org/cli --latest`, then repeat the check. Finally, confirm `install.sh` selects that CLI release and the installed `nself version` reports its version and tag commit. Do not apply this stable Latest check to a pre-release.

## The denylist runs in two places

`.github/release-denylist.json` has five rules. S4 evaluates the three that can be decided at tag time:

- DENY-003: at least one commit since the prior tag.
- DENY-004: the newest `license-audit.yml` run on `main` succeeded. If it is still running (a merge to Go files starts one, about 20 minutes), S4 waits up to 30 minutes. S4 then reads the newest run on any branch, the exact query `pre-release-gate.yml` uses, and stops before tagging if the gate would fail (for example a newer failed PR-branch run). Messages: still running after 30 minutes (wait, `--from S4`); failed (fix the licenses, `gh workflow run license-audit.yml -R nself-org/cli --ref main`, `--from S4`); none on main (start one the same way). Do not dispatch a new run while one is in flight: it cancels the first.
- DENY-005: a closed milestone titled exactly `vX.Y.Z`, as `pre-release-gate.yml` requires. A milestone named `X.Y.Z` does not count. S4 reads milestones of every state, as the gate does, and needs exactly one milestone with that title, closed; an open duplicate stops the run and is named.

DENY-001 (cosign bundles) and DENY-002 (SBOMs) need the build output, so they are evaluated after the build by the Pre-Release Gate, the `denylist` job in `release.yml`. That job re-checks all five before anything is published. `scripts/release-create.sh` is not used here: it evaluates the post-build rules before the build exists, so it would always refuse at tag time. Recovery for a refusal: close the milestone or fix `license-audit.yml`, then `--from S4`.

## Merges are yours

S2, S3 and S6 merge PRs on protected repos. For each one the script polls the PR's required checks for up to 45 minutes (red stops the run, nothing merges; pending after 45 minutes stops it with `--from S<n>`), then runs `scripts/admin-merge.sh --repo nself-org/<repo> --pr <n>` with your terminal attached. If a PR shows no checks yet, the script asks branch protection what is required: none means it goes on to your prompt, otherwise it retries six times and then stops without merging. When checks come back green it also confirms each required check name is among the passing ones. That script snapshots branch protection, relaxes it for under a minute, merges, and restores it. You answer its `[y/N]` prompt. `--yes` does not cover it, and the script never pipes an answer in.

`admin-merge.sh` exit codes: `0` merged, and protection restored and verified. `1` usage or pre-flight error, or the PR did not reach MERGED; protection was restored and verified first, so nothing stays relaxed. Fix the cause and rerun. `3` the restore failed or could not be verified, so protection may be weaker than before. Run `scripts/admin-merge.sh --force-restore --repo nself-org/<repo> --snapshot <file>` (the snapshot path is in the `RESTORE_FAILED` or `restore_mismatch` audit line) and check the result against the baseline before anything else. On any non-zero exit, read its message and its audit log (`~/.nself/admin-merge-audit.log`).

## Recovery

- Any step: fix the cause, then `release.sh --version X.Y.Z --from S<n>`.
- Guard rules are in the exit-code paragraph above. A stale branch on the remote from an earlier attempt stops the run with the exact `gh api -X DELETE …/git/refs/heads/<branch>` command to clear it. A work directory that is not a clone is named in the message; delete it and re-run.
- Wrong tag or broken release: use `scripts/release-retract.sh`, then start again with a new patch version.
- Artifacts expire after a day. Before the release is published there is no asset to fall back on, so S5 stops with the real recovery: re-run the whole `release.yml` run, wait for the build, `--from S5`. After publish, S6 and S7 use the published `checksums.txt`.
- If the script cannot be used, P7-PROD-23 keeps a fallback token path for the old dispatch workflows. It is a fallback only; do not use it for a normal release.

## Version files

`scripts/bump-version.sh` edits every lockstep file in one go: `.github/VERSION`, `internal/version/version.go`, the SDK manifests, the four version-hash goldens (skip with `--no-goldens`; needs `go`), and in admin `package.json`, `src/lib/cli-version.ts` and `Dockerfile`. Do not hand-edit them. Tags are `vX.Y.Z`.

## See Also

- [[Contributing]], how to contribute code
- [[Dev-Setup]], local development setup
- [[Changelog]], version history

---
← [[Home]] | [[_Sidebar]]
