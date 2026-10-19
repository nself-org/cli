# nself release

**This command moved to a plugin.**

`nself release` orchestrates the nSelf project's own 12-step release cascade —
tagging this repo and bundles, building the admin image, opening the
Homebrew formula PR. It is maintainer tooling for the project, not something a
self-hosted user runs, so 1,523 lines of it no longer ship in the core binary.

## Install

```bash
nself install release
```

Once installed, `nself release ...` works exactly as before — the CLI proxies the
command to the plugin.

---

← [[Commands]] · [[Plugin-Overview]] · [[Home]]
