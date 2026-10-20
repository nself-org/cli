# notion-clone

Collaborative workspace starter built with nSelf.

## What you get

- `np_workspaces`, `np_pages`, `np_blocks`, `np_page_members` tables with RLS
- Hasura permissions and relationships
- Required plugins: `cms`, `auth`, `realtime`

## Getting started

<!-- Generated from manifest.json by generate_readmes.py. -->

```bash
nself plugin install cms auth realtime
nself build
nself start
nself db migrate up --migration-dir migrations
nself db hasura metadata apply
```
