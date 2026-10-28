# nself gauth

<!-- BEGIN PROSE:summary -->
> Moved to a plugin (CLI-R11). Install it to keep using `nself gauth`.
<!-- END PROSE:summary -->

## This command moved

`gauth` was extracted from the core binary into the `gauth` plugin as part of
the thin-core extraction (CLI-R11); see the thin-core
extraction). The command surface is unchanged — only where the code lives
changed.

```bash
nself install gauth
nself gauth status
nself gauth refresh --account work
nself gauth revoke --account work
```

Until it is installed, `nself gauth ...` prints an install hint pointing
here. Full documentation (flags, exit codes, examples) now lives with the
plugin: https://github.com/nself-org/plugins/tree/main/free/gauth

## See Also

<!-- BEGIN PROSE:see-also -->
- [[cmd-gateway]], manage the nSelf AI gateway
- [[cmd-install]], plugin/bundle install sugar
- [[cmd-plugin]], plugin lifecycle management
- [[Commands]], full command index
<!-- END PROSE:see-also -->

← [[Commands]] | [[Home]] →
