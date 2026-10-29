# nself monitor

<!-- BEGIN PROSE:summary -->
> Moved to a plugin (CLI-R11). Install it to keep using `nself monitor`.
<!-- END PROSE:summary -->

## This command moved

`monitor` was extracted from the core binary into the `monitor` plugin as
part of the thin-core extraction (CLI-R11); see the
thin-core extraction). The command surface is unchanged — only where the
code lives changed.

```bash
nself install monitor
nself monitor upgrade-dashboards
nself monitor upgrade-dashboards --force
```

Until it is installed, `nself monitor ...` prints an install hint pointing
here. Full documentation (flags, exit codes, examples) now lives with the
plugin: https://github.com/nself-org/plugins/tree/main/free/monitor

## See Also

<!-- BEGIN PROSE:see-also -->
- [[cmd-install]], plugin/bundle install sugar
- [[cmd-plugin]], plugin lifecycle management
- [[cmd-alerts]], alert rules and silences
- [[cmd-status]], service health
- [[Commands]], full command index
<!-- END PROSE:see-also -->

← [[Commands]] | [[Home]] →
