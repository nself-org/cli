# nself infra

**This command lives in the free `infra` plugin.**

`nself infra` is not part of the CLI core. Provisioning cloud servers is a
separate job from the self-hosted backend lifecycle the core covers, so it
ships as a free plugin for the people who want it. The Terraform module that
used to sit in the CLI repository now lives only in the plugin.

## Install

```bash
nself add infra
```

Once installed, the CLI proxies `nself infra ...` to the plugin.

## Provider

Hetzner Cloud is the only provider that ships. Any other `--provider` value
exits 1 with "not shipped".

## OpenTofu first

The plugin embeds its Hetzner module and runs `tofu` when OpenTofu is on your
`PATH`, falling back to `terraform` otherwise. OpenTofu is the preferred tool;
the plugin pins the OpenTofu version it is tested with.

## Commands

```bash
nself infra validate --provider hetzner
nself infra plan     --provider hetzner --domain myapp.com
nself infra apply    --provider hetzner --domain myapp.com --i-accept-cloud-spend
nself infra destroy  --provider hetzner --i-accept-cloud-spend
```

`validate` and `plan` change nothing. `apply` and `destroy` spend money or
delete servers, so they need `--i-accept-cloud-spend` and a confirmation at a
terminal; without a terminal they refuse and exit 4.

## Environment

| Variable | Purpose |
|---|---|
| `HCLOUD_TOKEN` | Hetzner Cloud API token used by the provider. |
| `HETZNER_NSELF_TOKEN` | Copied to `HCLOUD_TOKEN` when that is unset. |

---

← [[Commands]] · [[Plugin-Overview]] · [[Home]]
