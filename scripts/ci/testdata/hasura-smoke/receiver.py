"""Synthetic receivers for hasura-upgrade-smoke.sh (P7-LIVE-23).

Purpose: two tiny HTTP servers in one process, run by the smoke in a throwaway
container on the project network (alias "receiver"):
  :9100  event trigger webhook: every POST body is appended to /tmp/events.jsonl
  :9101  remote schema: POST /graphql answers introspection for
         "type Query { smoke_ping: String }" and the query { smoke_ping } with "pong"
Inputs: none. Outputs: /tmp/events.jsonl, HTTP answers. Constraints: stdlib only,
no secrets, synthetic data only.
"""
import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

EVENTS = "/tmp/events.jsonl"
LOCK = threading.Lock()


def scalar(name):
    return {"kind": "SCALAR", "name": name, "description": None, "fields": None,
            "inputFields": None, "interfaces": None, "enumValues": None, "possibleTypes": None}


INTROSPECTION = {"data": {"__schema": {
    "queryType": {"name": "Query"}, "mutationType": None, "subscriptionType": None,
    "directives": [],
    "types": [
        {"kind": "OBJECT", "name": "Query", "description": None,
         "fields": [{"name": "smoke_ping", "description": None, "args": [],
                     "type": {"kind": "SCALAR", "name": "String", "ofType": None},
                     "isDeprecated": False, "deprecationReason": None}],
         "inputFields": None, "interfaces": [], "enumValues": None, "possibleTypes": None},
        scalar("String"), scalar("Boolean"),
    ]}}}


class Hook(BaseHTTPRequestHandler):
    def do_POST(self):
        body = self.rfile.read(int(self.headers.get("Content-Length") or 0))
        with LOCK, open(EVENTS, "ab") as f:
            f.write(body.replace(b"\n", b" ") + b"\n")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(b"{}")

    def log_message(self, *args):
        pass


class Remote(BaseHTTPRequestHandler):
    def do_POST(self):
        raw = self.rfile.read(int(self.headers.get("Content-Length") or 0))
        try:
            query = json.loads(raw).get("query", "")
        except ValueError:
            query = ""
        if "__schema" in query:
            out = INTROSPECTION
        elif "smoke_ping" in query:
            out = {"data": {"smoke_ping": "pong"}}
        else:
            out = {"errors": [{"message": "unsupported query"}]}
        data = json.dumps(out).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def log_message(self, *args):
        pass


if __name__ == "__main__":
    open(EVENTS, "ab").close()
    hook = ThreadingHTTPServer(("0.0.0.0", 9100), Hook)
    threading.Thread(target=hook.serve_forever, daemon=True).start()
    ThreadingHTTPServer(("0.0.0.0", 9101), Remote).serve_forever()
