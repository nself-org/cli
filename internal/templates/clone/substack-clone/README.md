# substack-clone

Newsletter publishing starter built with nSelf.

## What you get

- `np_newsletters`, `np_posts`, `np_subscribers`, `np_tiers` tables with RLS
- Hasura permissions and relationships
- Backend scaffold only; no frontend starter is bundled.
- Required plugins: `cms`, `notify`, `auth`

## Getting started

```bash
nself plugin install cms notify auth
nself start
nself db migrate
nself db hasura metadata apply
# Add a frontend service with nself add, or connect your existing React app.
```
