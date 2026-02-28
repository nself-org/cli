# nself cloud (Deprecated)

Legacy compatibility wrapper for cloud/provider operations.

## Status

- Deprecated command.
- Canonical replacement: `nself infra provider ...`

## Migration

- `nself cloud provider list` -> `nself infra provider list`
- `nself cloud provider init <name>` -> `nself infra provider init <name>`
- `nself cloud provider validate` -> `nself infra provider validate`
- `nself cloud provider info <name>` -> `nself infra provider show <name>`
- `nself cloud server create` -> `nself infra provider server create`
- `nself cloud server list` -> `nself infra provider server list`
- `nself cloud server ssh <name>` -> `nself infra provider server ssh <name>`
- `nself cloud cost estimate` -> `nself infra provider cost estimate`
- `nself cloud deploy quick <server>` -> `nself infra provider deploy quick <server>`

## Syntax

```bash
nself cloud <legacy-subcommand> ...
```

## References

- Canonical command: [`PROVIDER.md`](PROVIDER.md)
- Infra namespace: [`INFRA.md`](INFRA.md)

