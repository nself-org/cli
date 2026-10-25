# JSON conversion argv files

Each `<fragment>.txt` file contains one command argv per line, without the leading `nself`. Blank lines and lines beginning with `#` are ignored. Arguments are separated by spaces; shell quoting and expansion are not supported. A tag section follows `#|` on the same line.

- `legacy-json` compares v1.4 `--json` stdout and exit status byte for byte against merge-base.
- `volatile-json=timestamp` replaces only the top-level `timestamp` value in the raw legacy JSON bytes before comparison. The `health` row applies this automatically. Every other byte and the exit status must match.
- `no-json=<reason>` skips the v1.5 envelope check for a command assigned to a later conversion ticket.
- `error-json=<code>` marks a row that fails by design on the fixture (for example `start` without a compose file). Besides the human comparison, the head binary runs with `--json` in v1.5 mode and must exit non-zero, print exactly one error envelope that validates against `envelope.v1.schema.json`, and carry the named code (`E004`). It replaces `no-json` for such rows.
- `human-skip=<reason>` skips the base versus head human stdout, stderr and exit comparison when a documented difference is expected.

The harness checks human stdout, stderr and exit status in v1.4 and v1.5 modes, then requires a successful exit and exactly one valid v1 data envelope with the registered data shape for every row without `no-json`.

The `doctor` row normalizes only the measured free-space number in human output. Human stderr comparison normalizes only the clock prefix of Go's `ENV resolved for .env cascade` log lines. All other bytes and exit status must match. The `health` row normalizes only its live timestamp value in legacy JSON.
