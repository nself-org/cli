# nself login

<!-- BEGIN PROSE:summary -->
> Log in to your ɳSelf account.
<!-- END PROSE:summary -->

## Synopsis

```
nself login [flags]
```

## Description

<!-- BEGIN PROSE:description -->
`nself login` authenticates the CLI with your ɳSelf account (cloud.nself.org or a self-hosted ɳSelf Cloud instance). On success, a session token is stored in the local credential store (`~/.nself/auth.json`, mode 0600) and used automatically by subsequent commands that require authentication.

Self-hosted users with no cloud account can skip this step, the CLI operates without a cloud login for local stack management. `nself login` is required for:

- Accessing `cloud.nself.org` to provision or manage hosted stacks
- Syncing license keys from your account
- Pushing plugin submissions via `nself plugin submit`

### Auth host

The CLI talks to the nSelf auth server at `https://auth-server.nself.org`. Set `NSELF_AUTH_SERVER_URL` to use another host (a local stack or a self-hosted auth server); a trailing slash is ignored.

### Device flow

1. The CLI calls `POST /auth/device-code` and prints the login code and the verification URL (`verification_uri`).
2. You open the URL, sign in and approve the code in the browser.
3. The CLI polls `POST /auth/device-code/token` every 5 seconds. `authorization_pending` keeps waiting; `slow_down` adds 5 seconds to the interval; `expired_token` and `access_denied` stop the login with a message (run `nself login` again).
4. On approval the server returns a device token (90 days). The CLI reads your account (`GET /auth/session`) for the email and tier, then stores both in `~/.nself/auth.json`. If that lookup fails, the login fails and nothing is stored.

The account commands `nself account devices` and `nself account team` call `/account/devices` and `/account/team`, which the auth server does not provide. They fail with `E225` (exit 2) instead of a raw 404.
<!-- END PROSE:description -->

## Flags

<!-- BEGIN GENERATED:flags -->
| Flag | Default | Description |
|------|---------|-------------|
| `--force` | `false` | Log in even if already logged in (replaces existing session) |
| `--no-browser` | `false` | Print the login URL instead of opening a browser |
| `--help`, `-h` | — | Show help |
<!-- END GENERATED:flags -->

## Examples

<!-- BEGIN PROSE:examples -->
```bash
# Interactive login (browser-based OAuth)
nself login

# Print the login URL instead of opening a browser (e.g. over SSH)
nself login --no-browser

# Log in again even though a session already exists
nself login --force
```
<!-- END PROSE:examples -->

## See Also

<!-- BEGIN PROSE:see-also -->
- [[cmd-logout]], Log out and clear session
- [[cmd-license]], Manage plugin license keys
- [[cmd-version]], Show CLI version and build info
<!-- END PROSE:see-also -->

← [[Commands]] | [[Home]] →
