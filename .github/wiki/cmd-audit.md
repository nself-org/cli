# nself audit

<!-- BEGIN PROSE:summary -->
> Moved to a plugin (CLI-R11). Install it to keep using `nself audit`.
<!-- END PROSE:summary -->

## This command moved

`audit` was extracted from the core binary into the `audit` plugin as part of
CLI-R11 (the thin-core
extraction). The command surface is unchanged — only where the code lives
changed. Note this is the `nself audit docs` documentation audit; the table
audit at `nself plugin audit-tables` and the security event log stayed in
core, they are unrelated code paths that happened to share the `internal/audit`
package name.

```bash
nself install audit
nself audit docs
```

Until it is installed, `nself audit ...` prints an install hint pointing
here. Full documentation (flags, exit codes, examples) now lives with the
plugin: https://github.com/nself-org/plugins/tree/main/free/audit

## See Also

<!-- BEGIN PROSE:see-also -->
- [[cmd-install]], plugin/bundle install sugar
- [[cmd-plugin]], plugin lifecycle management (includes `nself plugin audit-tables`)
- [[cmd-doctor]], system diagnostics
- [[Commands]], full command index
<!-- END PROSE:see-also -->

← [[Commands]] | [[Home]] →
