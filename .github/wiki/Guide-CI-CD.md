# Guide: CI/CD

nSelf gates code locally. The gate is the `nself-ci` suite, and its result is a GitHub commit status that branch protection can require. Deploys never depend on GitHub Actions.

Reference: [[cmd-ci]]. Full CI detail lives in the plugins repo and is linked, not copied: [CI Getting Started](https://github.com/nself-org/plugins/blob/main/.github/wiki/guides/ci/Getting-Started.md) and the [CI guides folder](https://github.com/nself-org/plugins/tree/main/.github/wiki/guides/ci).

## Install the CI plugin

```bash
nself add ci
```

## Run the local gate

```bash
nself ci --check
```

`--check` runs lint, tests, build and the secret scan, and posts nothing. Run `nself ci` without it to also post the `nself-ci` commit status for the current commit. Pass a path to gate another checkout, for example `nself ci ./backend`.

## Branch protection

Require the `nself-ci` status on your main branch instead of Actions checks. See the CI Getting Started page above for the exact settings.

## Test tiers

There is no staging server (ADR 0014). Use two tiers:

1. Local servers: run the stack on your machine with `nself init` and `nself start`, and run `nself ci` against it.
2. Production dry-run: preview a production change without applying it before the real deploy.

```bash
nself start
```

Take a verified backup before any production action. See [[Guide-Production-Deployment]] and [[Guide-Backup-Restore]].
