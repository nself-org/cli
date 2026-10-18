# AI Plugin: Gemini OAuth Re-Authentication

> Reference for re-authorizing Gemini accounts in the GFP (Gemini Free Pool) after token expiry, scope changes, or account rotation.

The Gemini Free Pool manages OAuth-connected Google accounts to provide free or near-free Gemini API capacity. Tokens expire, scopes sometimes change, and accounts occasionally need rotation. This page explains when re-auth is needed and how to do it.

## When Re-Auth Is Needed

Four situations require re-authenticating a Gemini account:

**Token expiry.** Google OAuth refresh tokens can be revoked by Google after 6 months of inactivity, after a password change, or when the account exceeds the concurrent-session limit. The pool detects this on the next key test and marks the key as `expired`.

**Scope change.** If the required OAuth scopes for the Gemini API change (for example, adding `https://www.googleapis.com/auth/generative-language`), existing tokens may not carry the new scope. The pool marks affected keys as `scope_mismatch`.

**Account rotation.** You may want to remove a compromised or cancelled Google account from the pool and replace it with a new one.

**`invalid_grant` errors.** A 401 response with `error: invalid_grant` from Google's token endpoint means the refresh token is no longer valid. The pool logs this and marks the key `invalid`.

To check current pool status and see which keys need attention:

```bash
nself ai pool status --verbose
```

## Step-by-Step: Re-Authorizing a Key

### 1. Identify the affected account

```bash
nself ai pool status --verbose --json
```

Look for keys with `status: expired`, `status: invalid`, or `status: scope_mismatch`. Note the `account` field (Google account email) and `key_id`.

### 2. Remove the stale key

```bash
nself ai pool remove --account user@example.com
```

Or by key index:

```bash
nself ai pool remove --key-id 3
```

This soft-revokes the key in the pool and optionally deletes the GCP API key (you are prompted). It does not delete the Google account.

### 3. Re-authorize the account

```bash
nself ai pool init --oauth
```

This opens your browser to a Google OAuth consent page. Sign in with the same (or a new) Google account. After consent, the pool:

1. Stores the refresh token locally (encrypted via `nself secrets`)
2. Creates a new GCP project under the account (or reuses an existing one)
3. Enables the Generative Language API
4. Issues a new API key
5. Registers the key in the pool with status `active`

The `--oauth` flag forces the browser-based flow. Without it, `nself ai pool init` uses any stored credentials first.

### 4. Verify the new key

```bash
nself ai pool test --all
```

Or test just the new key:

```bash
nself ai pool test --key-id <new-id>
```

A passing test sends a 1-token Gemini request and confirms the key is live.

## How Token Rotation Works Internally

The pool runs a background goroutine that refreshes tokens before they expire. The rotation schedule:

- **Active check interval:** every 12 hours
- **Token refresh window:** 7 days before expiry
- **Quota reset trigger:** daily at midnight UTC (`nself ai pool daily-reset`)
- **Failure threshold:** 3 consecutive failures on a key mark it `degraded`; 5 failures mark it `expired` and alert via the `notify` plugin (if installed)

Token storage uses AES-256-GCM encryption via the nself secrets manager. Refresh tokens never appear in logs or the `pool status` output (only the first 8 characters of the key ID are shown).

When `nself build` runs, the pool configuration is written to the AI plugin's environment as `GFP_POOL_CONFIG` (base64-encoded JSON). The plugin reads this at startup and manages the active key selection. If all pool keys are exhausted for the day, the plugin falls back to `GEMINI_API_KEY` (the static key in `.env.dev`), then to Ollama if configured.

## Troubleshooting

### `invalid_grant`

The refresh token is revoked. Follow the re-auth steps above. Common causes: Google account password changed, account security event, or the token was unused for 6 months.

```
error: token refresh failed for user@example.com: invalid_grant
```

Fix: `nself ai pool remove --account user@example.com` then `nself ai pool init --oauth`.

### `401 Unauthorized` on inference requests

The API key itself may be deleted on the GCP side (not the OAuth token). Check the GCP console for the project associated with the account.

Fix: `nself ai pool remove --key-id <id>` then `nself ai pool init --oauth` to issue a fresh key.

### `quota_exceeded` immediately after re-auth

The new account's free-tier quota is already exhausted (possible if the GCP project was reused and had prior usage). Add a different Google account or wait for the daily quota reset.

```bash
nself ai pool daily-reset --dry-run   # check what would reset
nself ai pool daily-reset             # reset counters
```

### Pool shows `scope_mismatch`

Run `nself ai pool init --oauth` for the affected account. The OAuth consent page will request the updated scopes. Accept them. The pool updates the stored token automatically.

### `nself ai pool init --oauth` opens no browser

The CLI tries `xdg-open` (Linux) or `open` (macOS). If neither works, the command prints the authorization URL for manual opening:

```
Open this URL in your browser to continue:
https://accounts.google.com/o/oauth2/auth?...
```

Copy and open it. After authorizing, paste the resulting code at the prompt.

## Verifying the Pool Is Healthy Post-Re-Auth

```bash
nself ai pool status --verbose
```

All keys should show `status: active` and a `last_tested` timestamp within the past hour. Quota counters should be below their daily limits.

Run a quick end-to-end smoke test:

```bash
nself ai chat "hello" --model gemini-1.5-flash
```

A successful response confirms the pool is routing correctly.

## See Also

- [[cmd-ai]], full `nself ai` command reference, including `pool` subcommands
- [[cmd-oauth]], `nself oauth refresh` for non-Gemini OAuth token refresh
- `plugin-nself-ai-gateway`, AI gateway configuration and supported providers

← `plugin-nself-ai-gateway` | [[Home]] →
