# nself watchdog

<!-- BEGIN PROSE:summary -->
> Moved to a plugin (CLI-R11). Install it to keep using `nself watchdog`.
<!-- END PROSE:summary -->

## This command moved

`watchdog` was extracted from the core binary into the `watchdog` plugin as
part of the thin-core extraction (CLI-R11); see the
thin-core extraction). The command surface is unchanged — only where the
code lives changed.

```bash
nself install watchdog
nself watchdog status
nself watchdog reset-breakers
nself watchdog history --since 7d
```

Until it is installed, `nself watchdog ...` prints an install hint pointing
here. Full documentation (flags, exit codes, examples) now lives with the
plugin: https://github.com/nself-org/plugins/tree/main/free/watchdog

## See Also

<!-- BEGIN PROSE:see-also -->
- [[cmd-install]], plugin/bundle install sugar
- [[cmd-plugin]], plugin lifecycle management
- [[cmd-alerts]], Prometheus alert rules
- [[cmd-health]], health checks
- [[cmd-status]], service health
- [[Commands]], full command index
<!-- END PROSE:see-also -->

← [[Commands]] | [[Home]] →
