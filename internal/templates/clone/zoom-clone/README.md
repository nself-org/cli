# zoom-clone

Video meeting platform starter built with nSelf.

## What you get

- `np_meetings`, `np_participants` tables with RLS
- Hasura permissions and relationships
- Flutter starter with meeting lobby, and video grid
- Required plugins: `livekit`, `auth`, `notify`

## Getting started

```bash
nself plugin install livekit auth notify
nself start
nself db migrate
nself hasura metadata apply --file hasura/metadata.json
cd flutter && flutter run
```
