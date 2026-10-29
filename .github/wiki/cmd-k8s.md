# nself k8s

<!-- BEGIN PROSE:summary -->
> Moved to a plugin (CLI-R11). Install it to keep using `nself k8s`.
<!-- END PROSE:summary -->

## This command moved

`k8s` was extracted from the core binary into the `k8s` plugin as part of
the thin-core extraction (CLI-R11); see the thin-core
extraction). The command surface is unchanged — only where the code lives
changed.

```bash
nself install k8s
nself k8s install --domain myapp.example.com
nself k8s status
nself k8s upgrade
```

Until it is installed, `nself k8s ...` prints an install hint pointing here.
Full documentation (flags, exit codes, examples, the Helm chart itself) now
lives with the plugin: https://github.com/nself-org/plugins/tree/main/free/k8s

## See Also

<!-- BEGIN PROSE:see-also -->
- [[cmd-install]], plugin/bundle install sugar
- [[cmd-plugin]], plugin lifecycle management
- [[Commands]], full command index
<!-- END PROSE:see-also -->

← [[Commands]] | [[Home]] →
