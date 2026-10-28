# discord-clone

Community chat platform starter built with nSelf.

## What you get

- `np_servers`, `np_channels`, `np_messages`, `np_members`, `np_roles` tables with RLS
- Hasura permissions and relationships
- Backend scaffold only; no frontend starter is bundled.
- Required plugins: `chat`, `realtime`, `auth`, `moderation`

## Getting started

```bash
nself plugin install chat realtime auth moderation
nself start
nself db migrate
nself db hasura metadata apply
# Add a frontend service with nself add, or connect your existing React app.
```
