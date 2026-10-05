# Branch Protection Baseline

The baseline is data, not prose: [`scripts/.policy/branch-protection.yaml`](../../scripts/.policy/branch-protection.yaml)
is the single source of truth for `main` protection on every nself-org repo
(defaults plus per-repo overrides under `repos:`). This page holds no setting
values, so there is nothing here to go stale. Read the YAML.

Never change protection in the GitHub web UI or with ad-hoc `gh api` calls. A
change goes through the YAML and `scripts/branch-protection-toggle.sh`, and every
change to GitHub is owner-named (P7-HYG-35).

## Check for drift (read-only)

`--dry-run` makes GET calls only and never writes to GitHub:

```bash
for r in cli admin plugins packages homebrew-nself ntask nchat nclaw nsentry nfamily clawde; do
  bash scripts/branch-protection-toggle.sh --on --dry-run --repo nself-org/$r
  sleep 5
done
```

The toggle sends and compares status checks as `checks` (context plus `app_id`),
never as bare `contexts`: a contexts-only PUT drops the app pins GitHub holds
(most `main` branches pin their checks to the GitHub Actions app, id 15368). The
YAML lists context names; each context keeps the `app_id` it has on the live
branch, and a context with no live pin is sent as `-1` (any app). So `--on` never
loses a pin. Because the pin is copied from live, the drift check does NOT detect
a swapped pin (the YAML cannot express one); a context pinned to two different
apps live makes the toggle refuse and send nothing. `scripts/ci/testdata/nself-ci-protect/toggle_pins_test.sh`
proves it against recorded app-pinned fixtures.

Each public repo prints `already matches baseline — no-op` when live protection
equals the YAML on the compared fields (strict, contexts, enforce_admins, the four
review settings, restrictions, force pushes, deletions). Anything else prints
`DRY-RUN: would PUT`, which means the YAML or GitHub drifted. Empty public repos
(no default branch) print `n/a: no default branch` and are not drift.

The YAML was last reconciled with live protection on 2026-10-04 (P7-HYG-14).
Snapshots of the live API answers from that day are kept in the hq evidence tree.

## Changing the baseline

1. Edit `scripts/.policy/branch-protection.yaml` (a default, or a per-repo
   override with a comment saying why).
2. Run the drift loop above to see which repos differ.
3. The owner applies it with `scripts/branch-protection-toggle.sh --on --repo <r>`.
   Review counts are never lowered. `--rollback` restores the previous state.

## Pending `nself-ci` checks (not required yet)

The `pending_nself_ci:` block of the YAML lists the app-pinned `nself-ci` checks
that become required once `nself ci` posts them (cli and packages: `nself-ci`;
plugins: `nself-ci` and `nself-ci/source`; each `{context, app_id: 15368}`). The
toggle ignores the block, so the baseline keeps matching live protection and a
drift check never reports the pending checks. Only `scripts/ci/nself-ci-protect.sh`
reads it. The toggle does not apply a pending check, but it carries one that is
already live with the same `app_id` (so after `--apply`, the next `--on` keeps
the gate and a drift check reads no-op). A live `nself-ci` pinned to another app
is not in the baseline and is dropped like any unlisted check.

```bash
# Read-only: GET the live protection, list the added checks, print the PUT body.
bash scripts/ci/nself-ci-protect.sh --repo nself-org/cli --dry-run

# Owner-named only (P7-CI-55): PUT the target, GET again, compare with the target.
bash scripts/ci/nself-ci-protect.sh --repo nself-org/cli --apply

# Put the saved pre-state back and verify it.
bash scripts/ci/nself-ci-protect.sh --repo nself-org/cli --restore <pre-state file> [--dry-run]

# Fixture tests (stub gh, no network).
bash scripts/ci/nself-ci-protect.sh --self-test
```

The PUT body is `snapshot_to_put_body` of the live state plus the pending checks,
so every existing check keeps its `app_id` and every other setting is replayed as
it was. Every run writes the live state it read to a new file under
`${NSELF_CI_PROTECT_DIR:-$HOME/.nself/protection}` (never overwritten) and prints
its path: that file is the argument of `--restore`. A pending context already
required with the same `app_id` is skipped; one already required with another
`app_id` is a conflict and stops the run before any PUT. A GET answer that is not
a protection document for `--repo` (for example `{}`), and a `--restore` file that
belongs to another repo, are refused before any PUT. `--restore --dry-run` prints
the body it would PUT.

A required check is the job `name:` GitHub registers for a workflow that runs on
`pull_request` targeting `main`. Add the exact string to the repo's `contexts` in
the YAML (lists are compared by context name, with their `app_id`).

## How owner PRs merge

Where the YAML sets `enforce_admins`, even the owner cannot bypass a red or
unapproved PR, so the owner merges through `scripts/admin-merge.sh --repo <r>
--pr <n>` (snapshot, short relax with a watchdog, merge, restore). Elsewhere the
owner merges green PRs by admin bypass. No ticket relaxes protection itself.

## Private repositories

`nself-org/web` and `nself-org/bundles` are private. On the free GitHub plan the
branch-protection API answers 403 ("Upgrade to GitHub Pro or make this repository
public to enable this feature"), so `main` cannot be protected there and the
toggle is never run on them (D-0012, accepted). The YAML records both as
unprotected, with the reason, in comments.

Compensating control, until it is built: the repos' own self-hosted CI plus the
convention that nothing merges red. The planned control is `nself ci` as a local
gate that posts the `nself-ci` status, plus the pre-push hook (P7-CI-07).

Re-entry trigger: a repo is made public (it then gets an override in the YAML and
is applied by the toggle). An agent never chooses a paid plan to close this gap.
