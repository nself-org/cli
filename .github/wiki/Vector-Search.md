# Vector search through Hasura

Apps never connect to Postgres. Similarity search follows the same rule: your app sends a GraphQL query to Hasura, and Hasura runs a small SQL function that nself generates for you. The function returns rows of your own table, so the table's select permission filters every row before the app sees it. No new role, grant or connection string is created.

You need a table with a `vector(n)` column, tracked in Hasura, in a project that runs the pgvector image (the default for new projects; see [[pgvector]]).

## Generate the function

```bash
nself db generate vector-search public.docs.embedding
```

In v1.4 mode the same command is `nself generate vector-search public.docs.embedding`.

The argument is `<schema>.<table>.<column>`. nself reads the column type from the running Postgres container, so the stack must be up. It writes one migration:

```
migrations/<timestamp>_vector_search_docs_embedding.sql
```

The migration creates `public.docs_nearest_embedding(query vector(n))`. It is a `STABLE` SQL function that returns `SETOF public.docs` ordered by distance. Options:

| Flag | Meaning |
|---|---|
| `--distance cosine\|l2\|ip` | `cosine` (default) orders by `<=>`, `l2` by `<->`, `ip` (inner product) by `<#>` |
| `--roles user,staff` | Hasura roles that may call the function. Default: `user` |
| `--track` | Track the function in Hasura (see below) |
| `--migration-dir DIR` | Where to write the migration. Default: `./migrations` |
| `--json` | Print one v1 JSON envelope |

Running the command again with the same options reuses the existing migration file. Changing `--distance` writes a new file, because the SQL changes.

The function has no row cap on purpose. A cap inside it would apply before the select permission filter and could return fewer rows than the caller is allowed to see. The caller passes `limit` in GraphQL instead.

## Apply and track

```bash
nself db migrate up --migration-dir migrations
nself db generate vector-search public.docs.embedding --track
```

`--track` sends one targeted Hasura call: track the function, then add a function permission for each role. It never replaces metadata and never runs `nself db hasura metadata apply`. A second run changes nothing.

Before it sends anything, nself checks the live metadata:

- The table must be tracked, and every role in `--roles` must have a select permission on it. Otherwise E514, because a function granted to a role without select would show no rows.
- Every role that has a select permission on the table must see the vector column (`columns: "*"` or a list that includes it). Otherwise E514 naming each role. This applies to roles outside `--roles` too.

The second rule exists because Hasura, by default, infers function permissions from the table's select permissions: the function is visible to every role with select on the table. The function orders by the vector column inside Postgres, where column permissions do not apply, so a role that cannot read `embedding` could still learn it from the result order. nself refuses to track the function until every select role can see the column, and nothing is sent when the check fails. The generator (without `--track`) runs the same check against the on-disk metadata.

`--roles` adds explicit function permissions. They only gate access when Hasura runs with `HASURA_GRAPHQL_INFER_FUNCTION_PERMISSIONS=false`; with the default inference, every select role can call the function.

## On-disk metadata

The generator reads your project's on-disk Hasura metadata to confirm the table is tracked. Without it the command stops with E513. Create it with `nself db hasura metadata export`.

After `--track`, nself adds the function to the on-disk metadata only when that metadata matches live Hasura. If it differs (someone changed permissions in the console, for example), nself tracks the function live, leaves the files alone and prints a note. Run `nself db hasura metadata export` to refresh the files, then commit them.

Two layouts are updated: the `hasura/metadata/sources.yaml` that `nself db hasura metadata export` writes, and the older `hasura/metadata.json` (rewritten as sorted-key JSON with two-space indent). Any other layout is left unchanged.

## Query it

```graphql
query Nearest {
  docs_nearest_embedding(args: { query: "[0.1, 0.2, 0.3]" }, limit: 5) {
    id
    owner
  }
}
```

Send it with a normal user JWT. Hasura applies the `docs` select filter (for example `owner = X-Hasura-User-Id`) to the ordered rows, then applies `limit`. The caller gets only rows they may read, nearest first. A role that has no select permission on the table, including `anonymous`, gets a "field not found" error.

To set a server-side ceiling, put a `limit` on the role's select permission for the table. It also caps this function.

## Limits

- One function per column. Running the command with another `--distance` replaces the function body (`CREATE OR REPLACE`), because the name `<table>_nearest_<column>` and the argument type stay the same.
- The column must be `vector(n)` with a fixed dimension. `halfvec`, `sparsevec` and `text` columns are refused with E513.
- The function name must fit in 63 bytes.
- Without an index, a search sorts the visible rows. For large tables create a pgvector index (`hnsw` or `ivfflat`) on the column with the operator class that matches your distance. Because the function inlines into the caller's query, the planner can use it; check with `EXPLAIN`.

## Errors

| Code | When |
|---|---|
| E513 | The column is not `vector(n)`, the table is not in the on-disk metadata, there is no on-disk metadata, or the function is missing from Postgres when you run `--track` |
| E514 | A role in `--roles` has no select permission on the table, or any role with a select permission hides the vector column |

See [[error-codes]].
