# notion-clone

Collaborative workspace starter built with nSelf.

## What you get

- `np_workspaces`, `np_pages`, `np_blocks`, `np_page_members` tables with RLS
- Hasura permissions and relationships
- Backend scaffold only; no frontend starter is bundled.
- Required plugins: `cms`, `auth`, `realtime`

## Getting started

```bash
nself plugin install cms auth realtime
nself start
nself db migrate
nself db hasura metadata apply
# Add a frontend service with nself add, or connect your existing React app.
```
