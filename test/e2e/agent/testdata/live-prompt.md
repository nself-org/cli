# Agent E2E live session (P7-SHIP-13)

You operate an nSelf project only through the `nself` MCP server. You have no
shell, file or web tools. The project directory and its inventory are already
prepared: environment `qa` (tier local-servers) and `prod` (tier prod) are two
loopback sim hosts.

Do these steps, in order, using MCP tools only:

1. Initialise the project (`nself_init`).
2. Build it: ask for the plan first, read it, then confirm it with the plan id
   the server returned.
3. Start the stack and check `nself_status` until `data.state` is `ok`.
4. If `nself_status` reports a service unhealthy, recover it with
   `nself_start` and check status again.
5. Call `nself_reset` once without a confirmation, read the confirmation
   request it returns, and do NOT confirm it.
6. Create a first migration (`nself_db_migrate_create` named
   `agent_e2e_first`) and apply pending migrations (`nself_db_migrate_up`).
7. Install the ci plugin (`nself_add` with `ci`), list the tools again, then
   run CI the async way: `nself_ci` with `detach: true`, then
   `nself_ci_runs_wait` with the returned run id until it is terminal, then
   `nself_ci_evidence`. The evidence result must be `pass`.
8. Deploy to `qa`.
9. Promote `qa` to `prod` with `dry-run` first, read the plan, then run the
   confirmed promote.

Rules: a remote or destructive call returns a confirmation request; repeat the
identical call with its `confirm` value only when the step above says so.
Never invent a confirmation value. Stop and report if a step fails.
