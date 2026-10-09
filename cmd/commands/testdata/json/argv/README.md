# JSON conversion argv files

Each `<fragment>.txt` file contains one command argv per line, without the leading `nself`. Blank lines and lines beginning with `#` are ignored. Arguments are separated by spaces; shell quoting and expansion are not supported. A tag section follows `#|` on the same line.

- `legacy-json` compares v1.4 `--json` stdout and exit status byte for byte against merge-base.
- `volatile-json=timestamp` removes only the top-level `timestamp` field before comparing a legacy document that embeds the current clock. The `health` row applies this automatically. Its remaining fields and exit status must match.
- `no-json=<reason>` skips the v1.5 envelope check for a command assigned to a later conversion ticket.
- `human-skip=<reason>` skips the base versus head human stdout and exit comparison when a documented difference is expected.

The harness checks human output in v1.4 and v1.5 modes, then requires exactly one valid v1 envelope and registered data shape for every row without `no-json`.

The `doctor` row normalizes only the measured free-space number in human output. Its remaining bytes and exit status must match. The `health` row normalizes only its live timestamp in legacy JSON.
