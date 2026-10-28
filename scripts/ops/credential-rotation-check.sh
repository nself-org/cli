#!/usr/bin/env bash
# credential-rotation-check.sh — Scan credential-rotation.md cadence matrix and
# emit PCI messages for credentials due within 14 days.
#
# Reads:  .claude/docs/operations/credential-rotation.md (rotation calendar)
# Writes: .claude/inbox/msg-YYYY-MM-DD-rotation-<credential-slug>.md (PCI per credential)
# Output: GH Actions step summary + JSON line per finding to stdout
#
# Usage: credential-rotation-check.sh [--dry-run] [--ppi-root <path>]
#
# Exit codes:
#   0 — scan completed (zero or more findings emitted)
#   1 — fatal error (missing file, parse failure)
#   2 — invalid args
set -euo pipefail

# ---- defaults
PPI_ROOT="${PPI_ROOT:-/Volumes/UG/Sites/nself}"
DRY_RUN="${DRY_RUN:-0}"

# ---- arg parse
while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run)   DRY_RUN=1; shift ;;
    --ppi-root)  PPI_ROOT="$2"; shift 2 ;;
    -h|--help)
      sed -n '2,12p' "$0"; exit 0 ;;
    *)
      echo "Unknown arg: $1" >&2; exit 2 ;;
  esac
done

CADENCE_FILE="${PPI_ROOT}/.claude/docs/operations/credential-rotation.md"
INBOX_DIR="${PPI_ROOT}/.claude/inbox"
TODAY="$(date -u +%Y-%m-%d)"
TODAY_EPOCH="$(date -u -j -f %Y-%m-%d "$TODAY" +%s 2>/dev/null || date -u -d "$TODAY" +%s)"

[[ -f "$CADENCE_FILE" ]] || { echo "Missing cadence file: $CADENCE_FILE" >&2; exit 1; }
mkdir -p "$INBOX_DIR"

# ---- helpers
days_until() {
  # $1 = YYYY-MM
  local due_year="${1%-*}" due_month="${1#*-}"
  # End-of-month 15th as canonical due date (mid-month anchor)
  local due_date="${due_year}-${due_month}-15"
  local due_epoch
  due_epoch="$(date -u -j -f %Y-%m-%d "$due_date" +%s 2>/dev/null || date -u -d "$due_date" +%s)"
  echo $(( (due_epoch - TODAY_EPOCH) / 86400 ))
}

slugify() {
  # Lowercase, replace non-alnum with dash, collapse, trim
  printf '%s' "$1" \
    | tr '[:upper:]' '[:lower:]' \
    | sed -E 's/[^a-z0-9]+/-/g; s/^-+|-+$//g'
}

emit_pci() {
  local credential="$1" days="$2" priority="$3" cadence="$4" next_due="$5" owner="$6"
  local slug; slug="$(slugify "$credential")"
  local subject; subject="[ROTATION-DUE] ${credential} expires in ${days} days"
  local msg_file="${INBOX_DIR}/msg-${TODAY}-rotation-${slug}.md"

  if [[ "$DRY_RUN" = "1" ]]; then
    printf '{"credential":"%s","days":%d,"priority":"%s","slug":"%s","action":"dry-run"}\n' \
      "$credential" "$days" "$priority" "$slug"
    return 0
  fi

  cat > "$msg_file" <<EOF
---
Subject: ${subject}
From: credential-rotation-cron[GitHub Actions] <ops@nself-org>
To: nself-ppi
Date: ${TODAY}
Priority: ${priority}
Type: info
Chain: credential-rotation-${TODAY}
---

## Context

Scheduled credential rotation reminder. The credential **\`${credential}\`** is due
for rotation in **${days} days** (cadence: ${cadence}, next due: ${next_due}, owner: ${owner}).

## Action Required

Rotate the credential before the due date to avoid service interruption.

## Rotation Procedure

See [\`credential-rotation.md\`](../docs/operations/credential-rotation.md) for the
general 8-step procedure. Per-credential specifics live in
[\`secrets-rotation.md\`](../docs/operations/secrets-rotation.md).

Summary:

1. Verify backup admin is available (see [\`bus-factor.md\`](../docs/operations/bus-factor.md)).
2. Generate new credential at vendor console (parallel to existing).
3. Stage as \`${credential}_NEW\` in your credential store.
4. Roll dependents (Vercel env, GH Actions secrets, Hetzner \`.env.secrets\`, CRD config).
5. Verify new credential works via a real action.
6. Revoke old credential at vendor.
7. Update rotation calendar in \`credential-rotation.md\` with new anchor month.
8. Drop the \`_NEW\` suffix.

## Vault Variable

Update in your secrets file:

\`\`\`bash
${credential}=<new-value>
\`\`\`

## Downstream Impact

Consumers that depend on this credential must be redeployed or have their secrets
refreshed after rotation. Refer to the consumer list in \`secrets-rotation.md\` for
the specific credential class.

## Links

- [Rotation calendar](../docs/operations/credential-rotation.md)
- [Bus-factor doctrine](../docs/operations/bus-factor.md)
- [Secrets rotation procedures](../docs/operations/secrets-rotation.md)
EOF

  printf '{"credential":"%s","days":%d,"priority":"%s","slug":"%s","msg_file":"%s","action":"emitted"}\n' \
    "$credential" "$days" "$priority" "$slug" "$msg_file"
}

# ---- parse rotation calendar
# Calendar table: | Credential | Cadence | Anchor month | Next rotation due | Owner |
# Skip header + divider rows. Strip backticks and surrounding spaces.
FINDINGS=0
while IFS='|' read -r _ cred cadence anchor next_due owner _; do
  cred="$(printf '%s' "$cred" | sed -E 's/^[[:space:]]+|[[:space:]]+$//g; s/`//g')"
  cadence="$(printf '%s' "$cadence" | sed -E 's/^[[:space:]]+|[[:space:]]+$//g')"
  next_due="$(printf '%s' "$next_due" | sed -E 's/^[[:space:]]+|[[:space:]]+$//g')"
  owner="$(printf '%s' "$owner" | sed -E 's/^[[:space:]]+|[[:space:]]+$//g')"

  # Filter: only YYYY-MM next_due values
  [[ "$next_due" =~ ^[0-9]{4}-[0-9]{2}$ ]] || continue
  [[ -z "$cred" ]] && continue

  days="$(days_until "$next_due")"

  if (( days < 7 )); then
    priority="high"
  elif (( days <= 14 )); then
    priority="medium"
  else
    continue
  fi

  emit_pci "$cred" "$days" "$priority" "$cadence" "$next_due" "$owner"
  FINDINGS=$((FINDINGS + 1))
done < <(grep -E '^\|[^|]+\|[[:space:]]*(90d|180d|annual)[[:space:]]*\|' "$CADENCE_FILE" || true)

# ---- step summary (GH Actions)
if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
  {
    echo "## Credential Rotation Scan — ${TODAY}"
    echo
    echo "- Findings: ${FINDINGS}"
    echo "- Dry-run: ${DRY_RUN}"
    echo "- Source: \`${CADENCE_FILE#${PPI_ROOT}/}\`"
  } >> "$GITHUB_STEP_SUMMARY"
fi

echo "credential-rotation-check: ${FINDINGS} finding(s) emitted (dry_run=${DRY_RUN})" >&2
exit 0
