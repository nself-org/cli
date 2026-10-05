# sdk/go/output

The plugin copy of the nSelf machine contract: the v1 JSON envelope, the error
object, exit classes and the NDJSON stream framing. Plugins are separate Go
modules and cannot import the CLI's `internal/output`, so this package renders
the same bytes. It imports the standard library only.

```go
import "github.com/nself-org/cli/sdk/go/v2/output"
```

## Success and error documents

```go
// stdout: 2-space indent, no HTML escaping, one trailing newline.
err := output.WriteData(os.Stdout, "infra status", map[string]any{"ok": true}, nil)

// An error envelope has an error object and no data key.
err = output.WriteError(os.Stdout, "infra status", output.ErrorDetail{
    Code:    "E410",
    Message: "cannot reach the host",
    Class:   output.ClassInfra, // exit_code 2 is derived
}, nil)

os.Exit(output.ExitCodeFor(output.ClassInfra))
```

`Data` and `Error` return the bytes instead of writing. `meta` is optional;
a `Meta` with no deprecations and no warnings writes no `meta` key.

| Class | Exit status |
|---|---|
| `user` | 1 |
| `infra` | 2 |
| `auth` | 3 |
| `destructive_blocked` | 4 |
| `other` | any other status; set `ErrorDetail.ExitCode` yourself |

Redaction is the CLI's job in its own writer. A plugin keeps secrets and
personal data out of the text it passes in.

## Streams (contract:cli.json-stream v1)

Every line is one compact envelope. Record lines carry `data.type`
(`log`, `progress`, `event`; never `result`). The last line is `StreamEnd`
(`data.type` is `result`) or `StreamError`.

```go
line, _ := output.StreamRecord("logs", "log",
    output.Field{Key: "service", Value: "web"},
    output.Field{Key: "line", Value: "started"})
os.Stdout.Write(line)

end, _ := output.StreamEnd("logs", nil, output.Field{Key: "lines", Value: 1})
os.Stdout.Write(end)
```

Members of `data` are written in the order given, after `type`.

## Testing a plugin

`CheckEnvelope` is the dependency-free equivalent of
`schemas/envelope.v1.schema.json`. Call it on a whole document, or on each
stream line, in a plugin's tests:

```go
if err := output.CheckEnvelope(stdout.Bytes()); err != nil {
    t.Fatal(err)
}
```

## V15 gating

A plugin that changes its output shape in v1.5 gates the new behaviour with
`sdk/go/compat` (`compat.V15`), the same switch the CLI uses (ADR 0021): the
old output stays the default and the envelope runs only when V15 reports true.

## Keeping it identical to the CLI

`testdata/cases/*.json` are `{input, expected}` pairs (`expected` is the
document as a list of lines). This package's tests render each input and
compare bytes. The CLI's `internal/repoqa/sdk_output_test.go` renders the same
inputs with `internal/output`, checks the expected bytes against the JSON
Schema, and checks the `valid` and `invalid` fixtures against the schema, so
neither implementation can drift without a failing test. Add a case when
either side gains behaviour.
