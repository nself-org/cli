package compose

// Purpose: container shell scripts for the SeaweedFS object store (P7-PLUG-60).
// Inputs: none; the scripts read S3_ACCESS_KEY, S3_SECRET_KEY and OBJECT_STORAGE_BUCKETS
// from the container environment.
// Outputs: the server entry script and the one-shot bucket bootstrap script.
// Constraints: every $ the container shell must expand is written $$ so compose does not
// interpolate it (credentials must never land in the rendered command).

// seaweedUser is the uid:gid of the seaweed account baked into the SeaweedFS image.
const seaweedUser = "1000:1000"

const seaweedServerScript = `set -eu
for key in "$${S3_ACCESS_KEY:-}" "$${S3_SECRET_KEY:-}"; do
  case "$$key" in
    ''|*'"'*|*'\'*|*[[:cntrl:]]*)
      printf '%s\n' 'S3_ACCESS_KEY and S3_SECRET_KEY must be nonempty and contain no quotes, backslashes or control characters' >&2
      exit 1 ;;
  esac
done
umask 077
printf '{"identities":[{"name":"nself","credentials":[{"accessKey":"%s","secretKey":"%s"}],"actions":["Admin","Read","Write","List","Tagging"]}]}\n' "$$S3_ACCESS_KEY" "$$S3_SECRET_KEY" > /tmp/s3.json
# Master, volume and filer bind to loopback (no authentication on them); only S3 listens on the network.
exec weed server -dir=/data -ip=127.0.0.1 -ip.bind=127.0.0.1 -s3 -s3.port=8333 -s3.ip.bind=0.0.0.0 -s3.config=/tmp/s3.json`

const seaweedBucketScript = `set -eu
set -f
[ -n "$${OBJECT_STORAGE_BUCKETS:-}" ] || exit 0
case "$$OBJECT_STORAGE_BUCKETS" in
  ,*|*,|*,,*) printf '%s\n' 'Invalid OBJECT_STORAGE_BUCKETS: empty bucket name' >&2; exit 1 ;;
esac
IFS=,
for bucket in $$OBJECT_STORAGE_BUCKETS; do
  if [ "$${#bucket}" -lt 3 ] || [ "$${#bucket}" -gt 63 ]; then
    printf '%s\n' 'Invalid OBJECT_STORAGE_BUCKETS name: expected 3-63 characters' >&2
    exit 1
  fi
  case "$$bucket" in
    *[!a-z0-9.-]*|[!a-z0-9]*|*[!a-z0-9]|*..*|*.-*|*-.*|xn--*|sthree-*|amzn-s3-demo-*|*-s3alias|*--ol-s3|*.mrap|*--x-s3|*--table-s3)
      printf '%s\n' 'Invalid OBJECT_STORAGE_BUCKETS name: expected a lowercase S3 bucket name' >&2
      exit 1 ;;
  esac
  if printf '%s\n' "$$bucket" | awk -F. 'NF == 4 && $$1 ~ /^[0-9]+$$/ && $$2 ~ /^[0-9]+$$/ && $$3 ~ /^[0-9]+$$/ && $$4 ~ /^[0-9]+$$/ { found=1 } END { exit !found }'; then
    printf '%s\n' 'Invalid OBJECT_STORAGE_BUCKETS name: IP address names are not allowed' >&2
    exit 1
  fi
done
for bucket in $$OBJECT_STORAGE_BUCKETS; do
  printf 's3.bucket.list\n' | weed shell -master=127.0.0.1:9333 > /tmp/buckets
  if ! awk '{print $$1}' /tmp/buckets | grep -F -x "$$bucket" >/dev/null; then
    if ! printf 's3.bucket.create -name %s\n' "$$bucket" | weed shell -master=127.0.0.1:9333; then
      printf 'Failed to create OBJECT_STORAGE_BUCKETS bucket: %s\n' "$$bucket" >&2
      exit 1
    fi
  fi
done
printf 's3.bucket.list\n' | weed shell -master=127.0.0.1:9333 > /tmp/buckets
for bucket in $$OBJECT_STORAGE_BUCKETS; do
  if ! awk '{print $$1}' /tmp/buckets | grep -F -x "$$bucket" >/dev/null; then
    printf 'OBJECT_STORAGE_BUCKETS bucket missing after create: %s\n' "$$bucket" >&2
    exit 1
  fi
done`
