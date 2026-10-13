# Dump every database inside one exported snapshot per database. Output, all under /stage:
#   globals.old.sql, cluster.old.json, then per database i: db-i.dump (pg_dump -Fc),
#   db-i.cap.old.json (capdb.sql in the snapshot), db-i.schema.old.sql (pg_dump -s in the snapshot).
. /stage/common.sh
globals old
capture_cluster old
i=0
while IFS= read -r db; do
  coproc PS { $PSQL -d "$db"; }
  echo "BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;" >&"${PS[1]}"
  echo "SELECT pg_export_snapshot();" >&"${PS[1]}"
  IFS= read -r SNAP <&"${PS[0]}"
  printf '\\o %s/db-%s.cap.old.json\n\\i %s/capdb.sql\n\\o\nSELECT %s;\n' "$S" "$i" "$S" "'CAPTURED'" >&"${PS[1]}"
  IFS= read -r DONE <&"${PS[0]}"
  [ "$DONE" = CAPTURED ] || { echo "capture of database $i failed" >&2; exit 1; }
  pg_dump -Fc --snapshot="$SNAP" -f "$S/db-$i.dump" -d "$db"
  pg_dump -s --snapshot="$SNAP" -f "$S/db-$i.schema.old.sql" -d "$db"
  echo "COMMIT;" >&"${PS[1]}"
  echo '\q' >&"${PS[1]}"
  wait "$PS_PID"
  i=$((i + 1))
done < "$S/dblist.txt"
