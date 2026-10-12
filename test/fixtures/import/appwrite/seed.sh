#!/usr/bin/env bash
# Seed the appwrite fixture through its REST API (P7-ADOPT-16).
# Usage: ENDPOINT=http://localhost:PORT/v1 FIXTURE_PASSWORD=... seed.sh OUTDIR
# Writes OUTDIR/read.key (read-only API key, the one the import uses) and OUTDIR/seed.json.
# Everything is synthetic: fake hashes, fake e-mail addresses, no real secret.
set -euo pipefail
OUT=${1:?output dir}
mkdir -p "$OUT"
jar=$OUT/console.jar
H=(-H "Content-Type: application/json" -H "X-Appwrite-Response-Format: 1.5.0")
api() { # api METHOD PATH [json] with extra args in $EXTRA
  local m=$1 p=$2 d=${3:-}
  if [[ -n $d ]]; then curl -fsS -X "$m" "$ENDPOINT$p" "${H[@]}" "${EXTRA[@]}" -d "$d"; else curl -fsS -X "$m" "$ENDPOINT$p" "${H[@]}" "${EXTRA[@]}"; fi
}
EXTRA=(-H "X-Appwrite-Project: console" -c "$jar" -b "$jar")
api POST /account "{\"userId\":\"admin1\",\"email\":\"admin@fixture.invalid\",\"password\":\"${FIXTURE_PASSWORD}\",\"name\":\"Admin\"}" >/dev/null
api POST /account/sessions/email "{\"email\":\"admin@fixture.invalid\",\"password\":\"${FIXTURE_PASSWORD}\"}" >/dev/null
api POST /teams '{"teamId":"team1","name":"Fixture"}' >/dev/null
api POST /projects '{"projectId":"proj1","name":"Fixture","teamId":"team1","region":"default"}' >/dev/null
WRITE_SCOPES='"databases.read","databases.write","collections.read","collections.write","attributes.read","attributes.write","documents.read","documents.write","users.read","users.write","buckets.read","buckets.write","files.read","files.write"'
READ_SCOPES='"databases.read","collections.read","attributes.read","documents.read","users.read","buckets.read","files.read"'
wkey=$(api POST /projects/proj1/keys "{\"name\":\"seed\",\"scopes\":[$WRITE_SCOPES]}" | jq -r .secret)
api POST /projects/proj1/keys "{\"name\":\"import\",\"scopes\":[$READ_SCOPES]}" | jq -r .secret > "$OUT/read.key"
chmod 600 "$OUT/read.key"
EXTRA=(-H "X-Appwrite-Project: proj1" -H "X-Appwrite-Key: $wkey")
api POST /databases '{"databaseId":"main","name":"Main"}' >/dev/null
api POST /databases/main/collections '{"collectionId":"authors","name":"Authors","documentSecurity":true,"permissions":["read(\"any\")"]}' >/dev/null
api POST /databases/main/collections '{"collectionId":"books","name":"Books","documentSecurity":true,"permissions":[]}' >/dev/null
api POST /databases/main/collections/authors/attributes/string '{"key":"name","size":64,"required":true}' >/dev/null
api POST /databases/main/collections/authors/attributes/string '{"key":"bio","size":255,"required":false}' >/dev/null
api POST /databases/main/collections/books/attributes/string '{"key":"title","size":128,"required":true}' >/dev/null
api POST /databases/main/collections/books/attributes/integer '{"key":"pages","required":false}' >/dev/null
api POST /databases/main/collections/books/attributes/float '{"key":"rating","required":false}' >/dev/null
api POST /databases/main/collections/books/attributes/boolean '{"key":"in_stock","required":false}' >/dev/null
api POST /databases/main/collections/books/attributes/datetime '{"key":"released","required":false}' >/dev/null
api POST /databases/main/collections/books/attributes/email '{"key":"contact","required":false}' >/dev/null
api POST /databases/main/collections/books/attributes/relationship '{"relatedCollectionId":"authors","type":"manyToOne","twoWay":false,"key":"author","onDelete":"setNull"}' >/dev/null
# Attributes are created by the databases worker: wait until every one is available.
for c in authors books; do
  for _ in $(seq 1 60); do
    n=$(api GET "/databases/main/collections/$c/attributes" | jq '[.attributes[] | select(.status!="available")] | length')
    [[ $n == 0 ]] && break
    sleep 2
  done
  test "$n" = 0
done
for i in 1 2 3 4 5; do
  api POST /databases/main/collections/authors/documents "{\"documentId\":\"a$i\",\"data\":{\"name\":\"Author $i\",\"bio\":\"Bio of \\\"author\\\" $i, with a comma\"},\"permissions\":[]}" >/dev/null
done
for i in $(seq 1 25); do
  perms='[]'
  [[ $i == 1 ]] && perms='["read(\"any\")"]'
  api POST /databases/main/collections/books/documents "{\"documentId\":\"b$i\",\"data\":{\"title\":\"Book $i\",\"pages\":$((100 + i)),\"rating\":4.$((i % 10)),\"in_stock\":$([[ $((i % 2)) == 0 ]] && echo true || echo false),\"released\":\"2025-01-$(printf '%02d' $((i % 28 + 1)))T00:00:00.000+00:00\",\"contact\":\"book$i@fixture.invalid\",\"author\":\"a$(((i % 5) + 1))\"},\"permissions\":$perms}" >/dev/null
done
# Four users: two bcrypt, two argon2 (synthetic hash strings; Appwrite stores them as given).
bc='$2a$10$fixturefixturefixturefOabcdefghijklmnopqrstuvwxyz0123'
ar='$argon2id$v=19$m=2048,t=4,p=1$Zml4dHVyZXNhbHQ$Zml4dHVyZWhhc2hmaXh0dXJlaGFzaDAwMDA'
api POST /users/bcrypt "{\"userId\":\"ub1\",\"email\":\"bcrypt1@fixture.invalid\",\"password\":\"$bc\",\"name\":\"Bcrypt One\"}" >/dev/null
api POST /users/bcrypt "{\"userId\":\"ub2\",\"email\":\"bcrypt2@fixture.invalid\",\"password\":\"$bc\",\"name\":\"Bcrypt Two\"}" >/dev/null
api POST /users/argon2 "{\"userId\":\"ua1\",\"email\":\"argon1@fixture.invalid\",\"password\":\"$ar\",\"name\":\"Argon One\"}" >/dev/null
api POST /users/argon2 "{\"userId\":\"ua2\",\"email\":\"argon2@fixture.invalid\",\"password\":\"$ar\",\"name\":\"Argon Two\"}" >/dev/null
api POST /storage/buckets '{"bucketId":"files","name":"Files","maximumFileSize":1000000,"fileSecurity":true,"permissions":["read(\"any\")"]}' >/dev/null
for i in 1 2 3; do
  printf 'fixture file %s\n' "$i" > "$OUT/file$i.txt"
  curl -fsS -X POST "$ENDPOINT/storage/buckets/files/files" -H "X-Appwrite-Project: proj1" -H "X-Appwrite-Key: $wkey" -F "fileId=f$i" -F "file=@$OUT/file$i.txt;filename=file$i.txt" >/dev/null
done
echo '{"databases":1,"collections":2,"documents":30,"users":4,"buckets":1,"files":3}' > "$OUT/seed.json"
