# airbnb-clone

Rental marketplace starter built with nSelf.

## What you get

- `np_listings`, `np_bookings`, `np_reviews` Postgres tables with RLS
- Hasura permissions and relationships
- Required plugins: `auth`, `notify`, `photos`

## Getting started

<!-- Generated from manifest.json by generate_readmes.py. -->

```bash
nself plugin install auth notify photos
nself build
nself start
nself db migrate up --migration-dir migrations
nself db hasura metadata apply
```
