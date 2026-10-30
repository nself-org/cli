#!/bin/sh
# Recording nself stub for the agent E2E sim hosts (P7-SHIP-02); the same
# shape as the promote stub of internal/controlplane/sim: health passes,
# migrations and backups are recorded under /tmp/agent-e2e.
S=/tmp/agent-e2e
mkdir -p "$S"
echo "$*" >> "$S/nself.log"
case "$*" in
  "doctor"*) exit 0 ;;
  "db migrate up"*) printf migrated > "$S/db-fp" ;;
  "backup create --tag "*) cp "$S/db-fp" "$S/db-backup" 2>/dev/null; echo "$4" > "$S/tag" ;;
  "backup list"*) t=$(cat "$S/tag" 2>/dev/null); printf '[{"id":"sim_%s","tag":"%s","date":"2026-10-29T12:00:00Z"}]' "$t" "$t" ;;
  "backup verify "*) exit 0 ;;
  "backup restore "*) cp "$S/db-backup" "$S/db-fp" 2>/dev/null ;;
esac
exit 0
