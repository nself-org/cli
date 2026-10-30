# Agent E2E harness

The MCP-only agent end-to-end proof (P7-SHIP-02, SHIP Epic D7). An agent runs a whole nSelf project through the `nself` MCP server and nothing else. The harness checks every step from a transcript.

## Layout

| File | Role |
|---|---|
| `agent_test.go` | Tests: verifier fixtures, replay file, transcript check, tools/list, the replay |
| `live_test.go` | Live mode: a headless `claude -p` session with only the nself MCP server |
| `flow.go` | Replay client (mcp-go `client`): one `tools/call` per step, confirmation round trips |
| `harness.go` | Sim hosts and harness actions (fixture, git commit, service stop, probes) |
| `sandbox.go` | Sandbox HOME, inventory v2 file, the `mcp-sandbox.sh` argv |
| `transcript.go` | Transcript JSONL writer and reader |
| `verify.go`, `verify_legs.go` | Transcript rules |
| `testdata/replay.jsonl` | The replayed agent session |
| `testdata/live-prompt.md` | The live-session prompt |
| `testdata/good.jsonl`, `testdata/bad/*.jsonl` | Verifier fixtures |
| `testdata/sim/*.sh` | Recording `docker` and `nself` stubs for the sim hosts |

Scripts: `scripts/e2e/agent/run.sh` (all modes) and `scripts/e2e/agent/mcp-sandbox.sh` (sealed server start). Workflow: `.github/workflows/agent-e2e.yml`.

## Run it

```text
bash scripts/e2e/agent/run.sh --self-test
bash scripts/e2e/agent/run.sh --replay --transcript /tmp/agent-e2e/replay.jsonl [--ci-plugin-src ../plugins/free/ci]
bash scripts/e2e/agent/run.sh --verify-transcript /tmp/agent-e2e/replay.jsonl
bash scripts/e2e/agent/run.sh --tools-list ./nself --env NSELF_V15=1
bash scripts/e2e/agent/run.sh --live --transcript /tmp/agent-e2e/live.jsonl [--prod-probe DIR]
```

`--replay` and `--live` need Docker. Linux runs are the evidence; macOS runs only the live session.

## Sandbox

`mcp-sandbox.sh` starts the server with `env -i` (PATH, HOME, TMPDIR, NSELF_V15 and a fixed list of extras). HOME is a temp dir with only the sim key (generated per run by `sdk/go/simharness`, never committed), an ssh config, known hosts and, when the compose plugin is outside the system plugin dirs, a docker config naming its directory (no auths). The script refuses to start when SSH_AUTH_SOCK is set, when HOME links to or names the operator's `~/.ssh`, or when HOME, the inventory or the ssh config names a host other than `127.0.0.1` or `localhost`.

## Transcript

One JSON object per line: `{seq, time, role, kind, tool, arguments, envelope, is_error, note}`. Role is `agent`, `server` or `harness`. Kind is `tools/list`, `tools/call`, `result`, `action` or `probe`. Confirmation tokens and secret arguments are written as `[REDACTED]`.

The verifier fails a transcript that has a non-MCP agent step, a missing D7 step, steps out of order, a promote apply before its dry run, CI evidence before the run wait, a `tools/call` result that is not a v1 envelope, or a failed recovery or gate leg.

## Notes

- The server starts in the empty fixture project and lists `nself_init` plus the read-only tools; after `init` it lists every tool, and after `add ci` it sends `notifications/tools/list_changed` and the ci tools appear.
- The replay and live runs start `nself mcp --timeout 30m`: a cold `start` pulls every image and outlasts the 5m default.
- The served ci release (1.2.13) predates the ci machine surface (P7-CI-62). `run.sh --replay --ci-plugin-src <plugins>/free/ci` builds ci from that source; after the agent's `nself_add ci` the harness swaps the build in (a `role: harness` record naming the revision) and an agent `nself_status` call lets the server refresh its tool list. The workflow uses plugins main. Without the flag the served plugin is kept, and `discover-ci` fails until a release carries the surface. The live mode has no such swap, so it needs that release.
- The sim hosts get recording `docker` and `nself` stubs, as the deploy-sim tests do. The image-digest compare after the promote reads the stubs' record of `compose up`.
