# zoom-clone

Video meeting platform starter built with nSelf.

## What you get

- `np_meetings`, `np_participants`, `np_recordings` tables with RLS
- Hasura permissions and relationships
- Backend scaffold only; no frontend starter is bundled.
- Required plugins: `livekit`, `recording`, `auth`, `notify`

## Getting started

```bash
nself plugin install livekit recording auth notify
nself start
nself db migrate
nself db hasura metadata apply
# Add a frontend service with nself add, or connect your existing React app.
```
