# nself redis (Deprecated)

Legacy compatibility wrapper for Redis service operations.

## Status

- Deprecated command.
- Canonical replacement: `nself service redis ...`

## Syntax

```bash
nself redis <legacy-subcommand> ...
```

## Canonical Redis Namespace

- `nself service redis init`
- `nself service redis add --name <name>`
- `nself service redis list`
- `nself service redis get <name>`
- `nself service redis delete <name>`
- `nself service redis test <name>`
- `nself service redis health [name]`
- `nself service redis pool configure`
- `nself service redis pool get <name>`

## References

- Consolidated service docs: [`SERVICE.md`](SERVICE.md)

