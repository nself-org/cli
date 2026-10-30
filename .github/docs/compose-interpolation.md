# Project Compose interpolation

Project Compose commands pass `.env`, when present, followed by
`.nself/compose.env` when the generated file exists. Without the generated
file, Compose keeps its normal `.env` discovery. This preserves values
written by `nself build` for logs, diagnostics, restarts, resets, migration,
plugin detection, WAF audit reads, rollback, and blue/green observations.

Use `docker.NewProjectCompose` for lifecycle calls and `docker.ComposeOutput`
for captured output. `docker.Stream` and `docker.StreamLines` apply the same
project interpolation files to Compose streams. Explicit global `--env-file`
arguments retain the caller's interpolation scope for streams and captured
commands. Child process arguments do not alter this scope. Compose version probes
do not read the project and need no project flags.

`nself dev` also reads the Compose manifest used by `nself start`, including
plugin fragments and user overrides. Its plugin rebuilds keep this file list.

Regression tests use a Docker stub that rejects calls missing either project
environment file. They require no daemon or running project.

The docker execution allowance drops from 97 to 88 sites. Routing migration,
model detection and WAF audit reads through this required funnel adds three
importing packages; its fan-in ceiling therefore moves from 27 to 30.
