# Exit Codes

← [[Home]]

---

`nself` exits with one of a small set of statuses so scripts, CI wrappers and agents can branch on the kind of failure without parsing text. The exit class is derived from the error code (see [[error-codes]]); the same number is `error.exit_code` in the JSON error envelope (see [[JSON-Output]]).

> The classes below are v1.5 behaviour. They are on from v1.5.0 and opt-in before that with `NSELF_V15=1` ([[Compat-V15]]). In v1.4 mode every failure exits 1, unless the command sets its own status.

## The contract

| Code | Class | Meaning |
|------|-------|---------|
| 0 | ok | Success. |
| 1 | user | Invalid input or usage. The fix is on the caller's side. This is also the default for any error with no more specific class. |
| 2 | infra | The host, runtime or a dependency failed (Docker down, port taken, database not running, backup failed). Retrying after fixing the host may succeed. |
| 3 | auth | A licence, credential or entitlement failure. |
| 4 | destructive_blocked | A safety gate refused a destructive action. |
| 10-12 | state | Reserved for documented state codes on success paths (v1.5 mode only): 10 unhealthy or failed, 11 transitional, 12 warnings only. |

Signals are not produced by nself; shells report 128 plus the signal number.

## How the status is chosen

In this order, the first rule that matches wins:

1. No error: 0.
2. The command set an explicit status (`errs.Exit`, `errs.ExitWith`, a plugin's own status): that status.
3. The error carries an error code: the **Exit** column of that code in [[error-codes]]. `E400` (unclassified) falls through.
4. The error wraps a known failure (for example Docker not running): the class of its code. When several are joined, the highest class wins: auth, then destructive, then infra, then user.
5. Anything else: 1.

A panic that escapes a command is recovered and reported as `internal error: <value>` with exit 2 (infra, the status a Go crash already gives). The stack trace goes to stderr only, never into the JSON envelope. In v1.4 mode it crashes with the raw Go trace as before.

## State codes

Two commands report the state they found on a success path, in v1.5 mode, with the reserved 10-12 range. The same fact is `data.state` in their JSON envelope (see [[JSON-Output]]).

| Command | Exit | `data.state` | Meaning |
|---------|------|--------------|---------|
| `status` | 0 | `ok` | every service is healthy |
| `status` | 10 | `unhealthy` | a service is unhealthy |
| `status` | 11 | `transitional` | services are still starting |
| `doctor` | 0 | `ok` | every check passed |
| `doctor` | 10 | `unhealthy` | a check failed |
| `doctor` | 12 | `warnings` | warnings only, no failures |

In v1.4 mode the codes stay as before: human `status` exits 2 (unhealthy) or 1 (starting), human `doctor` exits 1 (failed) or 2 (warnings only), and `--json` exits 0 for both.

## Exceptions

- **Plugin passthrough.** A command proxied to a plugin exits with the plugin's own status.
- **Per-command state codes.** Commands that document `exit_codes` in `nself help --json` (`status`, `doctor`) use them on success paths.
- **Silent errors.** When the command already printed its own failure output, nothing more is printed; the status is still chosen as above.

## Output on failure

| Mode | stdout | stderr |
|------|--------|--------|
| Human, v1.5 | whatever the command printed before failing | `Error: [Exxx] message` with indented `Why:`, `Fix:`, `Docs:` lines when the error has a code; otherwise `Error: message` |
| `--json`, v1.5 | exactly one error envelope | the same human rendering |
| v1.4 | whatever the command printed | `Error: message` |

Raw arguments and secrets are never written to either stream; credentials in URLs, tokens and e-mail addresses are masked.

## Changes in v1.5.0

Failures that exited 1 now exit with their class. Only the failures below move; everything else keeps 1. Check any script that tests `$? -eq 1` for these.

| Failure | Code | Exit before | Exit now | Commands that can return it |
|---------|------|-------------|----------|-----------------------------|
| Docker not installed, daemon not running | E001, E002 | 1 | 2 | `start` |
| Compose not found | E004 | 1 | 2 | `start` |
| Port conflict | E005 | 1 | 2 | `start`, `build`, `config validate` |
| mkcert missing, SSL generation failed | E150, E151 | 1 | 2 | `ssl`, `trust`, `start`, `build` |
| Database not running | E200 | 1 | 2 | `db`, `start`, `backup` |
| Migration failed, prerequisite missing | E201, E204 | 1 | 2 | `db migrate` |
| Backup, restore, encrypt, decrypt, remote, prune, WAL, DR failures | E202, E205-E216 | 1 | 2 | `backup`, `db backup`, `db restore` |
| Service unhealthy, health timeout, service not found | E250-E252 | 1 | 2 | `status`, `start`, `health`, `restart` |
| Licence key invalid, tier too low, expired, licence server unreachable | E101-E104 | 1 | 3 | `license`, `plugin install`, `bundle`, `start` |
| Plugin tier not entitled | E110 | 1 | 3 | `plugin install`, `bundle` |
| Destructive action blocked by a safety gate | E403 | 1 | 4 | any command that wraps the gate error |

Failures with exit 1 before and now (config, init, domain, plugin manifest and signature, migration validation, usage) are unchanged. Commands that already returned an explicit status keep it.

State codes (see State codes above):

| Command | Before | Now |
|---------|--------|-----|
| `status --json` | 0 | 10 (unhealthy), 11 (starting) |
| `doctor --json` | 0 | 10 (failed), 12 (warnings only) |
| `status` (human) | 2 unhealthy, 1 starting | 10 unhealthy, 11 starting |
| `doctor` (human) | 1 failed, 2 warnings only | 10 failed, 12 warnings only |

`status --json`, `doctor --json` and `config get|list|show --json` also print the v1 envelope instead of the bare report (status, doctor) or the human text (config) in v1.5 mode (see [[JSON-Output]]).

---

← [[Home]] | [[error-codes]] | [[Compat-V15]]
