# nself plugin marketplace

> Browse and search plugins from the ɳSelf marketplace.

## Synopsis

```
nself plugin marketplace <subcommand> [flags]
```

## Description

`nself plugin marketplace` queries the ɳSelf plugin registry at `plugins.nself.org` and lets you browse, search, and inspect plugins before installing them. Use it to explore what is available, filter by tier or bundle, and copy install commands.

Free plugins install without a license key. Licensed plugins require a valid key set with `nself license set`.

## Subcommands

### nself plugin marketplace list

Lists all available plugins from the marketplace.

**Flags:**

| Flag | Description |
|------|-------------|
| `--tier free\|pro` | Filter by tier |
| `--bundle <slug>` | Filter by bundle slug: `nclaw`, `clawde`, `nmedia`, `nfamily`, `nchat` |
| `--category <name>` | Filter by category (e.g. `ai`, `media`, `messaging`) |
| `--json` | Output raw JSON |

**Examples:**

```bash
nself plugin marketplace list
nself plugin marketplace list --tier free
nself plugin marketplace list --tier pro
nself plugin marketplace list --bundle nclaw
nself plugin marketplace list --category ai
nself plugin marketplace list --tier pro --json
```

### nself plugin marketplace search

Search for plugins matching a query string (name and description).

```
nself plugin marketplace search <query> [flags]
```

**Flags:** same as `list`

**Examples:**

```bash
nself plugin marketplace search "email"
nself plugin marketplace search "media streaming" --tier pro
nself plugin marketplace search "notification" --bundle nclaw
```

### nself plugin marketplace info

Show detailed information about a specific plugin.

```
nself plugin marketplace info <name> [flags]
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--json` | Output raw JSON |
| `--open` | Open the plugin page in a browser |

**Examples:**

```bash
nself plugin marketplace info ai
nself plugin marketplace info claw --json
nself plugin marketplace info livekit --open
```

## Installing a plugin

After finding a plugin, install it with:

```bash
nself plugin install <name>
```

Licensed plugins require a valid license key. Set one first:

```bash
nself license set nself_pro_xxxxx...
nself plugin install ai
```

## See also

- [[cmd-plugin]], full plugin management reference
- [[Plugin-Licensing]], license tiers and key format
- [[Plugin-Catalog]], full catalog of all plugins

← [[Commands]] | [[Home]] →
