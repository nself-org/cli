# multi-app

Two apps, one backend. Every row carries a `source_account_id`; a user reads
and writes only rows of the app named in their token, and only if the server
lists them as a member of that app.

## What you get

- `public.notes` and `public.projects`: example per-app tables (`source_account_id` defaults to `primary`).
- `public.app_members` `(user_id, source_account_id)`: who belongs to which app. Server-controlled: the Hasura `user` role has no permission on it, so only the admin secret writes it.
- Hasura permissions for role `user` on `notes` and `projects` (select, insert, update, delete).
- No plugins are required.

## How the wall works

Every `user` permission uses this filter (insert uses it as `check`, update also as its post-update check):

```json
{"_and": [
  {"source_account_id": {"_eq": "X-Hasura-Source-Account-Id"}},
  {"_exists": {"_table": {"schema": "public", "name": "app_members"},
    "_where": {"user_id": {"_eq": "X-Hasura-User-Id"},
               "source_account_id": {"_eq": "X-Hasura-Source-Account-Id"}}}}
]}
```

The app id in the token alone is not enough: a user who signs up with another
app's id and has no `app_members` row reads and inserts nothing. Inserts set
`source_account_id` from the token, so a client cannot choose it. Updates
cannot change `source_account_id`.

## Requirements

The migration references `auth.users`, so hasura-auth must be running: `nself start` must have
started it once before `nself db migrate up`.

## Claim setup

Copy the `AUTH_JWT_CUSTOM_CLAIMS` line from `.env.example` into `.env.local`.
hasura-auth then puts the user's `metadata.source_account_id` into the
`x-hasura-source-account-id` claim. Sign users up with it:

```json
{"email": "user@example.com", "password": "...", "options": {"metadata": {"source_account_id": "app_a"}}}
```

## Getting started

```bash
nself build
nself start
nself db migrate up --migration-dir migrations
nself db reconcile            # dry run: prints the plan
nself db reconcile --apply    # pushes it
```

Reconcile sends targeted per-object calls and never replaces the whole Hasura
metadata document, so hasura-auth's own tables stay tracked. Do not run
`nself db hasura metadata apply` here. The metadata lives in
`hasura/metadata/databases/default/tables/tables.yaml`; `hasura/metadata.json`
holds the same tables.

Add a member with the admin secret:

```sql
INSERT INTO public.app_members (user_id, source_account_id) VALUES ('<user uuid>', 'app_a');
```

## Queries

As a member of `app_a`:

```graphql
query { notes { id title source_account_id } }
mutation { insert_notes_one(object: {title: "hello"}) { id source_account_id } }
```

Full walk-through: `.github/wiki/Multi-App-Isolation.md` in the nSelf CLI repo.
