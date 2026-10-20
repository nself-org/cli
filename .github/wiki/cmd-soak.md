# nself soak

<!-- BEGIN PROSE:summary -->
> Moved to a plugin (CLI-R11). Install it to keep using `nself soak`.
<!-- END PROSE:summary -->

## This command moved

`soak` was extracted from the core binary into the `soak` plugin as part of
CLI-R11 (the thin-core
extraction). The command surface is unchanged — only where the code lives
changed.

```bash
nself install soak
nself soak abort --rollback v1.0.8
```

Until it is installed, `nself soak ...` prints an install hint pointing here.
Full documentation (flags, exit codes, examples) now lives with the plugin:
https://github.com/nself-org/plugins/tree/main/free/soak

## See Also

<!-- BEGIN PROSE:see-also -->
- [[cmd-install]], plugin/bundle install sugar
- [[cmd-plugin]], plugin lifecycle management
- [[Commands]], full command index
<!-- END PROSE:see-also -->

← [[Commands]] | [[Home]] →
