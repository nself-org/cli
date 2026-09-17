# nself encryption

<!-- BEGIN PROSE:summary -->
> Moved to a plugin (CLI-R11). Install it to keep using `nself encryption`.
<!-- END PROSE:summary -->

## This command moved

`encryption` was extracted from the core binary into the `encryption` plugin
as part of [CLI-R11](https://github.com/nself-org/cli/blob/main/.claude) (the
thin-core extraction). The command surface is unchanged — only where the code
lives changed.

```bash
nself install encryption
nself encryption configure --provider aws --key-id arn:aws:kms:us-east-1:123456:key/abc123
nself encryption verify
nself encryption rotate
nself encryption status
nself encryption key-events
```

Until it is installed, `nself encryption ...` prints an install hint pointing
here. Full documentation (flags, exit codes, examples) now lives with the
plugin: https://github.com/nself-org/bundles/tree/main/paid/encryption

BYOK encryption requires an ɳSelf+ or Enterprise license (`NSELF_BYOK=true`);
it has no effect on self-hosted Community deployments.

## See Also

<!-- BEGIN PROSE:see-also -->
- [[cmd-install]], plugin/bundle install sugar
- [[cmd-plugin]], plugin lifecycle management
- [[cmd-license]], manage your nSelf license key
- [[Commands]], full command index
<!-- END PROSE:see-also -->

← [[Commands]] | [[Home]] →
