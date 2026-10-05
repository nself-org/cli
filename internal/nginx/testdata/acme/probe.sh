set -u
nginx -t 2>&1 || { echo "PROBE-FAIL nginx -t"; exit 1; }
nginx
sleep 1
fail=0
# want <status> <label> <curl args...>: the status code must match.
want() {
  st=$1; label=$2; shift 2
  got=$(curl -s -o /tmp/body -w '%{http_code}' --max-time 5 "$@")
  if [ "$got" != "$st" ]; then echo "PROBE-FAIL $label: HTTP $got, want $st"; fail=1; else echo "ok $label: $got"; fi
}
U=http://127.0.0.1/.well-known/acme-challenge
want 200 "bare token" "$U/tok-OK_1"
grep -qx proof /tmp/body || { echo "PROBE-FAIL token body"; fail=1; }
want 404 "unknown token" "$U/nope"
want 404 "directory" "$U/"
want 404 "subdirectory file" "$U/sub/deep"
want 404 "dotted token" "$U/a.b"
want 404 "encoded slash" "$U/sub%2fdeep"
# %2e%2e%2f normalises out of the prefix: it must not be answered from the webroot.
curl -s --max-time 5 "$U/%2e%2e%2ftok-OK_1" | grep -qx proof && { echo "PROBE-FAIL encoded dots reached the webroot"; fail=1; } || echo "ok encoded dots leave the prefix"
want 403 "POST" -X POST "$U/tok-OK_1"
want 200 "HEAD" -I "$U/tok-OK_1"
want 200 "route host" -H 'Host: api.example.com' "$U/tok-OK_1"
# a traversal must never return a file from outside the webroot
got=$(curl -s --path-as-is --max-time 5 "$U/../../../../etc/passwd" || true)
case "$got" in *root:*) echo "PROBE-FAIL traversal served /etc/passwd"; fail=1;; *) echo "ok traversal not served";; esac
got=$(curl -s --path-as-is --max-time 5 "$U/..%2f..%2f..%2fetc/passwd" || true)
case "$got" in *root:*) echo "PROBE-FAIL encoded traversal served /etc/passwd"; fail=1;; *) echo "ok encoded traversal not served";; esac
# the default server answers the rest as before
want 200 "default /health" http://127.0.0.1/health
[ "$fail" = 0 ] && echo PROBE-PASS
exit $fail
