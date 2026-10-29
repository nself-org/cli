# Guide: Agents and MCP

How to let an AI agent (Claude Code, Cursor, Codex) operate an nSelf project safely.

Reference pages: [[MCP-Tools]], [[MCP-Server]], [[HTTP-API-Reference]], [[Agent-Guide]], [[cmd-mcp]].

## Register the MCP server

Run this from inside an nSelf project:

```bash
nself mcp setup --client claude-code --write # doc-check: skip registered in cmd/commands/mcp_setup.go, tested by flag in mcp_setup_test.go
```

`--client` also accepts `cursor`. The `codex` and `generic` clients print the config for you to paste. Without `--write` nothing is changed on disk.

## One server per project

Run one MCP server per project. `nself mcp` must start inside a directory that holds an nSelf project, and its tools act on that project only. Do not point one server at several projects. If you work on two projects, register two servers, each started from its own project directory. Tool results then never mix up which stack was changed.

## The confirm and dry-run gate

Tools mirror the command registry. The gate decides what an agent may do alone:

- Read and write commands run directly.
- Remote and destructive commands need a confirmation round trip. The first call returns `confirmation_required` with a plan id or nonce. The agent repeats the identical call with the `confirm` value to run it.
- A remote or destructive command with no confirm block is refused (E421).
- Secret values are redacted in request ids and error tails.

Prefer the dry-run form of a command before the confirmed one, and show the plan to a person before you confirm anything destructive. Details: [[MCP-Server]].

## The HTTP API

For agents that speak HTTP instead of MCP, serve the same command surface on loopback:

```bash
nself admin serve --port 3857
```

On loopback the server creates a machine token file and prints its path (never the token). Send the token from that file as a bearer header (`Authorization: Bearer YOUR_TOKEN`). Non-loopback binds refuse weak tokens and need an allowed host. See [[HTTP-API-Reference]].

For MCP over HTTP, set the `NSELF_MCP_TOKEN` environment variable and start `nself mcp --transport http`.

## Running CI from an agent

These commands need the free `ci` plugin installed, so they are shown as text and are not run by the guide checker.

```text
nself add ci
nself ci --check
nself ci
```

`nself ci --check` runs the gates only. `nself ci` also posts the `nself-ci` commit status. See [[Guide-CI-CD]] and the plugins CI [Getting Started](https://github.com/nself-org/plugins/blob/main/.github/wiki/guides/ci/Getting-Started.md) page.
