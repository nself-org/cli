# nself upgrade

<!-- BEGIN PROSE:summary -->
> Upgrade the ɳSelf CLI (detects install method).
<!-- END PROSE:summary -->

## Synopsis

```
nself upgrade [flags]
```

## Description

<!-- BEGIN PROSE:description -->
`nself upgrade` upgrades the CLI binary. It detects the install method (Homebrew, direct download, or system package manager) and applies the appropriate strategy: Homebrew users get a `brew upgrade` instruction (or the canary tap for the canary channel), system-package users are pointed at their package manager, and direct-install users get an in-place binary swap with rollback support.

Use `--check` to inspect current version, install method, and channel without installing. Use `--channel` to switch between `stable` and `canary`; the choice is persisted in `~/.config/nself/channel.json`. Use `--version 1.2.3` to pin a specific release. Use `--rollback` to restore the previously installed binary from `~/.nself/bin/nself.prev`.

Before swapping the binary, the command warns about installed Licensed plugins that may be incompatible with the new version. After a successful upgrade, the previous binary is preserved for one-shot rollback.

### --binary-url: install from a specific URL

Operators running air-gapped environments, staging mirrors, or custom build pipelines can use `--binary-url` to bypass the GitHub releases API entirely and install a specific binary directly.

```bash
nself upgrade --binary-url https://install.nself.org/builds/v1.0.13-rc1/nself-linux-amd64.tar.gz
```

The command:

1. Validates the URL is HTTPS and the host is on the allowed list (see below).
2. Derives the checksum file URL from the same directory (`checksums.txt` in the same path).
3. Downloads the binary or archive.
4. Verifies the SHA-256 checksum against `checksums.txt`. The install is aborted if the checksum does not match.
5. Backs up the current binary to `~/.nself/bin/nself.prev`.
6. Atomically swaps the running binary with the new one.

If the install fails for any reason, run `nself upgrade --rollback` to restore the previous binary.

**Allowed hosts for `--binary-url`:**

- `github.com` and subdomains
- `objects.githubusercontent.com` and subdomains
- `install.nself.org` and subdomains
- `ping.nself.org` and subdomains

URLs from any other host are rejected. Plain `http://` URLs are never accepted, only `https://`.

**Checksum file convention:** the checksum file must live in the same URL directory as the binary and be named `checksums.txt`. Each line follows the standard `sha256sum` format:

```
<sha256hex>  <filename>
```
<!-- END PROSE:description -->

## Flags

<!-- BEGIN GENERATED:flags -->
| Flag | Default | Description |
|------|---------|-------------|
| `--binary-sha256` | `""` | Operator-supplied SHA-256 digest for --binary-url (skips checksums.txt fetch) |
| `--binary-url` | `""` | Install from a specific binary URL (HTTPS only; .tar.gz/.tgz; operator use) |
| `--channel` | `""` | Release channel: stable or canary |
| `--check` | `false` | Check for updates without installing |
| `--rollback` | `false` | Revert to the previously installed version |
| `--version` | `""` | Upgrade to a specific version (e.g. 1.2.3) |
| `--help`, `-h` | — | Show help |
<!-- END GENERATED:flags -->

## Examples

<!-- BEGIN PROSE:examples -->
```bash
# Check current version, channel, and install method
nself upgrade --check

# Upgrade to the latest stable
nself upgrade

# Switch to the canary channel and upgrade
nself upgrade --channel canary

# Pin to a specific version
nself upgrade --version 1.0.6

# Roll back to the previously installed binary
nself upgrade --rollback

# Install a specific binary from a staging mirror (operator)
nself upgrade --binary-url https://install.nself.org/builds/v1.0.13-rc1/nself-linux-amd64.tar.gz

# Install a specific binary from a GitHub release asset
nself upgrade --binary-url https://github.com/nself-org/cli/releases/download/v1.0.13/nself-darwin-arm64.tar.gz
```
<!-- END PROSE:examples -->

## See Also

<!-- BEGIN PROSE:see-also -->
- [[cmd-update]], update CLI and admin Docker image
- [[cmd-version]], show current version and system info
- [[cmd-doctor]], diagnose CLI/install issues
- [[Commands]], full command index
<!-- END PROSE:see-also -->

← [[Commands]] | [[Home]] →
