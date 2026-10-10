# Plugins and Bundles

Plugins have a **Free** or **Licensed** licence. A Licensed plugin belongs to a Bundle; the registry still uses its legacy `pro` tier value for compatibility with older CLIs.

Use `nself plugin search --licensed <query>` to find Licensed plugins. The older `--pro` spelling works through v1.5 and prints a deprecation warning; it is removed in v1.6.0. Use `nself plugin install <name> --tier licensed` to select the Licensed side of a tier pair. The older `--tier pro` spelling remains accepted.

`nself plugin list --available` shows both sides of a tier pair with Free and Licensed labels. In v1.5 mode, JSON output uses `license: free|licensed`; v1.4 mode keeps the legacy tier value. Bundle membership and licence entitlement still determine whether a Licensed plugin can be installed.

## Tier resolution and stickiness (v1.5 mode)

A plugin served as a Free and a Licensed pair (cron, notify) keeps the tier it was installed with. `nself plugin update notify` on a Free install stays Free even after you buy a licence, so a purchase never changes a running stack by itself. To move to the other tier on purpose, pass `--tier licensed` (or `--tier free`); `--tier licensed` still needs the licence.

The reason a tier was chosen is one of `installed`, `override`, `entitlement` or `default-free`. If the installed tier cannot be kept (the licence lapsed, or the registry now serves only the other tier), the update stops with E131 and changes nothing; pass `--tier` to choose. A failed entitlement lookup is reported as itself, never as a downgrade. In v1.4 mode an update keeps resolving by entitlement as before. `nself plugin info <name> --json` reports `tier` (`free` or `licensed`) and `tier_reason`; `nself plugin update <name> --tier free|licensed` is the explicit switch.

## Compose fragment rules

`nself build` reads each installed plugin's `docker-compose.plugin.yml` and normalises it in place.

- **No fragment (E128).** A plugin that declares a compose service (`service.kind: compose`, or a v1 manifest with a Dockerfile and a port) but ships no fragment fails the build in v1.5 mode and names the plugin. In v1.4 mode it warns and is left out of the stack. Plugins of kind `cli` or `library`, and disabled plugins, are not affected.
- **Networks.** In v1.5 mode a service with no `networks:` is attached to `${DOCKER_NETWORK}`, and `${DOCKER_NETWORK:-nself_network}` is written as `${DOCKER_NETWORK}`. A service with `network_mode` is left alone.
- **Foreign networks (E129).** A fragment may use an external network only if it is the project network (`${DOCKER_NETWORK}`, or `<project>_network`). Any other external network fails the build in v1.5 mode (E129 names the plugin and network) and warns in v1.4 mode.
- **PORT.** In v1.5 mode every fragment gets `PORT` equal to `SERVICE_PORT` when the plugin declares a port. A `PORT` the fragment sets itself wins.
- **Policy (E130).** E130, "plugin compose fragment violates policy (ADR 0027)", is reserved for the fragment checks that forbid host-level access such as the Docker socket.

## Updating pulls new images

After `nself plugin update`, every image reference the new fragment has that the old one did not (a changed tag, or a new service) is pulled right away, so the next `nself start` runs it. Services that build locally are untouched here, and a reference that contains a `${VAR}` is pulled by `nself start`. A failed pull is a warning; the update itself is done.
