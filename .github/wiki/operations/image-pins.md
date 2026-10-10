# Image Pin Bump Runbook

Image pins move only through one reviewed pull request a week. See [ADR 0030: Image Supply Chain](../../architecture/adr/0030-image-supply-chain.md) for the reason, and [Image Mirror Runbook](image-mirror.md) for the mirror tool this flow calls.

## Weekly flow

The workflow `.github/workflows/image-pin-bump.yml` runs every Monday at 06:00 UTC. It can also be started by hand (`workflow_dispatch`).

1. `tools/imagebump` reads `internal/compose/images.yaml` and the policy `.github/image-bump-policy.yaml`. For each entry it lists the registry tags (through `scripts/images/mirror-images.sh --list-tags`) and picks the newest tag the policy allows.
2. It rewrites the `version` of the entries that moved and nothing else in the file.
3. It runs `go run ./tools/imagelock -resolve`, which regenerates `internal/compose/images.lock.json`. This also runs when no version moved, so floating tags such as `latest` or `alpine` refresh their digests.
4. `go run ./tools/imagelock -check` verifies the lock.
5. If nothing changed, the job prints `no pin changes` and ends green.
6. Otherwise the job opens one PR from branch `chore/image-pins` with the label `image-pins`. A PR that is still open is updated in place.
7. On the branch head the job runs three checks and writes the results into the PR body:
   - the image probe (`default-images-pullable.yml`),
   - the stack smoke (`e2e-golden-path.yml` with `source=local`),
   - `scripts/images/mirror-images.sh --plan` (status-line counts and the plan detail).
8. The job fails if the probe, the smoke or the plan failed. The PR body is written first.

The PR is never merged automatically. A person reads the bumps and the check results, then merges.

PRs opened with the workflow token do not start the normal PR checks. Close and reopen the PR to run them.

Nothing is pushed to Docker Hub by the PR or by the weekly job.

Listing tags is slow for one repository: `docker.elastic.co/elasticsearch/elasticsearch` has about 49000 tags and takes roughly 15 minutes. The job allows 30 minutes per repository by default. Set `IMAGEBUMP_LIST_TIMEOUT` (a Go duration such as `45m`) to change that.

## Policy file

`.github/image-bump-policy.yaml` has one entry per non-plugin entry of `images.yaml`. Plugin pins move with plugin releases and are skipped.

```yaml
schema_version: 1
services:
  hasura: {regex: '^v(\d+)\.(\d+)\.(\d+)$', track: patch}
```

- `regex` is an anchored RE2 expression (`^...$`). Its capture groups are the numeric version components. A tag the regex does not match is never chosen, so release candidates and variant tags stay out.
- `track: patch` keeps the first two groups equal to the current tag's. Only the rest can grow.
- `track: minor` keeps the first group equal. Use it only where upstream ships fixes as minor releases, and give the reason in a comment.
- A tag with fewer groups than the track fixes never moves. A regex with no groups (`latest`, `alpine`, `pg16`) never moves; only its digest refreshes.
- `track: manual` skips the entry. The committed policy does not use it.
- The highest tag by numeric comparison wins (`v1.10.0` is newer than `v1.9.0`). The tool never proposes a lower version.

`tools/imagebump` exits 1 and names the entry when an image has no policy entry or its current version does not match its regex. Add the policy entry in the same change that adds the image.

Moving a floating line (for example Postgres 16 to 17) is a deliberate edit of `images.yaml`, not a weekly bump.

## Try it locally

```bash
go run ./tools/imagebump --dry-run          # prints BUMP lines, writes nothing
bash scripts/images/mirror-images.sh --plan # status lines on stdout, detail on stderr
```

Both need Docker. A dry run on the throwaway branch `chore/image-pins-dry-run` runs the whole workflow without a PR:

```bash
gh workflow run image-pin-bump.yml -f dry_run_pr=true -R nself-org/cli
```

The rendered PR body appears in the job summary and the branch is deleted at the end.

## Owner switch: mirror apply

After a merge that changes `internal/compose/images.lock.json`, the job `mirror-apply` runs. It publishes the mirrors (`mirror-images.sh --apply`, using the `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN` secrets) only when the repository variable `IMAGE_MIRROR_APPLY` is `true`. Otherwise the run log shows `skipped: IMAGE_MIRROR_APPLY != true`.

The owner turns it on and off:

```bash
gh variable set IMAGE_MIRROR_APPLY --body true -R nself-org/cli
gh variable delete IMAGE_MIRROR_APPLY -R nself-org/cli
```

Mirroring is a public push to `docker.io/nself`. Leave the variable unset until you want that.

## Rollback

Revert the merged PR. The lock returns to the previous pins on the next release. Mirrors are additive: a mirrored tag stays on Docker Hub and nothing needs deleting.
