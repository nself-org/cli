# nself alerts

<!-- BEGIN PROSE:summary -->
> Moved to a plugin (CLI-R11). Install it to keep using `nself alerts`.
<!-- END PROSE:summary -->

## This command moved

`alerts` was extracted from the core binary into the `alerts` plugin as
part of CLI-R11 (the
thin-core extraction). The command surface is unchanged — only where the
code lives changed.

```bash
nself install alerts
nself alerts list --severity P1
nself alerts silence ServiceDown --duration 4h --reason "scheduled deploy"
nself alerts test ServiceDown
```

Until it is installed, `nself alerts ...` prints an install hint pointing
here. Full documentation (flags, exit codes, examples) now lives with the
plugin: https://github.com/nself-org/plugins/tree/main/free/alerts

## See Also

<!-- BEGIN PROSE:see-also -->
- [[cmd-install]], plugin/bundle install sugar
- [[cmd-plugin]], plugin lifecycle management
- [[cmd-monitor]], monitoring stack management
- [[cmd-watchdog]], self-healing container watchdog
- [[cmd-health]], health checks
- [[Commands]], full command index
<!-- END PROSE:see-also -->

← [[Commands]] | [[Home]] →
