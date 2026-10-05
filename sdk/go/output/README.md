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
    Code:    "E901",
    Message: "cannot reach the host",
    Class:   output.ClassInfra, // exit_code 2 is derived
}, nil)

os.Exit(output.ExitCodeFor(output.ClassInfra))
```

`Data` and `Error` return the bytes instead of writing. `meta` is optional;
repeated deprecations and warnings are dropped (first kept), and a `Meta` with
none writes no `meta` key, as `internal/output` does.

| Class | Exit status |
|---|---|
| `user` | 1 |
| `infra` | 2 |
| `auth` | 3 |
| `destructive_blocked` | 4 |
| `other` | any other status; set `ErrorDetail.ExitCode` yourself |

### Rules for `Error`, `WriteError` and `StreamError`

- **No redaction.** The CLI redacts error text in its own writer; this package
  does not. Keep passwords, tokens, DSNs with credentials and personal data out
  of `Message`, `Cause` and `Remediation`, and be careful passing `err.Error()`.
- **`DocsURL` is written as given.** The SDK cannot read the CLI's error
  registry, so the CLI's default link for a known code is not filled in; set
  `DocsURL` when you want one.
- **The status decides the class**, as in `internal/output`. `ExitCode` 0 means
  unset: it comes from `Class`, and with neither set it is 1 (user). `Class` is
  always rewritten to match the status, so an unknown class or a class that
  disagrees with the status is normalised (class `auth` with status 2 is written
  as `infra`).
- **A document `CheckEnvelope` would reject is never written.** An empty code
  or message, a code that is not `E` and three digits, or a status outside 0 to
  255 returns an error and writes nothing.

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
document as a list of lines; `expect_error` marks an input the SDK refuses).
This package's tests render each input and compare bytes, and require that
every rendered case passes `CheckEnvelope`. The CLI's
`internal/repoqa/sdk_output_test.go` renders the same inputs with
`internal/output`, checks the expected bytes against the JSON Schema, checks
the `valid` and `invalid` fixtures against the schema, checks that each refused
input would give a schema-invalid document, and checks the exit-class table
`testdata/exit-classes.json` and the SDK constants against `internal/errs`.
Neither implementation can drift without a failing test.

Limits of that guarantee: the SDK is a separate module, so the two sides are
compared through the shared expected bytes, not in one process. Cases with
`go_type` render a typed Go struct (field order, nil versus empty slices and
maps) on both sides; the other data cases pass `map[string]any`. `internal/output`
has no stream encoder, so stream lines are compared with the compacted form of
its envelope. Add a case when either side gains behaviour.
