# nself dlq

<!-- BEGIN PROSE:summary -->
> Moved to a plugin (CLI-R11). Install it to keep using `nself dlq`.
<!-- END PROSE:summary -->

## This command moved

`dlq` was extracted from the core binary into the `dlq` plugin as part of
the thin-core extraction (CLI-R11); see the thin-core
extraction). The command surface is unchanged — only where the code lives
changed.

```bash
nself install dlq
nself dlq replay mux --dry-run
nself dlq replay mux --filter status=quarantined
```

Until it is installed, `nself dlq ...` prints an install hint pointing here.
Full documentation (flags, exit codes, examples) now lives with the plugin:
https://github.com/nself-org/plugins/tree/main/free/dlq

## See Also

<!-- BEGIN PROSE:see-also -->
- [[cmd-install]], plugin/bundle install sugar
- [[cmd-plugin]], plugin lifecycle management
- [[cmd-queue]], async job queue management
- [[Commands]], full command index
<!-- END PROSE:see-also -->

← [[Commands]] | [[Home]] →
