# zoom-clone

Video meeting platform starter built with nSelf.

## What you get

- `np_meetings`, `np_participants`, `np_recordings` tables with RLS
- Hasura permissions and relationships
- Required plugins: `livekit`, `recording`, `auth`, `notify`

## Getting started

<!-- Generated from manifest.json by generate_readmes.py. -->

```bash
nself plugin install livekit recording auth notify
nself build
nself start
nself db migrate up --migration-dir migrations
nself db hasura metadata apply
```
