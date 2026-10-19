# nself version

<!-- BEGIN PROSE:summary -->
> Show version and system information.
<!-- END PROSE:summary -->

## Synopsis

```
nself version [flags]
```

## Description

<!-- BEGIN PROSE:description -->
`nself version` prints the CLI version, Go build version, git commit hash, and build date. This information is embedded at compile time using Go linker flags (`-ldflags`).

Use `--short` for scripts that only need the version number. Use `--json` for structured output in monitoring systems or CI pipelines.
<!-- END PROSE:description -->

## Flags

<!-- BEGIN GENERATED:flags -->
| Flag | Default | Description |
|------|---------|-------------|
| `--json` | `false` | Print version info as JSON |
| `--short` | `false` | Print version number only |
| `--help`, `-h` | — | Show help |
<!-- END GENERATED:flags -->

## Examples

<!-- BEGIN PROSE:examples -->
```bash
# Show full version info
nself version

# Version number only
nself version --short

# JSON output
nself version --json
```

**Sample output:**

```
nself version X.Y.Z
  Go version:  go1.23.4
  Git commit:  a1b2c3d
  Build date:  2026-03-28T00:00:00Z
  OS/Arch:     darwin/arm64
```

**JSON output:**

```json
{
  "version": "1.0.0",
  "go_version": "go1.23.4",
  "git_commit": "a1b2c3d",
  "build_date": "2026-03-28T00:00:00Z",
  "os_arch": "darwin/arm64"
}
```
<!-- END PROSE:examples -->

## See Also

<!-- BEGIN PROSE:see-also -->
- [[Commands]] — full command index
- [[Core-Services]] — what a stack is made of
<!-- END PROSE:see-also -->

← [[Commands]] | [[Home]] →
