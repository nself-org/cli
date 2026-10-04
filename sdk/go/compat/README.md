# compat

The ADR 0021 switch for plugin modules. Plugins are separate Go modules and
cannot import the CLI's `internal/compat`, so this package implements the same
rule and a golden table keeps the two copies identical.

```go
import "github.com/nself-org/cli/sdk/go/v2/compat"

// compat.V15(P7-XXX-NN): old behaviour -> new behaviour
if compat.V15() {
    // v1.5 behaviour
}
```

## The rule

`V15()` is true when `NSELF_V15` is `1` or `true` in any case. Unset, empty,
`0`, `yes`, `on` and a padded ` 1` are false. The environment is read on every
call and never cached. `Mode()` returns `"v1.5"` or `"v1.4"`.

Every gated call carries the marker `// compat.V15(<ticket-id>): <old> -> <new>`
on the line above or the same line.

P7-SHIP-09 flips the CLI default to v1.5. The CLI exports the resolved mode to
the plugin processes it starts (`contract:cli.plugin-exec-env`), so a plugin run
by the CLI sees the CLI's answer.

## One rule, two copies

`testdata/rule.json` holds `{"rows": [{"value": <string|null>, "cli": <bool>,
"sdk": <bool>}]}` (`null` means unset). This package's `TestRuleTableSDK` reads
the `sdk` column; the CLI's `internal/compat` `TestRuleTableCLI` reads the `cli`
column. They are equal until P7-SHIP-09, which changes only the `cli` column.

## Testing a gated plugin

```go
import "github.com/nself-org/cli/sdk/go/v2/compat/compattest"

func TestFeature(t *testing.T) {
    compattest.Both(t, func(t *testing.T) { /* runs as subtests v1.4 and v1.5 */ })
}
```

`compattest.Set(t, true)` selects v1.5 for one test; the value is restored when
the test ends.

## Requiring the SDK

A plugin requires `github.com/nself-org/cli/sdk/go/v2` at the version that holds
this package: the `v2.M.P` module version published for CLI `v1.M.P` (see
[`../COMPATIBILITY.md`](../COMPATIBILITY.md)), or a merge-commit pseudo-version
until that tag exists.

This package imports the standard library only.
