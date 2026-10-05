# tools/catalog

One generator for `registry.json`, `catalog.json` and `counts.json` (contract:plugins.catalog v1,
Epic P7-PLUG D3). It replaces the hand-edited registries, `plugins-pro:scripts/build-registry.py`
and `plugins:scripts/plugin-counts.sh`. Every value derives from a manifest, `releases.json` or
`bundles.json`; nothing is typed by hand.

Run it in a plugin repo, pinned to a CLI tag (no network beyond the module download):

```sh
go run github.com/nself-org/cli/tools/catalog@<cli tag> -mode free ...
```

| Mode | Reads | Writes |
|---|---|---|
| `free` | `-plugins free/`, `-releases`, `-bundles`, optional `-peer-registry` (licensed registry) | `registry.json` |
| `licensed` | `-plugins paid/`, `-releases`, `-bundles`, optional `-peer-registry` (free registry) | `registry.json` |
| `catalog` | `-free-registry`, `-licensed-registry`, `-bundles` | `catalog.json`, `counts.json` |

Common flags: `-out dir`, `-require-released` (a manifest with no `releases.json` row is an error instead of a note), `-check` (write nothing, exit 1 when a file differs), `-plugins-ref`,
`-bundles-ref`, `-generated-at` (RFC 3339; default `SOURCE_DATE_EPOCH`, else the Unix epoch, never a
clock). Exit 0 ok, 1 `-check` found a difference, 2 bad input or a data problem (all problems are listed).
`-partial` is a diagnostic: it writes what loads and still exits 2.

## Inputs

- **Manifests** are read only through `internal/plugin/manifestv2`, so v1 and v2 `plugin.json` files give
  the same entry. The directory name must equal the manifest `name`. Directories starting with `_` or `.`
  are not plugins: they are skipped with a `note:` on stderr, so a manifest placed there is never read.
  A manifest carrying both `minNselfVersion` and `min_nself_version` with different values is refused. A manifest whose `license` does not
  match the tree (free in `paid/`, licensed in `free/`) is refused: a plugin never moves tier here.
  The public repo never reads licensed manifests; it reads the licensed `registry.json` this tool wrote.
- **releases.json** (`schemas/releases.v1.schema.json`) is release data written by the release pipeline
  from published bytes: per slug `version`, `sha256`, `release_signature` (`{key_id, alg: ed25519, sig}` or
  null), `tarball_url`, and optional `release_tag`, `tarball`, `platform_checksums`, `binaries`. A manifest
  with no row is left out of the registry (no checksum yet) and named on stderr (`-require-released` makes it
  an error); a row with no plugin directory is an error. When the manifest and the row disagree on the
  version, the registry carries the release and a note names both.
- **Command binaries.** v1.4.12 turns an entry's `cliCommands`, `binaryName` and `pluginType` into required
  `nself-<command>` binaries and fails the install when the archive lacks one. A manifest cannot say whether
  the archive ships them, so those three keys are emitted only for a release whose `binaries` list covers
  every binary they require (a shortfall is an error); a row without `binaries` emits none, and the
  entry requires nothing, as the committed registry does for source-only archives. The release pipeline
  writes `binaries` from the tarball listing.
- **bundles.json** is the membership source of truth. A `free` bundle lists free entries, a `paid` bundle
  lists licensed entries (catalog tier `licensed`). A member missing from its tier's registry is an error.

## Outputs

`registry.json` keeps the key names and types of the registry released CLIs decode (v1.4.12). It adds
`_generated`, `generated_from`, `release_signature` (when signed) and a per-entry `catalog` block (the
manifest-only columns, so the catalog is built from registries alone). Dropped: the hand-written `note`,
the two timestamps `generated_at` and `last_updated`, the licensed `version` and `checksum_provenance`
envelope keys, and the licensed `security_always_free` entry key (no manifest v2 home, read by no CLI).
`tier` is `free` or `pro` on the wire. `tier_pair` is set when the slug is in the peer registry.

`catalog.json` has one row per slug and tier sorted by `(slug, tier)`; a tier-pair slug has a row per
tier and counts once. `counts.json` keeps the existing shape (`free`, `pro`, `overlap`, `totals`,
`advertised`) with `generated_from` in place of `generated_at` and `sources`.

All files: sorted keys, 2-space indent, trailing newline, no HTML escaping, byte-identical for identical
inputs. Timestamps appear only as `generated_from.generated_at`.

## Tests

`go test ./tools/catalog/...` runs golden files over trimmed copies of real plugin manifests and the real
bundles membership, the v1.4.12 decode test, the v1/v2 twin test and the schema checks.
`go test ./tools/catalog -update` rewrites the goldens.
