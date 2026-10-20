# substack-clone

Newsletter publishing starter built with nSelf.

## What you get

- `np_newsletters`, `np_posts`, `np_subscribers`, `np_tiers` tables with RLS
- Hasura permissions and relationships
- Required plugins: `cms`, `notify`, `auth`

## Getting started

<!-- Generated from manifest.json by generate_readmes.py. -->

```bash
nself plugin install cms notify auth
nself build
nself start
nself db migrate up --migration-dir migrations
nself db hasura metadata apply
```
