# Support Report

Build a redacted support bundle you can attach to a GitHub issue or send to support.

The `nself doctor report` subcommand creates a tar.gz file with diagnostics and configuration, with sensitive values automatically redacted.

## Usage

```bash
# Create a report with default settings (last 200 log lines per service)
nself doctor report

# Specify output file path
nself doctor report --output /tmp/my-report.tar.gz

# Include more log lines
nself doctor report --log-lines 500

# Write the archive and print a JSON summary (path, files, bytes, redactions)
nself doctor report --json
```

By default, the report is saved as `nself-report-<UTC timestamp>.tar.gz` in the project directory with mode `0600` (readable only by you). The path is printed on exit. If a file with that name exists, the command exits with an error and does not overwrite it.

## What is collected

| File | Contents |
|------|----------|
| `versions.json` | nself version, Docker version, Docker Compose version, OS version and kernel |
| `config-keys.txt` | Names of environment variables (values never included) |
| `docker-compose.yml` | Generated compose file from your project |
| `nginx/` | All generated nginx config files (no certificates or keys) |
| `doctor.txt` | Full output of `nself doctor`, run with 120-second timeout |
| `logs/<service>.log` | Last N lines per running service (default 200, configurable with `--log-lines`) |
| `manifest.json` | File list, redaction counts per class, notes (e.g. "no running stack") |

## What is never collected

These items are explicitly excluded for security and privacy:

- Values of any environment variable (only variable names are collected)
- `.env*` files in any form
- Secret store content or credential metadata
- SSL/TLS certificates and private keys (the `ssl/` directory is skipped)
- License key data from `~/.nself`

## How redaction works

The bundle redacts sensitive values in this order:

1. **Secret-like variables:** Any environment variable whose name contains `SECRET`, `PASSWORD`, `PASS`, `TOKEN`, `KEY`, `PRIVATE`, `CREDENTIAL`, or `DSN` is replaced with `[REDACTED:<NAME>]` everywhere it appears, including in URL-escaped and JSON-escaped forms.

2. **PEM blocks:** All private key and certificate PEM blocks are replaced with `[REDACTED:PEM]`.

3. **Passwords in URLs:** Patterns like `scheme://user:password@host` become `scheme://[REDACTED]@host`.

4. **Standard patterns:** JWTs, API key prefixes (e.g. `sk-`, `sk_`), email addresses, and IP addresses are redacted.

5. **Final pass:** Any assignment matching `NAME=value`, `NAME: value`, or `--password value` where the name looks secret-like is redacted.

The `manifest.json` file counts each redaction class, so you can verify coverage before sharing.

## Without a running stack

If no ɳSelf stack is running, the report still succeeds:

- The `logs/` directory is empty
- The `manifest.json` file explains why (e.g., "no containers running")
- All other files are included

## Review before you share

Always inspect the report on your machine before sharing it:

```bash
# List the contents
tar -tzf nself-report-*.tar.gz

# Extract to review
mkdir -p review-report
tar -xzf nself-report-*.tar.gz -C review-report

# Search for a value you know is secret (type it, do not paste it into a shared terminal log)
grep -rF 'your-known-secret' review-report
```

If you find any unredacted sensitive value, do not share the file. Open an issue describing the class of leak (never paste the actual value).

## See Also

- [[cmd-doctor]]: the `nself doctor` command reference and all diagnostic checks
- [[Commands]]: full command index

← [[Home]]
