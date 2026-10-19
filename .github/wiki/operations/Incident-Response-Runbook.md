# Incident Response Runbook

**Status:** v2 (S9.T01 — P1/P2/P3 SLAs + PagerDuty + full war-room protocol)
**Canonical URL:** `.claude/docs/operations/incident-response-runbook.md`
**Cross-ref:** [`incident-response.md`](incident-response.md) (SEV definitions + decision tree), [`bus-factor.md`](bus-factor.md), [`postmortem-template.md`](postmortem-template.md), [`pagerduty-setup.md`](pagerduty-setup.md), [`runbooks/`](runbooks/) (per-scenario)

---

## 1. Severity Definitions and SLAs

nSelf uses four severity levels. P1/P2/P3 map to the most common operator response patterns.

| Level | Alias | Threshold | Ack SLA | Mitigate SLA | Resolve SLA | Postmortem? | Status Page? |
|---|---|---|---|---|---|---|---|
| **P0 / SEV1** | Total outage | Production down, data loss risk, or confirmed breach. >10% users blocked. | 5 min | 60 min | 4 hours | Required (48h) | YES — every 15 min |
| **P1 / SEV2** | Partial outage | Single-customer or partial outage. <10% users, or one critical-tier customer blocked. Auth/billing broken. | 1 hour | 4 hours | 8 hours | Required (48h) | YES — on detect/mitigate/resolve |
| **P2 / SEV3** | Degraded | User-visible degradation but workaround exists. Feature unavailable, slow response, non-critical plugin offline. | 4 hours | 24 hours | 48 hours | Optional | Optional |
| **P3 / SEV4** | Monitoring | No user impact observed. Issue visible in monitoring only. Cert expiry >14 days, log volume warning. | Next business day | Next sprint | Next sprint | No | No |
| **P4** | Info | Capacity warning, informational signal. No action needed. | — | — | — | No | No |

### Severity Auto-Escalation

- Auth broken for ANY user → P1 minimum.
- Payment/billing broken → P1 minimum.
- Any credential or secret exposed → P0 regardless of user count.
- If uncertain between P1 and P2: pick P1 and downgrade after triage.

---

## 2. Acknowledgment Flow

### Who Acks

nSelf is a single-maintainer ecosystem. Primary on-call is `aric.camarata@gmail.com` (24/7). Backup on-call: per [`bus-factor.md`](bus-factor.md) (USER DECISION PENDING — nomination required for 9 critical services).

### How to Ack

**Via nself-incident-mgmt plugin UI** (preferred when running):

1. Open `https://your-nself-instance/incidents` or `localhost:3833/incidents`
2. Click "Acknowledge" on the triggered alert
3. Fill in: IC name, initial hypothesis, war-room channel name
4. Set status to "Investigating"

**Via Slack** (fallback when UI unavailable):

Post to `#incidents` within ack SLA:
```
:ack: P{N} ack — {one-line summary} — IC: {your name} — {HH:MM UTC}
Opening war room: #incident-{YYYY-MM-DD}-{slug}
```

**Via PagerDuty** (if wired — see [`pagerduty-setup.md`](pagerduty-setup.md)):

1. Accept the page from the PagerDuty mobile app or email
2. Post ack note in the PagerDuty incident: "Investigating — IC: {name}"
3. PagerDuty ack automatically updates nself-incident-mgmt via webhook

### What Goes in the Timeline

Every action during the incident gets a timestamped entry. Minimum data per entry:

```
{HH:MM UTC} | {who} | {what} | {outcome/finding}
```

Examples:
```
14:23 UTC | Aric | Checked Hasura logs — confirmed query timeout spike | 500 errors since 14:17
14:28 UTC | Aric | Restarted Hasura container | Errors clearing
14:35 UTC | Aric | Confirmed resolution — no new errors for 5 min | Mitigated
```

Update the timeline in nself-incident-mgmt UI or in the war-room Slack channel pinned message.

---

## 3. Communication Tree

### Internal Notifications

| Severity | Notify | Channel | Method | Timing |
|---|---|---|---|---|
| P0 | Primary on-call + all engineers | `#incident-{slug}` | PagerDuty page + Slack | Immediately on detection |
| P1 | Primary on-call | `#incidents` + `#incident-{slug}` | PagerDuty or Slack push | Within ack SLA |
| P2 | Primary on-call | `#incidents` | Slack | Within ack SLA |
| P3 | Primary on-call | `#ops-low` | Slack message | Business hours |

### External (Customer) Notifications

| Severity | Status Page | Customer Email | Timing |
|---|---|---|---|
| P0 | Post immediately on detection | Send within 30 min of mitigation | Update every 15 min until resolved |
| P1 | Post on detection | Send within 1h of resolution | Update on detect/mitigate/resolve |
| P2 | Optional | No (unless customer contacts support) | Single update on resolution |
| P3 | No | No | — |

### Status Page Templates (status.nself.org)

**Initial post:**
```
[Investigating] {service} — {one-line symptom} — {start-time UTC}

We are investigating reports of {symptom}. Affected users may see {impact}.

Next update: {now + 15 min}.
```

**Identified:**
```
[Identified] Root cause located — fix in progress.

We identified {cause}. Rollout is underway; impact should clear by {ETA UTC}.

Next update: {now + 15 min}.
```

**Resolved:**
```
[Resolved] Service restored at {time UTC}. Duration: {N} min.

{service} is fully restored. Post-mortem within 48h at {link}.
```

### Customer Email Template (P0/P1 outbound)

```
Subject: nSelf service incident — {one-line summary}

Hi {name or "there"},

We had an issue affecting {service} from {start UTC} to {end UTC} ({N} min).

What happened: {plain-English explanation — no jargon}.
Impact on you: {specific impact if known, else "some requests may have failed"}.
What we did: {plain-English mitigation steps}.
What's next: post-mortem within 48h; we'll send it your way.

If you noticed anything we missed, reply here.

— Aric, nSelf
```

(Tone rule: human, no "we apologize for the inconvenience" — see GCI Outbound Human Correspondence.)

### PagerDuty Escalation Chain

nSelf uses PagerDuty in stub mode — routing is configured but single-escalation for now.

```
Alert triggered (nself-alert-router → PagerDuty)
  └─ Tier 1: Primary on-call (push notification + SMS) — 5 min
       └─ No ack → Tier 2: Backup on-call (see bus-factor.md) — +5 min
            └─ No ack → Escalation policy: email + phone — +10 min
```

See [`pagerduty-setup.md`](pagerduty-setup.md) for integration key setup and test flow.

---

## 4. Investigation Steps

### Step 0 — Open War Room (P0/P1 always; P2 optional)

See War Room Protocol (Section 6).

### Step 1 — Triage: What is broken?

Run in this order:

```bash
# Check overall health
nself health

# Check specific service
nself service status <postgres|hasura|auth|nginx|redis>

# Check recent logs (last 100 lines)
nself logs <service> --tail 100

# Check error rate spike
nself monitor --last 15m
```

### Step 2 — Identify the blast radius

- Which nself.org subdomains affected? (`curl -I https://{subdomain}.nself.org/health`)
- Which plugins returning errors? (Grafana → Plugin Error Rate panel)
- Cloud customers affected? (check `np_cloud_tenants` activity via Hasura)
- License validation broken? (`curl https://ping.nself.org/health`)

### Step 3 — Check recent changes

```bash
# What deployed in the last 2 hours?
cd /Volumes/UG/Sites/nself/cli && git log --oneline --since="2 hours ago"

# Any nself update recently?
nself version && nself update --dry-run
```

### Step 4 — Consult scenario runbooks

Navigate to the matching scenario, then follow it to resolution:

| Symptom | Runbook |
|---|---|
| Postgres down / query errors / deadlock | [`runbooks/postgres-deadlock.md`](runbooks/postgres-deadlock.md) |
| Hetzner server unreachable | [`hetzner-failover-runbook.md`](hetzner-failover-runbook.md) |
| Hasura metadata errors / migration failure | [`hasura-migration-runbook.md`](hasura-migration-runbook.md) |
| Vercel deploy needed / rollback | [`vercel-failover-runbook.md`](vercel-failover-runbook.md) |
| Cloudflare DNS failure / license validation down | [`cloudflare-dns-failure-runbook.md`](cloudflare-dns-failure-runbook.md) |
| Stripe billing broken | [`stripe-failover-runbook.md`](stripe-failover-runbook.md) |
| Secret/credential exposed | [`runbooks/secret-rotation.md`](runbooks/secret-rotation.md) |
| Mass data leak / GDPR trigger | [`runbooks/mass-data-leak.md`](runbooks/mass-data-leak.md) |
| AI provider (OpenAI/Anthropic) unreachable | [`runbooks/ai-provider-outage.md`](runbooks/ai-provider-outage.md) |
| License server (ping.nself.org) down | [`runbooks/license-server-outage.md`](runbooks/license-server-outage.md) |
| Malicious plugin behavior detected | [`runbooks/malicious-plugin-response.md`](runbooks/malicious-plugin-response.md) |
| No matching runbook | Use Root-Cause Template below as live working doc |

### Step 5 — Mitigation actions (most common)

**Rollback a bad deploy:**
```bash
# Vercel — rollback to prior production deployment
vercel rollback --prod

# CLI fix pushed to prod — roll back to prior tag
cd /Volumes/UG/Sites/nself/cli
git tag --sort=-creatordate | head -5  # find prior tag
# then trigger deploy via nself deploy with prior version
```

**Restart a crashed service:**
```bash
nself service restart <service-name>
```

**Enable graceful degradation (disable a bad plugin):**
```bash
nself plugin disable <plugin-name>
```

**Route around a failing dependency:**
See specific runbooks for Hetzner / Cloudflare / Vercel failovers.

---

## 5. Post-Mortem

### When

- P0: required, within 48 hours.
- P1: required, within 48 hours.
- P2: optional but recommended for recurring issues.
- P3/P4: skip.

### Format (blameless)

Use [`postmortem-template.md`](postmortem-template.md) verbatim. Key sections:

```markdown
## Post-Mortem: {one-line title}

**Date:** {YYYY-MM-DD}
**Severity:** P{N}
**Duration:** {start UTC} → {resolved UTC} ({N} min)
**IC:** {name}
**Authors:** {names}

### Impact
{Quantified user impact: N users affected, N min downtime, N failed requests.}

### Timeline
{Timestamped list — detection through resolution.}

### Root Cause
{Specific, factual. Use 5-Whys: keep asking "why" until you reach a process or system gap.}

### What Went Well
{Be honest — what actually helped?}

### What Went Wrong
{Be honest — what slowed response or caused the gap?}

### Action Items

| Action | Owner | Due | Status |
|---|---|---|---|
| {specific preventive or detective fix} | {name} | {YYYY-MM-DD} | open |
```

### Rules

- Blameless means no "X did the wrong thing." Focus on system and process gaps.
- Every action item gets an owner and due date. Track in `.claude/tasks/active.md` until closed.
- Action items are always one specific, verifiable fix — never vague ("improve monitoring").
- Publish within 48 hours. Send link to affected customers by email.

---

## 6. War Room Protocol

### When to open

- P0: always.
- P1: always.
- P2: at IC discretion.

### Setup (under 3 minutes)

**1. Create the Slack channel:**
```
#incident-{YYYY-MM-DD}-{slug}   e.g.  #incident-2026-05-07-auth-down
```

Channel topic:
```
P{N} | {one-line summary} | IC: {name} | Zoom: {url} | started {HH:MM UTC}
```

**2. Assign roles:**

| Role | Responsibility | Required? |
|---|---|---|
| **Incident Commander (IC)** | Owns the incident. Drives triage, calls mitigation, declares resolution. | Always |
| **Technical Lead** | Runs commands, reads logs, executes the runbook steps. | P0/P1 |
| **Scribe** | Posts timestamped updates every 15 min. Updates status page. | P0/P1 |
| **Comms Officer** | Drafts customer messages. Handles @-mentions + support tickets. | P0 (optional P1) |

For single-maintainer: IC = Technical Lead. Scribe and Comms can be the same person or deferred until mitigation is underway.

**3. Open Zoom bridge:**

Use the standing bridge URL stored in the operator's secret store (`INCIDENT_ZOOM_URL`).

Paste the URL in the channel topic immediately. For P2: text-only in Slack is fine.

**4. Pin the root-cause template:**

Paste into the channel as a pinned message:
```markdown
## Incident {YYYY-MM-DD} — {slug}

**P-level:** P{N}
**IC:** {name}
**Started:** {HH:MM UTC}
**Mitigated:** pending
**Resolved:** pending

### Symptoms
- {bullet each user-visible signal with timestamp}

### Affected surfaces
- [ ] {service / endpoint / customer segment}

### Hypotheses (live — update as ruled out or confirmed)
- H1: {hypothesis} — Evidence: {link/paste} — Status: open
- H2: …

### Actions taken
- {HH:MM} {action} → {outcome}
```

### Status Update Cadence

| Severity | Internal (Slack) | External (status page) |
|---|---|---|
| P0 | Every 10 min | Every 15 min |
| P1 | Every 30 min | On state change |
| P2 | On state change | On resolution |

Update format for internal:
```
[HH:MM UTC] Update #{N}: {one-line current state}. Next: {what we're doing now}.
```

### Declaring Resolution

The IC declares resolution when:
1. The user-visible symptom is confirmed gone (not just "looks better").
2. Root cause is identified and either fixed or safely mitigated.
3. No new errors in the past 10 min (P0) or 5 min (P1).

Say explicitly in the channel:
```
:white_check_mark: RESOLVED — P{N} resolved at {HH:MM UTC}. Duration: {N} min.
Post-mortem due: {YYYY-MM-DD HH:MM UTC}.
```

Then: post status page resolution, send customer email if required, and file the post-mortem task in `.claude/tasks/active.md`.

---

## Quick Links

- [nself-incident-mgmt UI](http://localhost:3833/incidents) — incident ack + timeline
- [status.nself.org](https://status.nself.org) — public status page
- [PagerDuty setup](pagerduty-setup.md) — alert routing + integration key
- [postmortem-template.md](postmortem-template.md) — blameless format
- [on-call-rotation.md](on-call-rotation.md) — who is primary on-call
- [vendor-contacts.md](vendor-contacts.md) — Cloudflare / Hetzner / Vercel / Stripe escalation
- [bus-factor.md](bus-factor.md) — backup admin assignments
- [dr-runbook.md](dr-runbook.md) — full disaster recovery
