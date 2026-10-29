# nself federation

<!-- BEGIN PROSE:summary -->
> Moved to a plugin (CLI-R11). Install it to keep using `nself federation`.
<!-- END PROSE:summary -->

## This command moved

`federation` was extracted from the core binary into the `federation` plugin
as part of the thin-core extraction (CLI-R11); see the
thin-core extraction). The command surface is unchanged — only where the
code lives changed.

```bash
nself install federation
nself federation compose
nself federation status
nself federation status --json
nself federation introspect
```

Until it is installed, `nself federation ...` prints an install hint
pointing here. Full documentation (subgraph configuration, flags, examples)
now lives with the plugin:
https://github.com/nself-org/plugins/tree/main/free/federation

Federation stays a free MIT plugin (no license required): it is an opt-in
architecture feature (`NSELF_FEDERATION=true`), not a licensed one.

## See Also

<!-- BEGIN PROSE:see-also -->
- [[cmd-install]], plugin/bundle install sugar
- [[cmd-plugin]], plugin lifecycle management
- [[cmd-build]], rebuild docker-compose and nginx configs after federation changes
- [[cmd-status]], view the running state of your nSelf install
- [[Commands]], full command index
<!-- END PROSE:see-also -->

← [[Commands]] | [[Home]] →
