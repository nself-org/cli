# Update Notifications

nself can print one line on stderr when a newer CLI release exists. It is **off by default**: nothing is sent and nothing is printed until you opt in.

---

## Turn it on

```bash
export NSELF_UPDATE_CHECK=1
```

Only the value `1` enables it. Put the line in your shell profile to keep it.

## What you see

After a command succeeds, at most once every 24 hours:

```
A newer nself is available (current 1.4.12, latest 1.5.0). Run `nself update` to upgrade.
```

`nself update --check` is unchanged and still reads GitHub releases. Use it for the full answer.

The hint is never printed when any of these is true:

- `NSELF_UPDATE_CHECK` is not `1`
- the command failed
- JSON mode is on (`--json`, or `--format json` on a command that supports it)
- stderr is not a terminal (pipes, redirects, cron)
- a CI marker is set: `CI`, `GITHUB_ACTIONS`, `GITLAB_CI`, `BUILDKITE` or `JENKINS_URL` (`CI=false` and `CI=0` do not count)
- the command is `nself update` itself
- the cached latest version is empty, not a plain `MAJOR.MINOR.PATCH` release, or not newer than yours

It writes to stderr only. stdout is never touched.

## What is sent

When opted in and the cache is older than 24 hours, a background goroutine makes one request:

```
GET https://ping.nself.org/version
User-Agent: nself/<your version>
```

There are no other headers (compression is off, so there is no `Accept-Encoding`). No account, project, licence key or machine id is included. The request has a 250 ms budget. A slow or failing network never delays the command and never prints anything; the goroutine is simply abandoned when the command ends.

The ping handler stores nothing. ping.nself.org's web proxy does log the client IP address and the `User-Agent` line like any web server. That is the reason the check is opt-in.

## The cache

`~/.nself/cache/update-check.json` (mode 0600):

| Field | Meaning |
|---|---|
| `checked_at` | time of the last refresh attempt, good or failed |
| `latest` | last good answer from ping |
| `last_error` | why the last refresh failed; empty after a good one |
| `hinted_at` | when the hint was last printed |

A failed refresh keeps the old `latest`, records `last_error`, and is not retried for 24 hours. A refresh that is cut short because the command ended first leaves no record, so the next opted-in command retries it. A corrupt file is replaced. Delete the file to reset it.

The hint itself reads only this file. It never touches the network.

## Related

- [[Command-Canon]]
- [[Compat-V15]]
