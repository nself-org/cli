# slack-clone

Team messaging starter built with nSelf.

## What you get

- `np_workspaces`, `np_channels`, `np_messages`, `np_threads`, `np_reactions` tables with RLS
- Hasura permissions and relationships
- Required plugins: `chat`, `livekit`, `realtime`, `auth`, `notify`

## Getting started

No frontend starter is bundled. Connect your own React + Vite or React Native app
to this backend; the former Flutter starter has been removed.

```bash
nself add chat livekit realtime auth notify
nself start
nself db migrate
nself db hasura metadata apply
```
