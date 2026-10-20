package compose

// cachePasswordConfig reads the environment inside awk, so the password never
// becomes an argument. Redis config strings escape quotes, slashes and lines.
const cachePasswordConfig = `umask 077; awk 'BEGIN {
 printf "requirepass \""
 p = ENVIRON["REDISCLI_AUTH"]
 for (i = 1; i <= length(p); i++) {
  c = substr(p, i, 1)
  if (c == "\\" || c == "\"") printf "\\%s", c
  else if (c == "\n") printf "\\n"
  else if (c == "\r") printf "\\r"
  else printf "%s", c
 }
 print "\""
}' > /run/nself-cache/redis.conf && `
