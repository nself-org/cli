# Guide: SDKs

nSelf has two SDK families. There is no Flutter SDK.

## API clients: @nself/cli-api

`@nself/cli-api` is the TypeScript client for the CLI's machine surface: the HTTP API served by `nself admin serve`. Its source is in `sdk/ts-cli-api` in the cli repo. Install it with your package manager:

```bash
pnpm add @nself/cli-api
```

Authenticate with the machine token file (a bearer token). Never put a token in source or in a URL. Endpoint and envelope details: [[HTTP-API-Reference]].

## Plugin SDK: sdk/go/v2

Plugins written in Go use the module `github.com/nself-org/cli/sdk/go/v2`. It provides the plugin server, config, logging, metrics, migrations and licensing helpers. Start from [[Plugin-Dev-Guide]] and [[Plugin-Architecture]].

```bash
go get github.com/nself-org/cli/sdk/go/v2
```

Compatibility rules and the changelog sit beside the code in `sdk/go`.

## Choosing

- Calling nSelf from an app or a script: `@nself/cli-api`.
- Building a plugin that runs as a compose service: the Go plugin SDK.
- Letting an AI agent drive nSelf: [[Guide-Agents-MCP]].
