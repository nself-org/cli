# slack-clone

Team messaging starter built with nSelf.

## What you get

- `np_workspaces`, `np_channels`, `np_messages`, `np_threads`, `np_reactions` tables with RLS
- Hasura permissions and relationships
- Backend scaffold only; no frontend starter is bundled.
- Required plugins: `chat`, `livekit`, `realtime`, `auth`, `notify`

## Getting started

```bash
nself plugin install chat livekit realtime auth notify
nself start
nself db migrate
nself db hasura metadata apply
# Add a frontend service with nself add, or connect your existing React app.
```
