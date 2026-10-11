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
| `config-keys.txt` | Names (never values) of the variables in your env cascade files (`.env`, `.env.local`, `.env.secrets`, and the other cascade layers). Build-written and extra env files (below) are scrub inputs only; their names are not listed |
| `docker-compose.yml` | The generated compose file of your project, scrubbed |
| `nginx/` | The generated nginx `*.conf` files, scrubbed (no symlinks, no certificate or key directories, no file that holds a PEM block, 1 MiB per file) |
| `doctor.txt` | Output of `nself doctor`, run with a 120-second timeout. On a timeout the partial output is kept and `manifest.json` says so |
| `logs/` | Always present. Holds `logs/<service>.log`, the last N lines of every service container (running or exited; default 200, set with `--log-lines`), at most 4 MiB per service (the end is kept, and `manifest.json` notes any cut; the log is scrubbed before it is cut). Each container log read has a 60-second limit, and `manifest.json` notes one that ran out, even when it returned part of the log. The directory is empty when no stack is running |
| `manifest.json` | File list, redaction counts per class, and notes (for example "no running stack", a timeout, a truncated log, secret-named values too short to scrub) |

## What is never collected

- `.env*` files as files (only the variable names of the env cascade, in `config-keys.txt`)
- Secret store content or credential metadata
- SSL/TLS certificates and private keys (the `ssl/` directory and similar key directories are skipped)
- License key data from `~/.nself` (the stored key is read only to learn what to scrub; it never enters the archive)
- The contents of compose fragment files, `CS_N_ENV_FILE` files and compose `env_file` files (they are read only to learn what to scrub)

Not every value is hidden. The generated compose file, the nginx files, the doctor output and the logs carry ordinary non-secret values (ports, hostnames, service names). That is intended: support needs them. Only values the scrubber recognizes as secret are replaced.

## How redaction works

The scrubber learns secrets from these inputs, as scrub inputs only (they never enter the archive):

- every env cascade file (`.env`, `.env.local`, `.env.secrets`, `.env.<environment>`, and the other layers)
- every `.env.*` file in the project directory (a stack started with `ENV=demo` leaves `.env.demo`), `.nself/compose.env` and `.env.computed`, the interpolation files `nself build` writes. Their values are scrub inputs only; their variable names never reach `config-keys.txt`
- the environment of the process that runs the report (the doctor run inherits it)
- the compose files listed in `.nself/compose-files.txt` (plugin fragments and the user override)
- the files named by `CS_N_ENV_FILE` (custom services). Every value seen for a key in any env layer or in the process environment is read, not only the one the build picks. Paths must stay inside the project directory
- the files named by `env_file:` in the generated `docker-compose.yml` and in every listed fragment (a string, a list of strings or a list of `{path: ...}` entries). A relative path is resolved from the directory of the compose file that names it and from the project directory, and every file that resolves is read. The file must be a regular file of at most 1 MiB inside the project directory. A fragment outside the project (a plugin fragment) may also use files inside its own directory
- the `environment`, `args` and `labels` entries of those compose files, in the spelling the container sees as well as the file spelling: `$$` becomes `$`, the defaults of `${NAME:-default}`, `${NAME-default}` and `${NAME:+alt}` are learned (the innermost literal of nested ones too), and a YAML scalar is learned as written (`0x1F2E...`) and as the YAML decoder prints it. Under a secret-named compose key, `${NAME}` and `$NAME` references are also filled in from the env inputs above (every layer's value of NAME), and each referenced variable's value is learned whatever NAME is called
- the stored license key (`~/.nself/license/key`), `NSELF_PLUGIN_LICENSE_KEY` and `NSELF_LICENSE_KEY_1` to `_10`

The age secret store is not a scrub input. An input file that is missing, outside the project, too large or unreadable is listed as a note in `manifest.json`, and so is an unreadable compose manifest or nginx file. The shell's own `PWD` and `OLDPWD` variables are not learned (they hold a directory, not a secret).

A variable counts as secret-like when its name contains `SECRET`, `PASSWORD`, `PASS`, `TOKEN`, `KEY`, `PRIVATE`, `CREDENTIAL`, `DSN`, `SALT`, `SEED`, `LICENSE` (or `LICENCE`), `COOKIE`, `SESSION`, `SIGNING`, `HTPASSWD`, `BASIC_AUTH`, `PWD`, `WEBHOOK` or `HMAC`, or ends in `_AUTH`, or has `PAT` (personal access token) as a whole word between underscores or dashes (`GITHUB_PAT` and `PAT_TOKEN` count; `PATH`, `PATTERN` and `XPATH` do not). `ENCRYPT` alone does not count. A bare `AUTH` prefix (for example `AUTH_URL`) does not count, because those values are hostnames that routing support needs.

Then every file passes five steps in this order:

1. **Known values.** Each secret-like value of 6 or more characters is replaced by `[REDACTED:<NAME>]` everywhere it appears. The scrubber also looks for its URL-escaped, JSON-escaped, base64 (standard and URL alphabets, padded and unpadded), lowercase hex and shell single-quote forms, and each line of a multi-line value on its own. The password inside any URL-shaped value is learned too, even when it holds `#`, `?`, `/` or a space, along with the base64 form of `user:password` (HTTP Basic). The userinfo spelling of a password (`url.UserPassword`, as the build writes the `*_URLENC` variables) and the `\/` spelling of a JSON log are covered. A token used as the user of a URL (`https://<token>@host`) is learned when it is 20 or more characters. An unquoted multi-line PEM in a `.env` file is read through its `-----END` line, so its body lines are scrubbed and never listed as variable names. Overlapping matches become one redaction. A value that equals (ignoring case) a compose service name, a `container_name` or a path segment of an image name of your stack (for example the `postgres` default of `${POSTGRES_PASSWORD:-postgres}`) is public structure: it is not used as a literal, so service, image and log names stay readable, and `manifest.json` says how many values were dropped ("N learned values equal service/image names and were not used as literals"). Steps 4 and 5 still apply to such a value when a secret-like name or pattern surrounds it.
2. **PEM blocks.** Every `-----BEGIN ... -----` block is replaced by `[REDACTED:PEM]`, including a block cut off at either end, a block with a log prefix on each line (the body is recognised by its last word, so a long prefix such as the Postgres timestamp and process id is fine), and a JSON-wrapped block with `\n` escapes.
3. **Passwords in URLs.** `scheme://user:password@host` becomes `scheme://[REDACTED]@host`. The forms `//user:password@host` and `user:password@tcp(host)` are covered too.
4. **Standard patterns.** JWTs, common API key prefixes, bearer tokens, every license key prefix the CLI accepts (`nself_<product>_...`), the secret path token of Slack, Discord and Telegram webhook URLs, email addresses, phone numbers, IP and MAC addresses and card-like numbers.
5. **Assignments.** Any `NAME=value`, `NAME: value`, `"name": "value"`, `--password value`, `--password=-value`, `Authorization` or `Cookie` header, `?token=` query value or nginx `proxy_set_header` value whose name looks secret-like is redacted, and so is the body of a YAML block scalar (`NAME: |`) under such a name. SQL `PASSWORD '...'` and `IDENTIFIED BY '...'` values and the `user:password` of `curl -u` or `--user` are redacted too.

A value that holds a known secret only in part is not trusted: when a step above replaced only the head of a longer secret-named value, the assignment step redacts the whole token. File names inside the archive (log and nginx entries) go through the scrubber too, then a filter that keeps them path-safe.

`manifest.json` counts each class, so you can see how much was removed.

If a variable has a secret-like name but its value is under 6 characters, the value cannot be scrubbed as a literal (it would damage the output). `manifest.json` then lists the variable names (never the values) of the env cascade files and the compose files so you can check those by hand. Short values in the other env files (`.nself/compose.env`, `.env.computed`, other `.env.*`) are not listed, because their names stay out of the bundle.

## Without a running stack

If no ɳSelf stack is running, the report still succeeds:

- The `logs/` directory is present and empty
- `manifest.json` explains why (for example "no running stack: ... not found in $PATH")
- All other files are included

## Limits (read this before you share)

The scrubber removes the values and patterns it knows. It does not understand your data. It can miss:

- a secret stored under a variable name outside the list above, or typed by hand into a log or config line with no secret-like name around it
- a secret shorter than 6 characters
- an encoded form it does not generate (best effort: uppercase hex, the YAML `''` spelling of a single quote inside a single-quoted value, `\uXXXX` escapes of non-ASCII characters, base64 inside a larger base64 blob, or a value split across lines by a log shipper)
- a literal value that starts with `$` and looks like a variable reference (`$Name`): it is treated as a reference and not learned (an assignment with a secret-like name still catches it)
- a webhook URL on another service, or a license key held only in a file other than `~/.nself/license/key`
- a secret whose value equals a service, container or image name of your stack (it is public structure, see step 1)
- secrets that live only in the age secret store: the store is not a scrub input
- a compose value interpolated from a variable that is set only in your shell at run time, when the shell was different from the one that runs the report. The process environment of the report run is a scrub input; a different shell's is not

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
