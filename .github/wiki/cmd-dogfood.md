# nself dogfood

<!-- BEGIN PROSE:summary -->
> Moved to a plugin (CLI-R11). Install it to keep using `nself dogfood`.
<!-- END PROSE:summary -->

## This command moved

`dogfood` was extracted from the core binary into the `dogfood` plugin as
part of CLI-R11 (the
thin-core extraction). The command surface is unchanged — only where the code
lives changed.

```bash
nself install dogfood
nself dogfood audit
nself dogfood report
```

Until it is installed, `nself dogfood ...` prints an install hint pointing
here. Full documentation (flags, exit codes, examples) now lives with the
plugin: https://github.com/nself-org/plugins/tree/main/free/dogfood

## See Also

<!-- BEGIN PROSE:see-also -->
- [[cmd-install]], plugin/bundle install sugar
- [[cmd-plugin]], plugin lifecycle management
- [[Commands]], full command index
<!-- END PROSE:see-also -->

← [[Commands]] | [[Home]] →
