# substack-clone

Newsletter publishing starter built with nSelf.

## What you get

- `np_newsletters`, `np_posts`, `np_subscribers`, `np_tiers` tables with RLS
- Hasura permissions and relationships
- Required plugins: `cms`, `notify`, `auth`

## Getting started

No frontend starter is bundled. Connect your own React + Vite or React Native app
to this backend; the former Flutter starter has been removed.

```bash
nself add cms notify auth
nself start
nself db migrate
nself db hasura metadata apply
```
