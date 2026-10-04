# scripts/docs

Documentation gates that run from a shell, with no Go build.

## check-guide-commands.sh

Fails when a guide documents an `nself` command that nothing exercises.

```bash
bash scripts/docs/check-guide-commands.sh .github/wiki/Guide-Custom-Services.md
```

Pass any number of Markdown files. The checker never runs a documented command
and never uses the network.

### What it checks

Inside a fenced block tagged `bash`, `sh`, `shell` or `zsh`, every line that
starts with `nself ` (after an optional `$ `) is a documented command. Inside a
`console` (or `shell-session`) block only `$ nself ...` prompt lines count; the
other lines are program output. Lines outside those fences, other fence
languages (`yaml`, `text`, ...) and lines that do not start with `nself` are
ignored.

A run that finds no command line at all (an empty guide, or one retagged to a
fence this tool does not scan) fails with exit 1, so a guide can not pass by
documenting nothing the checker can see.

### Signature rule

The signature is `nself` plus the leading tokens up to the first token that

- starts with `-`, `<`, `[`, `$`, a quote or `#`, or
- contains `=`.

A token is also cut at the first `;`, `|`, `&`, `>`, `<` or `\` inside it, so
`nself start; echo hi` and `nself status|jq` have the signatures `nself start`
and `nself status`.

`nself db import supabase --file x` has the signature `nself db import supabase`.
`nself env set KEY=value` has the signature `nself env set`.

Positional arguments are part of the signature: `nself init myproject` needs
the text `nself init myproject` somewhere in the corpus. Write placeholders in
angle brackets (`nself init <project>`, signature `nself init`) or add a skip
marker when a guide has to show a concrete value.

The signature must appear, as a fixed string, in a file under `scripts/`
(excluding `scripts/docs/`) or in any `*_test.go` file (`vendor/`,
`node_modules/`, `.git/` and `.claude/` are not searched). Add a test or a
script leg that spells the command out; a one-line comment in a leg is enough
only when that leg really runs the command.

### Skip marker

End a line with `# doc-check: skip <reason>` to exempt it, for example a
command that needs a live external service:

```bash
nself db import supabase --file dump.sql # doc-check: skip needs a live Supabase project
```

A skip with no reason fails the check.

### Output and exit codes

Each unmatched line prints `<file>:<line>: <signature>`. A skip without a
reason prints `<file>:<line>: doc-check: skip needs a reason`. A summary line
`checked N command lines (K skipped) in M files` goes to stderr.

| Exit | Meaning |
|------|---------|
| 0 | every documented command is matched or skipped |
| 1 | at least one unmatched line, an empty skip reason, or no command lines found |
| 2 | no arguments, an input file does not exist, or the root has no `scripts/` directory (`missing root`) |

Consumers call it exactly as `bash scripts/docs/check-guide-commands.sh <files>`.
Keep these exit codes stable.

### Tests

```bash
bash scripts/docs/check-guide-commands_test.sh
```

The test builds throwaway repository roots through `GUIDE_CHECK_ROOT` and reads
its Markdown inputs from `scripts/docs/testdata/`.
