# nself model

<!-- BEGIN PROSE:summary -->
> Moved to a plugin (CLI-R11). Install it to keep using `nself model`.
<!-- END PROSE:summary -->

## This command moved

`model` was extracted from the core binary into the `model` plugin as part of
the thin-core extraction (CLI-R11); see the thin-core
extraction). The command surface is unchanged — only where the code lives
changed. This includes the `ollama` subcommand tree ([[cmd-ollama]]),
which was already a child of `model`, not a separate top-level command.

```bash
nself install model
nself model list
nself model pull llama3.2:3b
nself model remove gemma-3-4b
nself model update llama3.2:3b
nself model benchmark llama3.2:3b
nself model ollama status
```

Until it is installed, `nself model ...` (and the legacy `nself ollama ...`
spelling) prints an install hint pointing here. Full documentation (flags,
exit codes, examples) now lives with the plugin:
https://github.com/nself-org/plugins/tree/main/free/model

## See Also

<!-- BEGIN PROSE:see-also -->
- [[cmd-ollama]], legacy spelling redirect (`nself ollama` → `nself model ollama`)
- [[cmd-ai]], manage models via the `nself-ai-gateway` registry
- [[cmd-install]], plugin/bundle install sugar
- [[cmd-plugin]], plugin lifecycle management
- [[Commands]], full command index
<!-- END PROSE:see-also -->

← [[Commands]] | [[Home]] →
