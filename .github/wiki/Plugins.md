# Plugins and Bundles

Plugins have a **Free** or **Licensed** licence. A Licensed plugin belongs to a Bundle; the registry still uses its legacy `pro` tier value for compatibility with older CLIs.

Use `nself plugin search --licensed <query>` to find Licensed plugins. The older `--pro` spelling works through v1.5 and prints a deprecation warning; it is removed in v1.6.0. Use `nself plugin install <name> --tier licensed` to select the Licensed side of a tier pair. The older `--tier pro` spelling remains accepted.

`nself plugin list --available` shows both sides of a tier pair with Free and Licensed labels. In v1.5 mode, JSON output uses `license: free|licensed`; v1.4 mode keeps the legacy tier value. Bundle membership and licence entitlement still determine whether a Licensed plugin can be installed.
