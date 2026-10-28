# Multi-App Isolation

Several apps can share one nSelf backend and still never see each other's rows. The `multi-app` template is the reference for how: copy its table shape and permission filter for your own tables.

```bash
nself init --non-interactive
nself init --template multi-app . --force   # overlay the template files
```

The overlay replaces the `.env.example` and `nself.yml` that `nself init` wrote with the template's own. Keep your own values in `.env` and `.env.local`.

`--dry-run` lists what it would write: `README.md`, `manifest.json`, `nself.yml`, `.env.example`, `migrations/001_multi_app.sql` and `hasura/metadata/databases/default/tables/tables.yaml` (the layout `nself db reconcile` reads). No plugins are required, and there is no UI starter.

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

## Checking isolation

```bash
nself doctor --deep --only isolation          # human report
nself doctor --deep --only isolation --json   # machine report
```

The guard is read-only. It reads the live Hasura permissions (`export_metadata`) and, with `psql` inside the project's postgres container, which tables have `source_account_id` or `tenant_id`. Every role except `admin` is checked on every tracked table of the default Hasura source (`nself_admin` is also skipped on `np_*` tables, where plugin metadata gives it an open filter by design). An inherited role is checked as the `_or` of its members' select filters, so a member with an open filter shows up on every role that inherits it. Each finding names the table, the role and the operation.

| Rule | Severity | What it flags |
|---|---|---|
| ISO-01 | warn (fail in strict mode) | A tracked app table without `source_account_id` while another app table has it. Skips `auth`, `storage`, `hdb_catalog` and `np_*`. |
| ISO-02 | fail | A table with `source_account_id` where a role's select, update or delete filter does not pin `source_account_id = X-Hasura-Source-Account-Id`, or its insert `check`/`set` does not. An update whose `check` or preset does not pin the column also fails while the column is writable (listed in `columns`, missing from the permission, or assigned by a preset). Skips `np_*`, `auth`, `storage`, `hdb_catalog`. |
| ISO-03 | fail | A table with `tenant_id` where a role's permission does not pin `tenant_id = X-Hasura-Tenant-Id`. Covers every tracked table, including `np_*` and `np_cloud_*`. |
| ISO-04 | fail | The interchange: `source_account_id` compared to `X-Hasura-Tenant-Id`, or `tenant_id` to `X-Hasura-Source-Account-Id`, in a filter, check or preset. Covers every tracked table. |

A filter pins a column only when it holds for every row it admits: an `_and` branch, an implicit AND, or every branch of an `_or`. `_not`, `_exists`, an empty filter `{}` and a missing permission do not pin. Session variable names compare case-insensitively.

Strict mode (`NSELF_DOCTOR_STRICT=1`) turns the ISO-01 warning into a fail. A project where no table has either column gets one pass, plus a failure only when the metadata or database cannot be read or bound. A second Hasura source with tracked tables is reported as a failure: its columns live in another database, so the guard cannot check it. So is a default source the guard cannot prove reads the project's own database: it must use nSelf's generated `HASURA_GRAPHQL_DATABASE_URL` reference (the guard trusts that variable, since nSelf generates it, and does not read the running container's environment), or a literal URL for the project's postgres host and database with no query parameters or fragment; any other host, port, database, variable, connection shape, read replica, connection template or second `default` source fails. So is a tracked table the project database does not have, and, when any relation in the database has an isolation column (tracked or not), a default source that exposes logical models, native queries or stored procedures, or a tracked function that is volatile in postgres and callable by a non-admin role (explicitly, or through the permissions Hasura infers for a query function) (these read or write outside the table permissions). Another source that has tables, native queries, logical models, stored procedures or functions is reported too. An inherited role is checked from its members: the `_or` of their select filters, and each member's insert, update and delete permission as role `name (via member)`. Only `_eq`, `_and` and `_or` shapes count as pinning (Hasura's `$`-aliases fail closed). A tracked stable function callable by a non-admin role gets a warning, because its body may read walled tables and metadata cannot show it. Computed fields, actions and remote schemas are outside the table-permission model and are not checked. A stopped stack, an unreadable config or an unreachable postgres container also gives a failure in this section, so a plain `nself doctor --deep` on a stopped project shows it too. If the metadata or the columns cannot be read (stack stopped, container unreachable), the section reports one failure and never a pass.

`PERM-HASURA-01` (the `hasura/metadata` files, `np_*` tables, the filter, check and preset of any role's select, insert, update and delete permissions) also rejects the interchange: a warning in v1.4 mode, a failure in v1.5 mode.

`scripts/e2e/isolation-guard.sh` proves it on Linux with the multi-app template: zero failures on the template, then a failure naming `public.notes`, `user`, `select` and ISO-02 after the admin API replaces the notes select filter with `{}`.

## Proof

`scripts/e2e/multi-app-isolation.sh` runs on Linux against a real stack (init, overlay, build, start, migrate, reconcile) with fake credentials. It checks:

- For both `notes` and `projects`: a user in `app_a` reads its 3 rows and none of the 2 `app_b` rows.
- A real sign-up for `app_b` without a membership row reads zero rows and cannot insert.
- A forged `app_b` token for a non-member reads nothing.
- Inserts get `source_account_id=app_a` from the token; sending the field is refused.
- A member of `app_b` updates or deletes 0 `app_a` rows and cannot move a row between apps.
- `app_members` is not reachable by the `user` role.

The script mints HS256 tokens with `openssl` from `nself config get HASURA_JWT_KEY --reveal`. The key stays in one shell variable and is never printed or put on a command line.
