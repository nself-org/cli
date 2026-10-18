# CLI HTTP API v1

`nself admin serve [--host 127.0.0.1] [--port 3857] [--allowed-host <name>]…
[--timeout 300s]` serves one listener. Non-loopback binds require an explicitly
configured machine token of at least 32 bytes with no whitespace, and an allowed
host on every start, and print a cleartext
HTTP warning. Loopback starts resolve or create the machine token and print its
file path only. Tokens are never arguments or request log fields.
Weak non-loopback tokens are refused with E426 on every start; the Fix points to
`~/.nself/mcp-token`, created by running `nself admin serve` on loopback.
The listening line prints only after the listener binds successfully. A bind
failure returns an error without printing that line.

`GET /openapi.json` and `POST /v1/commands/<token>/<token>…`
(`cmdregistry.RoutePath`) require `Authorization: Bearer <token>`. POST bodies
are machine requests with `Content-Type: application/json`, at most 1 MiB.
Requests have optional `args` (strings), `flags` (typed values), and `confirm`
(server nonce or plan ID). A leaf plugin without declared parameters instead
accepts `argv`. Unknown members and invalid types are rejected; callers cannot
set invoker-owned consent or plan flags.

Document responses use `application/json` and preserve the envelope bytes from
`NSELF_V15=1 nself <path> --json`. Confirmations and server refusals are v1
envelopes. `X-Nself-Exit-Code` carries the child exit status or the server error's
exit class; `X-Nself-Request-Id` carries the invocation request ID. A success
confirmation issued by the invoker requires HTTP 428 and a second
request carrying the returned `confirm`. One server guard owns confirmations and
the shared machine audit log; every call passes through `internal/invoke`.
HTTP 428 comes only from trusted `Result.Confirmation`, never child data. Envelope
keys `error`, `code`, `class` and `data` are matched exactly, including case.

`config set`, `config trust` and `config trust dns` are cli-only until they gain
a confirmation round trip (D-0403/D-0402). They have no HTTP routes exposed or
OpenAPI operations in v1.

`Accept: application/x-ndjson` requests exposed read commands whose effective
output is stream, including document commands with an enabled stream override.
Transport selection uses the command's effective output and request flags. A
stream command selects NDJSON when that media type has positive quality. Other
requests use documents when JSON or a JSON wildcard is acceptable; a document
command accepting only NDJSON receives 406 E430. Mixed JSON and NDJSON Accept
values keep document commands on JSON. Stream commands requested as JSON are
refused by the invoker with E421.
The response carries the child's JSON stream records and final envelope,
flushes each write, and has no invocation timeout. The child is killed within
2 seconds after client disconnect. Once streaming begins the status is 200 and
`X-Nself-Exit-Code` is a trailer carrying the child exit code only when invocation
completes without an error. Cancellation or another invocation error sets it to
`-1`. A refusal before the first byte is a normal JSON error response with its
mapped status. If a child writes no bytes and returns empty stdout, the server
answers with an E422 v1 error envelope and HTTP 502.

Status selection uses error code first: E420/E425/E430 → 400, E426 → 401,
E429 → 403, E421/E432 → 404, E424/E460 → 409, E428 → 421, E431 → 503
with `Retry-After: 1`, E423 → 504. Confirmation success → 428. Remaining error
classes map user → 422, auth → 403, destructive_blocked → 409, infra → 502,
other → 500. A data envelope maps to 200, including state exits 10–12. HTTP
request validation can specialize E430 to 405, 406, 413 or 415.

The host allowlist contains `localhost`, `127.0.0.1`, `[::1]` and each
`--allowed-host`, with the listener port. Host matching is case-insensitive.
Invalid allowed-host values use E430 with Why: "an allowed host is a DNS name,
an IPv4 literal or a bracketed IPv6 literal, without scheme, port or path" and
Fix: `pass --allowed-host <name>; the server adds the port itself`.
A present Origin must be `http://<allowed host>:<port>`; `null` is refused.
Requests with more than one Origin header receive HTTP 403 and E429 before
authentication.
Host, Origin and authentication refusals always use an empty envelope command
and a request ID computed from the empty path, independent of route existence.
No Access-Control-* header is sent. Middleware order is Host, Origin,
AuthHook, method, content type, size, strict decode, Accept, then a maximum of
four in-flight invocations. Unknown registry paths return E432; existing but
unexposed registry paths return E421 with the exposure reason in `cause`.
Mounted handlers and OpenAPI use the same Host, Origin and AuthHook chain.

`AuthHook.Authenticate(*http.Request) (Principal, error)` permits P8 to add
session authentication and CSRF checks on the same listener. P7 implements
bearer authentication with constant-time token comparison. Empty tokens refuse
every request. Authentication runs before any body read. Authentication refusals
carry `WWW-Authenticate: Bearer realm="nself"`.

The server defaults to a 10-second ReadHeaderTimeout, 30-second ReadTimeout,
60-second IdleTimeout, and no WriteTimeout. Context cancellation shuts it down
gracefully for up to 5 seconds before closing active HTTP connections. After the
HTTP server stops, shutdown drains invocation handlers, including child cleanup
and mutation audit writes, for up to 11 additional seconds (5-second document
kill delay plus 5-second audit flock bound plus 1 second). Serve returns after
the drain completes; a drain exceeding the cap logs one line. Each request logs
one stderr line containing method,
path truncated to 200 bytes, status, and duration in milliseconds. No body,
header, query string or token is logged.
Non-context invocation errors add one stderr line, `invoke error: <Go error type
name>`. The error message is never logged, and the per-request line stays unchanged.

OpenAPI is 3.1.0 with `info.version: "1"`, global bearer security. Each exposed document command has one POST whose
operationId is `cmdregistry.ToolName`, request schema is the machine params
schema, 200 data references a copied `<ToolName>_data` component, 428 references
the confirmation schema, and default references the error envelope. GET
`/openapi.json` is also documented. OpenAPI v1 is document-only: stream-read
commands are served as NDJSON but have no OpenAPI operation in v1.
Stream-override flags are described by the stream contract above. Keys are
sorted, indentation is two spaces, and the bytes end in a newline. There is no
timestamp or CLI version. Under an isolated HOME and plugin directory, served
bytes equal `.github/openapi.json`.
Generation rejects exposed command paths whose `cmdregistry.ToolName` collides
with another exposed path or exceeds 64 characters.

## Synthetic request and response examples

All examples use fake values. The normal authenticated document request is:

```http
POST /v1/commands/config/get HTTP/1.1
Host: 127.0.0.1:3857
Authorization: Bearer fake-token-0000
Content-Type: application/json
Accept: application/json

{"args":["BASE_DOMAIN"]}
```

Its 200 response has `X-Nself-Exit-Code: 0` and a synthetic envelope:

```json
{"schema_version":"1","command":"config get","data":{"key":"BASE_DOMAIN","value":"fake.example","masked":false,"file":".env"}}
```

The following request variants illustrate every status class. Error response
bodies use the v1 envelope shown after the table; class and exit values follow
the error registry. Request IDs shown in headers are synthetic.

| HTTP | Request example | Response example |
| --- | --- | --- |
| 200 | Normal request above; a state command can exit 10–12 | Data envelope, `X-Nself-Exit-Code: 10` for a state exit |
| 400 | Same POST, body `{"unknown":true}` | E420, class user, exit 1 |
| 401 | Same POST, omit Authorization | E426, class auth, exit 3; `WWW-Authenticate: Bearer realm="nself"` |
| 403 | Same POST, `Origin: null` | E429, class user, exit 1; other auth errors use class auth |
| 404 | POST `/v1/commands/nope` | E432, class user, exit 1; `config/set` gives E421 and its exposure reason |
| 405 | GET `/v1/commands/config/get` | E430, class user, exit 1; `Allow: POST` |
| 406 | Same POST, `Accept: text/html` | E430, class user, exit 1 |
| 409 | Destructive command POST with expired fake confirmation | E424, class destructive_blocked, exit 4; E460 also maps here |
| 413 | Same POST with a body larger than 1 MiB | E430, class user, exit 1 |
| 415 | Same POST, `Content-Type: text/plain` | E430, class user, exit 1 |
| 421 | Same POST, `Host: evil.example:3857` | E428, class user, exit 1 |
| 422 | Valid machine request rejected by command validation | Error class user, exit 1 |
| 428 | Destructive command POST without confirmation | Success confirmation envelope below, exit 0 |
| 500 | Valid POST whose command reports an unclassified error | Error class other, exit 2 |
| 502 | Valid POST whose child reports an infrastructure failure | Error class infra, exit 2 |
| 503 | Fifth POST while four invocations are blocked | E431, class infra, exit 2; `Retry-After: 1` |
| 504 | Valid POST whose document child exceeds its timeout | E423, class infra, exit 2 |

An E432 example (replace code, message, class and exit for the other error rows):

```json
{"schema_version":"1","command":"","error":{"code":"E432","message":"no such route","class":"user","exit_code":1}}
```

The synthetic confirmation response to a destructive POST without consent is:

```json
{"schema_version":"1","command":"destroy","data":{"confirmation_required":true,"kind":"request","confirm":"0000000000000000000000000000000000000000000000000000000000000000","side_effect":"destructive","reason":"confirmation required","plan":null}}
```

Repeat the same request with that synthetic `confirm` value to illustrate the
round trip; real confirmations must come from the running server. A streamed
request uses `Accept: application/x-ndjson` and, when declared by the command,
`{"flags":{"follow":true}}`. Records arrive as separate JSON lines followed
by a final v1 envelope; the exit code arrives in the declared trailer.
