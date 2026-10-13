# Restore into the EMPTY new volume inside a one-shot postgres server (no init scripts, no network,
# no published port), then capture the restored cluster for the comparison.
# Runs as root in the target image so it can read the 0600 stage files; the server itself runs as
# the postgres user (gosu). Inputs from the environment: BOOT_USER, NSELF_COLLATE, NSELF_CTYPE,
# HOST_UID, HOST_GID. The stage is handed back to the host uid:gid on every exit.
set -euo pipefail
umask 077
S=/stage
trap 'chown -R "$HOST_UID:$HOST_GID" "$S" || true' EXIT
PGDATA=/var/lib/postgresql/data/pgdata
export PGHOST=/tmp PGUSER="$BOOT_USER" PGDATA
gosu postgres initdb -D "$PGDATA" -U "$BOOT_USER" --encoding=UTF8 --lc-collate="$NSELF_COLLATE" --lc-ctype="$NSELF_CTYPE" > /dev/null 2>&1
# What the image entrypoint appends when it initialises a volume itself (compose sets scram-sha-256).
echo "host all all all scram-sha-256" >> "$PGDATA/pg_hba.conf"
gosu postgres pg_ctl -D "$PGDATA" -w -s -l /tmp/pg.log -o "-c listen_addresses='' -c unix_socket_directories=/tmp -c max_parallel_workers_per_gather=0" start
PSQL="psql -X -q -At -v ON_ERROR_STOP=1"
$PSQL -d template1 -f "$S/globals.restore.sql"
$PSQL -d template1 -f "$S/create-dbs.sql"
i=0
while IFS= read -r db; do
  pg_restore --exit-on-error --no-password -d "$db" "$S/db-$i.dump"
  i=$((i + 1))
done < "$S/dblist.txt"
$PSQL -d template1 -f "$S/replay.sql"
$PSQL -d template1 -f "$S/capcluster.sql" > "$S/cluster.new.json"
pg_dumpall --globals-only -f "$S/globals.new.sql"
i=0
while IFS= read -r db; do
  $PSQL -d "$db" -f "$S/capdb.sql" > "$S/db-$i.cap.new.json"
  pg_dump -s -f "$S/db-$i.schema.new.sql" -d "$db"
  i=$((i + 1))
done < "$S/dblist.txt"
gosu postgres pg_ctl -D "$PGDATA" -w -s -m fast stop
