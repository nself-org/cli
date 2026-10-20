# discord-clone

Community chat platform starter built with nSelf.

## What you get

- `np_servers`, `np_channels`, `np_messages`, `np_members`, `np_roles` tables with RLS
- Hasura permissions and relationships
- Required plugins: `chat`, `realtime`, `auth`, `moderation`

## Getting started

<!-- Generated from manifest.json by generate_readmes.py. -->

```bash
nself plugin install chat realtime auth moderation
nself build
nself start
nself db migrate up --migration-dir migrations
nself db hasura metadata apply
```
