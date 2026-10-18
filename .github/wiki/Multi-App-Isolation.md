# Multi-App Isolation

Several apps can share one nSelf backend and still never see each other's rows. The `multi-app` template is the reference for how: copy its table shape and permission filter for your own tables.

```bash
nself init --non-interactive
nself init --template multi-app . --force   # overlay the template files
```

The overlay replaces the `.env.example` and `nself.yml` that `nself init` wrote with the template's own. Keep your own values in `.env` and `.env.local`.

`--dry-run` lists what it would write: `README.md`, `manifest.json`, `nself.yml`, `.env.example`, `migrations/001_multi_app.sql` and `hasura/metadata/databases/default/tables/tables.yaml` (the layout `nself db reconcile` reads). There is no `hasura/metadata.json`: `nself db hasura metadata apply` would read it and replace the whole metadata document, which untracks the hasura-auth tables. No plugins are required, and there is no UI starter.

## The model

| Piece | Role |
|---|---|
| `source_account_id` | Which app a row belongs to. Text, defaults to `primary`, set from the token on insert, never from the client. |
| `public.app_members` | `(user_id, source_account_id)`. Says which app a user belongs to (one app per user). Only the admin secret writes it; the Hasura `user` role has no permission on it. |
| `public.notes`, `public.projects` | Example per-app tables (`projects` also has a `name`). |

`source_account_id` separates apps that share a deploy. It is not `tenant_id`, which separates paying customers; never interchange them.

**One identity pool, one app per user.** All apps share one hasura-auth user table. An email can sign up once, so the same email cannot join a second app, and signup for app B reports "email already in use" for an address registered in app A. Login does not check the app: app A credentials entered in app B's login form return an app A token, which reads nothing of app B. Rows inserted by the admin secret without the column land in `primary`.

## The permission filter

Every `user` permission on `notes` and `projects` uses this filter (insert uses it as `check`, update also as its post-update check):

```json
{"_and": [
  {"source_account_id": {"_eq": "X-Hasura-Source-Account-Id"}},
  {"_exists": {"_table": {"schema": "public", "name": "app_members"},
    "_where": {"user_id": {"_eq": "X-Hasura-User-Id"},
               "source_account_id": {"_eq": "X-Hasura-Source-Account-Id"}}}}
]}
```

The first clause matches the row to the token's app. The second needs a membership row, so a token that merely claims an app id gets nothing. Without it, anyone could sign up with `options.metadata.source_account_id` set to another app's id and read that app's data.

Also enforced:

- Insert presets `source_account_id` from the token; the client cannot send it.
- Update cannot change `source_account_id`, and the new row must still pass the filter.
- Delete uses the same filter.

## Setup

1. Add the `AUTH_JWT_CUSTOM_CLAIMS` and `ENV_ALLOWLIST` lines from `.env.example` to `.env.local`. The first makes hasura-auth copy the user's `metadata.source_account_id` into the `x-hasura-source-account-id` claim.
2. Run the template's next steps. The migration references `auth.users`, so hasura-auth must be running: `nself start` must have started it before `nself db migrate up` runs:

```bash
nself build
nself start
nself db migrate up --migration-dir migrations
nself db reconcile --apply
```

Plain `nself db reconcile` is a dry run: it prints the plan and writes nothing. Reconcile sends targeted per-object calls in one atomic bulk request. It never replaces the whole Hasura metadata document (`replace_metadata`), so hasura-auth's own tables stay tracked. Do not use `nself db hasura metadata apply` with this template.

3. Sign users up with their app id, then add the membership with the admin secret:

```bash
AUTH_URL=http://localhost:4000   # your AUTH_PORT
curl -fsS "$AUTH_URL/signup/email-password" -H 'Content-Type: application/json' \
  -d '{"email":"user@example.com","password":"...","options":{"metadata":{"source_account_id":"app_a"}}}'
```

```sql
INSERT INTO public.app_members (user_id, source_account_id) VALUES ('<user uuid>', 'app_a');
```

## Proof

`scripts/e2e/multi-app-isolation.sh` runs on Linux against a real stack (init, overlay, build, start, migrate, reconcile) with fake credentials. It checks:

- For both `notes` and `projects`: a user in `app_a` reads its 3 rows and none of the 2 `app_b` rows.
- A real sign-up for `app_b` without a membership row reads zero rows and cannot insert.
- A forged `app_b` token for a non-member reads nothing.
- Inserts get `source_account_id=app_a` from the token; sending the field is refused.
- A member of `app_b` updates or deletes 0 `app_a` rows and cannot move a row between apps.
- `app_members` is not reachable by the `user` role.

The script mints HS256 tokens with `openssl` from `nself config get HASURA_JWT_KEY --reveal`. The key stays in one shell variable and is never printed or put on a command line.
