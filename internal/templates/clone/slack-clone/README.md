# slack-clone

Team messaging starter built with nSelf.

## What you get

- `np_workspaces`, `np_channels`, `np_messages`, `np_threads`, `np_reactions` tables with RLS
- Hasura permissions and relationships
- Required plugins: `chat`, `livekit`, `realtime`, `auth`, `notify`

## Getting started

<!-- Generated from manifest.json by generate_readmes.py. -->

```bash
nself plugin install chat livekit realtime auth notify
nself build
nself start
nself db migrate up --migration-dir migrations
nself db hasura metadata apply
```
