# notion-clone

Collaborative workspace starter built with nSelf.

## What you get

- `np_workspaces`, `np_pages`, `np_blocks`, `np_page_members` tables with RLS
- Hasura permissions and relationships
- Required plugins: `cms`, `auth`, `realtime`

## Getting started

No frontend starter is bundled. Connect your own React + Vite or React Native app
to this backend; the former Flutter starter has been removed.

```bash
nself add cms auth realtime
nself start
nself db migrate
nself db hasura metadata apply
```
