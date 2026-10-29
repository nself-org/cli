# airbnb-clone

Rental marketplace starter built with nSelf.

## What you get

- `np_listings`, `np_bookings`, `np_reviews` Postgres tables with RLS
- Hasura permissions and relationships
- Backend scaffold only; no frontend starter is bundled.
- Required plugins: `auth`, `notify`, `photos`

## Getting started

```bash
nself start
nself db migrate
nself db hasura metadata apply
# Add a frontend service with nself add, or connect your existing React app.
```

## Required plugins

Install these via `nself plugin install <name>` (free tier):

- `auth` — user authentication
- `notify` — booking confirmation emails
- `photos` — listing image storage
