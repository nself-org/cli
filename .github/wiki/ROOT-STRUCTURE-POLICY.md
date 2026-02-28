# Root Structure Policy

This repository follows a strict structure-first root layout.

## Preferred Root Contents

1. `/.claude` (gitignored)
2. `/.codex` (gitignored)
3. `/.github`
4. `/.wiki`
5. `/bin`
6. `/src`
7. `/install.sh`
8. `/README.md`

## Allowed Exceptions

Only absolutely required extras are allowed at root, such as:

1. `LICENSE`
2. `.gitignore`
3. repository/security metadata files

## Temporary and Planning Files (Hard Rule)

1. Claude planning/temp files must stay in `/.claude`.
2. Codex planning/temp files must stay in `/.codex`.
3. Use versioned folders such as `/.claude/v1` and `/.codex/v1` for active runs.
4. Do not place scratch reports or planning markdown files in root.

## Documentation Rule

1. Public docs source is `/.wiki`.
2. Changelog entrypoint should exist at `/.wiki/CHANGELOG.md` and may link to detailed release notes under `/.wiki/releases`.
3. Keep private operational instructions out of `/.wiki`.
