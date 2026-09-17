# nself mail

<!-- BEGIN PROSE:summary -->
> Moved to a plugin (CLI-R11). Install it to keep using `nself mail`.
<!-- END PROSE:summary -->

## This command moved

`mail` was extracted from the core binary into the `mail` plugin as part of
[CLI-R11](https://github.com/nself-org/cli/blob/main/.claude) (the thin-core
extraction). The command surface is unchanged — only where the code lives
changed.

```bash
nself install mail
nself mail send --to user@example.com --subject "Welcome" --body "Hi"
nself mail broadcast --list customers --template welcome
nself mail status --message-id <id>
nself mail templates list
nself mail dkim verify --domain example.com
```

Until it is installed, `nself mail ...` prints an install hint pointing
here. Full documentation (flags, exit codes, examples) now lives with the
plugin: https://github.com/nself-org/bundles/tree/main/paid/mail

`mail` requires an ɳSelf+ or ɳClaw bundle license (the bundle that ships the
Postmark plugin); without a configured key it exits 2, exactly as it did
in-core.

## See Also

<!-- BEGIN PROSE:see-also -->
- [[cmd-install]], plugin/bundle install sugar
- [[cmd-plugin]], plugin lifecycle management
- [[cmd-license]], manage license keys for paid bundles
- [[Commands]], full command index
<!-- END PROSE:see-also -->

← [[Commands]] | [[Home]] →
