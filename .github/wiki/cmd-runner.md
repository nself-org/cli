# nself runner

<!-- BEGIN PROSE:summary -->
> Provision and audit self-hosted GitHub Actions CI runner hosts.
<!-- END PROSE:summary -->

## Synopsis

```
nself runner <subcommand> [flags]
```

## Description

<!-- BEGIN PROSE:description -->
Provision and audit self-hosted GitHub Actions CI runner hosts.

Runner hosts were previously hand-built: required system dependencies (gh,
zip, unzip, Playwright/Chromium's shared libraries, ...) were discovered
only when a job failed mid-run, and two hosts advertising the identical
GitHub Actions labels (self-hosted,Linux,X64) could silently drift apart —
the same commit would pass or fail depending on which host claimed the job.

The dependency set is declarative (internal/runner/manifest.yaml, compiled
into this binary) so provision and verify always check the same list.

Subcommands:
  provision   Install dependencies, create the runner user, register N
              runner instances as systemd services
  verify      Check one or more hosts against the manifest and print a
              parity matrix — the important half: this is how "same
              labels, different tools" gets caught before it causes a
              mystery failure.
<!-- END PROSE:description -->

## Flags

<!-- BEGIN GENERATED:flags -->
| Flag | Default | Description |
|------|---------|-------------|
| `--help`, `-h` | — | Show help |
<!-- END GENERATED:flags -->

## Subcommands

<!-- BEGIN GENERATED:subcommands -->
| Name | Description |
|------|-------------|
| `provision` | Install runner dependencies, user, sudoers, and N runner instances |
| `verify` | Check host(s) against the dependency manifest and report drift |
<!-- END GENERATED:subcommands -->

## Examples

<!-- BEGIN PROSE:examples -->
```bash
# Check this machine against the manifest
nself runner verify

# Check two remote runner hosts in one pass and print a parity matrix —
# this is how "same GitHub Actions labels, different tools installed"
# gets caught before it causes a mystery job failure
nself runner verify --host ci@runner-a.example.com --host ci@runner-b.example.com

# Machine-readable output for a CI gate
nself runner verify --host ci@runner-a.example.com --json

# Provision a fresh host with 2 runner instances
GITHUB_RUNNER_TOKEN=... nself runner provision \
  --host ci@runner-a.example.com \
  --github-url https://github.com/nself-org/cli \
  --instances 2 \
  --labels nself-ci
```
<!-- END PROSE:examples -->

## See Also

<!-- BEGIN PROSE:see-also -->
- [[Commands]] — full command index
- [[Core-Services]] — what a stack is made of
<!-- END PROSE:see-also -->

← [[Commands]] | [[Home]] →
