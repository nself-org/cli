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

Run one MCP server per project. Start `nself mcp` in the project's directory; its tools act on that project only. Started in a directory with no project yet, it serves `nself_init` and the read-only tools; once `init` has created the project it lists every tool and sends `notifications/tools/list_changed`, and it does the same after a plugin is added or removed (for example the `ci` tools after `add ci`). Do not point one server at several projects. If you work on two projects, register two servers, each started from its own project directory. Tool results then never mix up which stack was changed.

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

## End-to-end proof

The agent E2E harness proves an agent can run a whole project through MCP alone. It lives in `test/e2e/agent/` (build tag `e2eagent`) and `scripts/e2e/agent/`.

What it covers:

- A replay client speaks MCP to `nself mcp` and nothing else. It runs `init`, `build` (plan, then confirm), `start`, `db migrate up`, `add ci`, an async CI run (detach, wait, evidence), `deploy qa`, then `deploy promote qa prod` as a dry run and as a confirmed run.
- Two legs: failure recovery (the harness stops one service, the agent reads `status` and recovers with `start`) and the destructive gate (`reset` without a confirmation returns the confirmation request and no volume changes).
- `qa` and `prod` are two loopback sim hosts. The server runs in a sandbox: a temp `HOME` holding only the sim key and ssh config, no SSH agent, and an allowlist of `127.0.0.1` for every host it may name.
- Every request, result and harness action goes to a JSONL transcript, and a verifier checks it.

How to run it (shown as text; it needs Docker and a cli checkout):

```text
bash scripts/e2e/agent/run.sh --self-test
bash scripts/e2e/agent/run.sh --replay --transcript /tmp/agent-e2e/replay.jsonl
bash scripts/e2e/agent/run.sh --verify-transcript /tmp/agent-e2e/replay.jsonl
```

The `Agent E2E` workflow runs the replay on GitHub-hosted ubuntu and uploads the transcript as `agent-e2e-transcript`. Details and known gaps: `test/e2e/agent/README.md`.
