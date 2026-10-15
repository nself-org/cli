# airbnb-clone

Rental marketplace starter built with nSelf.

## What you get

- `np_listings`, `np_bookings`, `np_reviews` Postgres tables with RLS
- Hasura permissions and relationships
- Required plugins: `auth`, `notify`, `photos`

## Getting started

No frontend starter is bundled. Connect your own React + Vite or React Native app
to this backend; the former Flutter starter has been removed.

```bash
nself add auth notify photos
nself start
nself db migrate
nself db hasura metadata apply
```

## Required plugins

Install these via `nself add <name>` (free tier):

- `auth` — user authentication
- `notify` — booking confirmation emails
- `photos` — listing image storage
