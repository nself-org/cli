# nself mcp

Start a Model Context Protocol (MCP) server for the local ɳSelf instance. Enables Claude Code and other MCP-compliant IDEs to introspect your ɳSelf project with zero configuration.

## Synopsis

```
nself mcp [--transport <stdio|sse|http>] [--port <port>]
```

There is no separate `serve` or `status` subcommand — `nself mcp` itself starts the server.

## Description

`nself mcp` starts the MCP server in **stdio mode** by default, the native transport that Claude Code expects. Pass `--transport sse` or `--transport http` to expose it over a local HTTP port instead; set `NSELF_MCP_TOKEN` to require a bearer token on those transports.

Run `nself mcp` from inside an nSelf project directory — it fails fast if one isn't found.

## Options

| Flag | Default | Description |
|---|---|---|
| `--transport`, `-t` | `stdio` | Transport: `stdio`, `sse`, or `http`. |
| `--port`, `-p` | `3825` | Port for the `sse`/`http` transports. |

## Environment variables

| Variable | Default | Description |
|---|---|---|
| `NSELF_MCP_TOKEN` | (empty) | Optional bearer token required on `sse`/`http` requests. Unset means no auth check; stdio mode is unaffected. |

## Claude Code integration

Add to `.claude/settings.json` in your project root:

```json
{
  "mcpServers": {
    "nself": {
      "command": "nself",
      "args": ["mcp"]
    }
  }
}
```

Claude Code will start the server automatically and expose its full tool, resource, and prompt set in its tool context (see [[cmd-mcp]] for the generated list). You can then ask Claude Code questions like:

- "What tables does this ɳSelf project have?"
- "What's the health status of this project?"
- "Which plugins are installed?"
- "Tail the hasura logs"
- "What permissions does the 'user' role have?"

## Available MCP tools

The tool, resource, and prompt list is generated from source (`make mcp-docs`) rather than hand-maintained here, so it can't drift the way an earlier hand-listed table did. See [[cmd-mcp]] for the current, authoritative list.

## Security

- HTTP/SSE transport binds to `127.0.0.1` by default. Never expose to `0.0.0.0` in production.
- Config values whose key contains `SECRET`, `PASSWORD`, `KEY`, or `TOKEN` are always redacted in tool output; there is no opt-out.
- The migration-apply tool requires an explicit `confirm: true` argument on every call, and a DDL allowlist blocks destructive statements even then. Treat it as a privileged operation.

## Examples

```bash
# Stdio only (Claude Code uses this)
nself mcp

# With HTTP/SSE on the default port
nself mcp --transport http --port 3825
```

## Plugin requirement

None — `nself mcp` is a core CLI command, no plugin or license required.

## See also

- [[cmd-mcp]]: generated tool/resource/prompt reference for this command
- [MCP specification](https://modelcontextprotocol.io/specification/2025-03-26)
- [[cmd-doctor]]

Note: `nself.org/plugins/mcp` documents an unrelated paid plugin (`bundles/paid/mcp`, ɳClaw/ClawDE bundles, its own port and tool set), not this core `nself mcp` command.
