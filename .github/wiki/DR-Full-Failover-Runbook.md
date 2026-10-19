# Annual Full Failover Drill Runbook

Once per year ɳSelf performs an extended disaster recovery drill that goes beyond the monthly cold-start drill: we actually swap live production DNS to the drill VM for 15 minutes, verify real traffic, then swap back. This page is the authoritative procedure.

## Schedule

- **Date:** April 15, every year.
- **Window:** 14:00-15:30 UTC (lowest combined EU/US traffic for nclaw-prod).
- **Rationale:** April 15 sits after US tax week and before the end-of-year code freeze, and both windows are clear of every other ɳSelf release milestone.

The annual drill is **never automated**. It requires explicit user authorization per the GCI production deploy rules before any DNS change is made.

## Targets

| Metric | Target |
| --- | --- |
| RTO (time to restored service on drill VM) | < 30 minutes |
| RPO (data loss from last backup to cutover) | < 5 minutes |
| Synthetic verification | 100 / 100 probe requests succeed on drill VM |

The monthly drill already exercises provision + restore + smoke. The annual adds live DNS cutover and a synthetic probe.

## Procedure

### Step 1, Prerequisites (T-60m)

1. Confirm the latest monthly drill passed (`dr_drill_report` WHERE `result = 'pass'` within the last 35 days).
2. Confirm explicit user authorization to proceed. Paste the authorization quote into the drill log.
3. Verify `HETZNER_CAMARATA_TOKEN` and `CLOUDFLARE_API_KEY` are available from the operator's secret store.

### Step 2, Run a fresh monthly drill (T-45m)

```bash
ssh nclaw-prod 'nself dr drill --now \
  --hetzner-project camarata \
  --vm-type cx22 \
  --ssh-key /root/.config/nself/dr-key.pub'
```

The drill must report `result: "pass"` before proceeding. On failure, abort and escalate.

### Step 3, Cut DNS TTL (T-30m)

Cut `api.nclaw-prod.camarata.com` TTL to 60 seconds in Cloudflare. Wait 30 minutes for the old TTL to expire across resolvers.

### Step 4, Swap A record to drill VM (T+0)

Swap the A record to the drill VM IP returned by step 2. Record the swap timestamp.

### Step 5, Synthetic verification (T+0 to T+15)

Run the synthetic probe bot for 15 minutes:

```bash
nself dr probe \
  --target https://api.nclaw-prod.camarata.com/healthz \
  --count 100 \
  --concurrency 4 \
  --timeout 10s
```

The probe emits JSON with `success`, `latency_p50_ms`, `latency_p99_ms`, `errors`. All 100 requests MUST succeed and p99 latency MUST stay under 2x the 30-day baseline. The probe is the single gate for declaring the drill a pass.

### Step 6, Swap DNS back (T+15)

Swap the A record back to the real primary IP. Wait for propagation (TTL 60s) and observe a second round of 100 probe requests against the real primary before restoring the long TTL.

### Step 7, Close-out

1. Destroy the drill VM via the Hetzner API.
2. Restore the production TTL (3600s).
3. Append a full report to `dr_drill_report` with `drill_id` prefix `annual-{YYYY}`.
4. Post the report JSON to `#ops-dr` Telegram channel.
5. File lessons to `.claude/memory/lessons.md` in the CLI repo if anything surprised us.

## Fail-Fix Playbook

If the drill fails at any step, apply the matching fix:

### `hetzner_quota_exceeded`

The Hetzner API rejected the VM create call because the camarata project is at its server quota.

1. Rotate the drill region: re-run with `--region fsn1-dc14` to use a different availability zone that may have spare capacity.
2. If the rotation also fails, request a quota bump from Hetzner support and reschedule the drill for the next business day. Never skip the drill, only defer it.

### `cloud-init` failure on drill VM

The VM provisioned but did not install Docker or ɳSelf.

1. Regenerate the user-data template and compare against the last known good copy:
   ```bash
   ssh nclaw-prod 'nself dr drill --render-cloud-init' > /tmp/u.yaml
   diff /tmp/u.yaml /etc/nself/drill-cloudinit-lkg.yaml
   ```
2. If the diff shows a template change, bisect which CLI release introduced it.
3. If the diff is empty, suspect a Hetzner Debian 12 image change, re-run with a pinned image snapshot.
4. The last known good user-data is persisted at `/etc/nself/drill-cloudinit-lkg.yaml` on nclaw-prod and updated after every green drill.

### Synthetic probe < 100/100 success

1. Capture the probe JSON output and the drill VM Hasura + Postgres logs.
2. Swap DNS back immediately, do not wait the full 15 minutes.
3. File a detail ticket with the failing requests and add the root cause to the lessons file.

## Related

- Monthly drill: `nself dr drill --now`
- Alert rule: `nself dr drill --render-alerts` (the `DRDrillFailed` rule in `web/backend/nself/monitoring/alerts/dr.rules.yml`)
- Report schema: `internal/dr/report.go` (`DrillReport`, `dr_drill_report` table)
