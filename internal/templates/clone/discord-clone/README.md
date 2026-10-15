# discord-clone

Community chat platform starter built with nSelf.

## What you get

- `np_servers`, `np_channels`, `np_messages`, `np_members`, `np_roles` tables with RLS
- Hasura permissions and relationships
- Required plugins: `chat`, `realtime`, `auth`, `moderation`

## Getting started

No frontend starter is bundled. Connect your own React + Vite or React Native app
to this backend; the former Flutter starter has been removed.

```bash
nself add chat realtime auth moderation
nself start
nself db migrate
nself db hasura metadata apply
```
