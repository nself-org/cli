# Re-capture the OLD cluster after the restore: same documents as dump.sh, no dump, suffix "chk".
# A difference from the "old" documents means something wrote after the dump (E510).
. /stage/common.sh
globals chk
capture_cluster chk
i=0
while IFS= read -r db; do
  $PSQL -d "$db" -f "$S/capdb.sql" > "$S/db-$i.cap.chk.json"
  pg_dump -s -f "$S/db-$i.schema.chk.sql" -d "$db"
  i=$((i + 1))
done < "$S/dblist.txt"
