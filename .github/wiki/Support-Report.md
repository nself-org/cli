# Support Report

Build a redacted support bundle you can attach to a GitHub issue or send to support.

The `nself doctor report` subcommand creates a tar.gz file with diagnostics and configuration. The command removes known secret values and known secret patterns from every file. It cannot promise to find every secret, so you must still review the archive before you share it (see [Limits](#limits-read-this-before-you-share)).

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

By default the report is saved as `nself-report-<UTC timestamp>.tar.gz` in the project directory (the nearest parent directory that holds your nself project, whatever directory you run the command from) with mode `0600` (readable only by you). A relative `--output` path is resolved against the directory you run the command from. The path is printed on exit. If a file with that name exists, the command exits with an error and does not overwrite it. Nothing is uploaded.

> **Warning:** the default location is inside your project and nself does not add `nself-report-*.tar.gz` to `.gitignore`. Add that line yourself, or use `--output` to write the archive outside the repository, so you never commit a report by accident.

## What is collected

| File | Contents |
|------|----------|
| `versions.json` | nself version, commit, build date, Go version, platform (OS/arch), and the Docker server version when Docker answers |
| `config-keys.txt` | Names (never values) of the variables in your env files (`.env`, `.env.local`, `.env.secrets`, and the other env cascade files) |
| `docker-compose.yml` | The generated compose file of your project, scrubbed |
| `nginx/` | The generated nginx `*.conf` files, scrubbed (no symlinks, no certificate or key directories, no file that holds a PEM block, 1 MiB per file) |
| `doctor.txt` | Output of `nself doctor`, run with a 120-second timeout. On a timeout the partial output is kept and `manifest.json` says so |
| `logs/` | Always present. Holds `logs/<service>.log`, the last N lines of every service container (running or exited; default 200, set with `--log-lines`), at most 4 MiB per service (the end is kept, and `manifest.json` notes any cut). The directory is empty when no stack is running |
| `manifest.json` | File list, redaction counts per class, and notes (for example "no running stack", a timeout, a truncated log, secret-named values too short to scrub) |

## What is never collected

- `.env*` files as files (only their variable names, in `config-keys.txt`)
- Secret store content or credential metadata
- SSL/TLS certificates and private keys (the `ssl/` directory and similar key directories are skipped)
- License key data from `~/.nself`
- The contents of compose fragment files and `CS_N_ENV_FILE` files (they are read only to learn what to scrub)

Not every value is hidden. The generated compose file, the nginx files, the doctor output and the logs carry ordinary non-secret values (ports, hostnames, service names). That is intended: support needs them. Only values the scrubber recognizes as secret are replaced.

## How redaction works

The scrubber learns secrets from these inputs, as scrub inputs only (they never enter the archive):

- every env cascade file (`.env`, `.env.local`, `.env.secrets`, `.env.<environment>`, and the other layers)
- the environment of the process that runs the report (the doctor run inherits it)
- the compose files listed in `.nself/compose-files.txt` (plugin fragments and the user override)
- the files named by `CS_N_ENV_FILE` (custom services); paths must stay inside the project directory
- the `environment`, `args` and `labels` entries of those compose files

A variable counts as secret-like when its name contains `SECRET`, `PASSWORD`, `PASS`, `TOKEN`, `KEY`, `PRIVATE`, `CREDENTIAL`, `DSN`, `SALT`, `SEED`, `LICENSE` (or `LICENCE`), `COOKIE`, `SESSION`, `SIGNING`, `HTPASSWD` or `BASIC_AUTH`, or ends in `_AUTH`. A bare `AUTH` prefix (for example `AUTH_URL`) does not count, because those values are hostnames that routing support needs.

Then every file passes five steps in this order:

1. **Known values.** Each secret-like value of 6 or more characters is replaced by `[REDACTED:<NAME>]` everywhere it appears. The scrubber also looks for its URL-escaped, JSON-escaped, base64 (standard and URL alphabets, padded and unpadded), lowercase hex and shell single-quote forms, and each line of a multi-line value on its own. The password inside any URL-shaped value is learned too, even when it holds `#`, `?`, `/` or a space, along with the base64 form of `user:password` (HTTP Basic). Overlapping matches become one redaction.
2. **PEM blocks.** Every `-----BEGIN ... -----` block is replaced by `[REDACTED:PEM]`, including a block cut off at either end, a block with a log prefix on each line, and a JSON-wrapped block with `\n` escapes.
3. **Passwords in URLs.** `scheme://user:password@host` becomes `scheme://[REDACTED]@host`. The forms `//user:password@host` and `user:password@tcp(host)` are covered too.
4. **Standard patterns.** JWTs, common API key prefixes, bearer tokens, email addresses, phone numbers, IP and MAC addresses and card-like numbers.
5. **Assignments.** Any `NAME=value`, `NAME: value`, `"name": "value"`, `--password value`, `Authorization` or `Cookie` header, `?token=` query value or nginx `proxy_set_header` value whose name looks secret-like is redacted, and so is the body of a YAML block scalar (`NAME: |`) under such a name.

`manifest.json` counts each class, so you can see how much was removed.

If a variable has a secret-like name but its value is under 6 characters, the value cannot be scrubbed as a literal (it would damage the output). `manifest.json` then lists the variable names (never the values) so you can check those by hand.

## Without a running stack

If no ɳSelf stack is running, the report still succeeds:

- The `logs/` directory is present and empty
- `manifest.json` explains why (for example "no running stack: ... not found in $PATH")
- All other files are included

## Limits (read this before you share)

The scrubber removes the values and patterns it knows. It does not understand your data. It can miss:

- a secret stored under a variable name outside the list above, or typed by hand into a log or config line with no secret-like name around it
- a secret shorter than 6 characters
- an encoded form it does not generate (for example base64 inside a larger base64 blob, or a value split across lines by a log shipper)
- secrets that live only in the age secret store: that store is read at deploy time, not at build time, so it is not a scrub input

Review the archive before you share it:

```bash
# List the contents
tar -tzf nself-report-*.tar.gz

# Extract to review
mkdir -p review-report
tar -xzf nself-report-*.tar.gz -C review-report

# Search for a value you know is secret (type it, do not paste it into a shared terminal log)
grep -rF 'your-known-secret' review-report
```

Read `config-keys.txt` and `manifest.json` first, then skim the logs. The archive file name is not ignored by git: keep it out of your repository.

If you find any unredacted sensitive value, do not share the file. Open an issue describing the class of leak (never paste the actual value).

## See Also

- [[cmd-doctor]]: the `nself doctor` command reference and all diagnostic checks
- [[Commands]]: full command index

← [[Home]]
