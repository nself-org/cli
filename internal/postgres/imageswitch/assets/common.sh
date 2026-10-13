# Shared by dump.sh and recheck.sh. Runs in the target pgvector image as the host uid:gid, on the
# project network, with PGHOST/PGUSER/PGPASSWORD in the environment (the password never in argv).
# Inputs: /stage/dblist.txt (one database per line), /stage/capcluster.sql, /stage/capdb.sql.
set -euo pipefail
umask 077
S=/stage
PSQL="psql -X -q -At -v ON_ERROR_STOP=1"
# capture_cluster OUT_SUFFIX: roles, memberships, databases, tablespaces as one JSON line.
capture_cluster() { $PSQL -d template1 -f "$S/capcluster.sql" > "$S/cluster.$1.json"; }
# globals OUT_SUFFIX: pg_dumpall --globals-only (roles, with their password hashes: the file is 0600).
globals() { pg_dumpall --globals-only -f "$S/globals.$1.sql"; }
