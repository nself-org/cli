# nself secrets (Deprecated)

Legacy compatibility wrapper for configuration secrets.

## Status

- Deprecated command.
- Canonical replacement: `nself config secrets ...`

## Syntax

```bash
nself secrets <legacy-subcommand> ...
```

## Canonical Secrets Namespace

- `nself config secrets list [--env ENV]`
- `nself config secrets get <key>`
- `nself config secrets set <key> <value>`
- `nself config secrets delete <key>`
- `nself config secrets rotate [key]`

## References

- Canonical config docs: [`CONFIG.md`](CONFIG.md)

