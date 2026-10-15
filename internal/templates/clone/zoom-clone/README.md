# zoom-clone

Video meeting platform starter built with nSelf.

## What you get

- `np_meetings`, `np_participants`, `np_recordings` tables with RLS
- Hasura permissions and relationships
- Required plugins: `livekit`, `recording`, `auth`, `notify`

## Getting started

No frontend starter is bundled. Connect your own React + Vite or React Native app
to this backend; the former Flutter starter has been removed.

```bash
nself add livekit recording auth notify
nself start
nself db migrate
nself db hasura metadata apply
```
