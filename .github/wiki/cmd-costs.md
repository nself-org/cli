# nself costs

<!-- BEGIN PROSE:summary -->
> Moved to a plugin (CLI-R11). Install it to keep using `nself costs`.
<!-- END PROSE:summary -->

## This command moved

`costs` was extracted from the core binary into the `costs` plugin as part of
CLI-R11 (the thin-core
extraction). The command surface is unchanged — only where the code lives
changed.

```bash
nself install costs
nself costs
nself costs --server-type cx23
nself costs --format json
```

Until it is installed, `nself costs ...` prints an install hint pointing
here. Full documentation (flags, exit codes, examples) now lives with the
plugin: https://github.com/nself-org/plugins/tree/main/free/costs

## See Also

<!-- BEGIN PROSE:see-also -->
- [[cmd-install]], plugin/bundle install sugar
- [[cmd-plugin]], plugin lifecycle management (affects bundle license cost lines)
- [[cmd-license]], manage your nSelf license key
- [[Commands]], full command index
<!-- END PROSE:see-also -->

← [[Commands]] | [[Home]] →
