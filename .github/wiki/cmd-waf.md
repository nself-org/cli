# nself waf

<!-- BEGIN PROSE:summary -->
> Moved to a plugin (CLI-R11). Install it to keep using `nself waf`.
<!-- END PROSE:summary -->

## This command moved

`waf` was extracted from the core binary into the `waf` plugin as part of
CLI-R11 (the thin-core
extraction). The command surface is unchanged — only where the code lives
changed.

```bash
nself install waf
nself waf enable
nself waf mode blocking
nself waf report --since 1h
```

Until it is installed, `nself waf ...` prints an install hint pointing here.
Full documentation (flags, exit codes, examples) now lives with the plugin:
https://github.com/nself-org/plugins/tree/main/free/waf

WAF stays a free MIT plugin (no license required): it is
Security-Always-Free per the nSelf product doctrine.

## See Also

<!-- BEGIN PROSE:see-also -->
- [[cmd-install]], plugin/bundle install sugar
- [[cmd-plugin]], plugin lifecycle management
- [[cmd-security]], security audit and setup
- [[cmd-alerts]], alert rules
- [[cmd-monitor]], Grafana dashboards
- [[Commands]], full command index
<!-- END PROSE:see-also -->

← [[Commands]] | [[Home]] →
