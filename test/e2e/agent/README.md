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
bash scripts/e2e/agent/run.sh --replay --transcript /tmp/agent-e2e/replay.jsonl
bash scripts/e2e/agent/run.sh --verify-transcript /tmp/agent-e2e/replay.jsonl
bash scripts/e2e/agent/run.sh --tools-list ./nself --env NSELF_V15=1
bash scripts/e2e/agent/run.sh --live --transcript /tmp/agent-e2e/live.jsonl [--prod-probe DIR]
```

`--replay` and `--live` need Docker. Linux runs are the evidence; macOS runs only the live session.

## Sandbox

`mcp-sandbox.sh` starts the server with `env -i` (PATH, HOME, TMPDIR, NSELF_V15 and a fixed list of extras). HOME is a temp dir with only the sim key (generated per run by `sdk/go/simharness`, never committed), an ssh config and known hosts. The script refuses to start when SSH_AUTH_SOCK is set, when HOME links to or names the operator's `~/.ssh`, or when HOME, the inventory or the ssh config names a host other than `127.0.0.1` or `localhost`.

## Transcript

One JSON object per line: `{seq, time, role, kind, tool, arguments, envelope, is_error, note}`. Role is `agent`, `server` or `harness`. Kind is `tools/list`, `tools/call`, `result`, `action` or `probe`. Confirmation tokens and secret arguments are written as `[REDACTED]`.

The verifier fails a transcript that has a non-MCP agent step, a missing D7 step, steps out of order, a promote apply before its dry run, CI evidence before the run wait, a `tools/call` result that is not a v1 envelope, or a failed recovery or gate leg.

## Known gaps (found while building P7-SHIP-02)

The replay drives the real product and stops at the first failing step. These gaps stop it today:

1. `nself mcp` in v1.5 will not start outside a project (`resolveProjectDir`), so an MCP-only agent cannot reach `init`. The harness writes an empty `.env` marker and the agent calls `nself_init` with `force`. Owner: P7-SURF.
2. `nself_build` on a fresh project fails with E451: the gate runs `--plan`, the plan needs no confirmation, the gate then runs `--plan-id`, and a build that generates secrets cannot bind a plan id. The agent never sees a plan to confirm. Owner: P7-SURF-25 / P7-LIVE-03.
3. MCP tools are built once at server start, so `nself_ci` and its sub-tools do not appear in `tools/list` after `nself_add ci` in the same session. The ci plugin's machine surface (P7-CI-62) is also not merged yet. Owner: P7-SURF-03, P7-CI-62.
4. `nself_deploy_promote` accepts at most one argument (the registry names one arg `source target`), so `["qa", "prod"]` is rejected. Owner: P7-SURF-15 / P7-DEPL-15.
5. Document invocations stop at 300 s (`invoke.DefaultTimeout`) and `nself mcp` has no `--timeout` flag, so a cold `start` that pulls every image may time out.

The sim hosts get recording `docker` and `nself` stubs, as the deploy-sim tests do; whether `deploy qa` and `deploy promote` run end to end against them is not proven until the gaps above are closed.
