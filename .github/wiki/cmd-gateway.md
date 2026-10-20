# nself gateway

<!-- BEGIN PROSE:summary -->
> Moved to a plugin (CLI-R11). Install it to keep using `nself gateway`.
<!-- END PROSE:summary -->

## This command moved

`gateway` was extracted from the core binary into the `gateway` plugin as
part of CLI-R11 (the
thin-core extraction). The command surface is unchanged — only where the code
lives changed.

```bash
nself install gateway
nself gateway status
nself gateway keys list
nself gateway quota
nself gateway routes
```

Until it is installed, `nself gateway ...` prints an install hint pointing
here. Full documentation (flags, exit codes, examples) now lives with the
plugin: https://github.com/nself-org/plugins/tree/main/free/gateway

## See Also

<!-- BEGIN PROSE:see-also -->
- [[cmd-gauth]], manage Google OAuth tokens for nSelf AI services
- [[cmd-install]], plugin/bundle install sugar
- [[cmd-plugin]], plugin lifecycle management
- [[Commands]], full command index
<!-- END PROSE:see-also -->

← [[Commands]] | [[Home]] →
