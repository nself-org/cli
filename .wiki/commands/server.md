# nself server (Legacy-Compatible Surface)

> **⚠️ DEPRECATED**: `nself server` is deprecated and will be removed in v1.0.0.
> Please use `nself deploy server` instead.
> Run `nself deploy server --help` for full usage information.

Server/VPS management command still present for compatibility and direct workflows.

## Status

- Runtime command exists (`src/cli/server.sh`).
- Canonical v1 organization maps server operations under `nself deploy server ...`.

## Syntax

```bash
nself server <command> [options]
```

## Commands

- `init`
- `check`
- `status`
- `diagnose`
- `setup`
- `ssl`
- `dns`
- `secure`

## Key Options

- `--host, -h`
- `--user, -u`
- `--port, -p`
- `--key, -k`
- `--domain, -d`
- `--env, -e`
- `--skip-ssl`
- `--skip-dns`
- `--yes, -y`

## References

- Canonical deploy docs: [`DEPLOY.md`](DEPLOY.md)
- Infra/provider docs: [`PROVIDER.md`](PROVIDER.md)

