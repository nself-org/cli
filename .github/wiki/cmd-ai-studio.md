# nself ai-studio

<!-- BEGIN PROSE:summary -->
> Moved to a plugin (CLI-R11). Install it to keep using `nself ai-studio`.
<!-- END PROSE:summary -->

## This command moved

`ai-studio` was extracted from the core binary into the `ai-studio` plugin as
part of CLI-R11 (the
thin-core extraction). The command surface is unchanged — only where the code
lives changed.

```bash
nself install ai-studio
nself ai-studio bridge
```

Until it is installed, `nself ai-studio ...` prints an install hint pointing
here. Full documentation (flags, exit codes, examples) now lives with the
plugin: https://github.com/nself-org/plugins/tree/main/free/ai-studio

## See Also

<!-- BEGIN PROSE:see-also -->
- [[cmd-install]], plugin/bundle install sugar
- [[cmd-plugin]], plugin lifecycle management
- [[Commands]], full command index
<!-- END PROSE:see-also -->

← [[Commands]] | [[Home]] →
