# Feature: Plugins

The ɳSelf plugin system extends your stack with additional services, API routes, and capabilities, without writing infrastructure code.

## How Plugins Work

A plugin is a manifest + Docker Compose overlay + optional Nginx config. When you install and build:

1. `nself plugin install <name>`, downloads and caches the plugin
2. `nself build`, merges the plugin's compose overlay into `docker-compose.yml` and injects Nginx routes
3. `nself start` / `nself restart`, runs the updated stack

## Plugin Tiers

The CLI no longer uses Basic / Pro / Elite / Business tiers. Bundles are independent purchases at $0.99/mo ($9.99/yr), with a free Task bundle and an all-access ɳSelf+ subscription at $3.99/mo ($39.99/yr). The authoritative catalog and pricing live at [nself.org/pricing](https://nself.org/pricing) and are fetched by the CLI from `https://plugins.nself.org/bundles.json`. See [[Licensing-Guide]] and [[Plugin-Licensing]] for details.

## Free Plugins

backup, content-acquisition, content-progress, cron, donorbox, feature-flags, github, github-runner, invitations, jobs, link-preview, mdns, mlflow, monitoring, notifications, notify, paypal, search, shopify, stripe, subtitle-manager, tokens, torrent-manager, vpn, webhooks

## Licensed Plugins

Categories: AI & Automation, Communication, Media & Processing, Content & Social, Commerce & Payments, Auth & Security, Infrastructure & Integration, Productivity, Gaming & Specialised. The newest addition is **claw-budget**, which adds token budget tracking and cost controls for AI assistant sessions.

See [[Plugin-Overview]] for the complete list.

## Plugin Commands

```bash
nself plugin install <name>       # install a plugin
nself plugin remove <name>        # remove a plugin
nself plugin update <name>        # update to latest version
nself plugin list                 # list installed plugins
nself plugin registry             # browse available plugins
```

## Building Your Own Plugin

See [[Plugin-Dev-Guide]] to build and publish a custom plugin.

## See Also

- [[Plugin-Overview]], browse all plugins
- [[Plugin-Architecture]], technical details
- [[Plugin-Licensing]], tier and pricing details
- [[Plugin-Dev-Guide]], build a plugin

---
← [[Home]] | [[_Sidebar]]
