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

A required check is the job `name:` GitHub registers for a workflow that runs on
`pull_request` targeting `main`. Add the exact string to the repo's `contexts` in
the YAML, in the order GitHub reports (lists are compared in order).

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
