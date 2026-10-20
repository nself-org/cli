# nself gdpr

<!-- BEGIN PROSE:summary -->
> Moved to a plugin (CLI-R11). Install it to keep using `nself gdpr`.
<!-- END PROSE:summary -->

## This command moved

`gdpr` was extracted from the core binary into the `gdpr` plugin as part of
CLI-R11 (the thin-core
extraction). The command surface is unchanged — only where the code lives
changed.

```bash
nself install gdpr
nself gdpr export --user <user_id> --output /tmp/export.zip
nself gdpr delete --user <user_id> --dry-run
nself gdpr forget --user <user_id>
nself gdpr status --request <request_id>
nself gdpr list-requests
```

All operations write an entry to `np_gdpr_requests`, the append-only audit
trail required by GDPR Art. 30. That table is never deleted.

Until it is installed, `nself gdpr ...` prints an install hint pointing
here. Full documentation (flags, exit codes, examples) now lives with the
plugin: https://github.com/nself-org/plugins/tree/main/free/gdpr

## See Also

<!-- BEGIN PROSE:see-also -->
- [[cmd-install]], plugin/bundle install sugar
- [[cmd-plugin]], plugin lifecycle management
- [[cmd-security]], security audit and hardening
- [[Commands]], full command index
<!-- END PROSE:see-also -->

← [[Commands]] | [[Home]] →
