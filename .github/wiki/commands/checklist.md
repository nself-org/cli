# nself checklist

Production readiness verification checklist.

## Status

- Runtime command available at `src/cli/checklist.sh`
- Auxiliary command (not part of the canonical 32-command runtime v1 surface)

## Syntax

```bash
nself checklist [options]
```

## Options

- `--fix`: auto-fix issues where possible
- `--verbose`: detailed output
- `--json`: JSON output
- `-h, --help`: help

## Checks Performed

- SSL certificate validity
- Backup recency/configuration
- Monitoring and alerts
- Resource limits
- Secret strength/configuration
- Firewall status
- Log rotation
- Health endpoint reachability
- Database tuning checks
- Security headers

## References

- Canonical production docs: [`../deployment/README.md`](../deployment/README.md)
- Related command: [`DOCTOR.md`](DOCTOR.md)
