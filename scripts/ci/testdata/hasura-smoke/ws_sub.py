"""One live subscription over a WebSocket, for hasura-upgrade-smoke.sh (P7-LIVE-23).

Usage:   python3 ws_sub.py HOST PORT ADMIN_SECRET_FILE NAME
Purpose: subscribe to the newest smoke_items row (graphql-transport-ws), wait for
         the first result, insert a row called NAME over HTTP, and wait for the
         subscription to push that row.
Output:  one line "SUBSCRIPTION-EVENT id=<n> name=<NAME>" and exit 0, or a reason
         on stderr and exit 1. The admin secret is read from the file, never argv.
Constraints: stdlib only (minimal RFC 6455 client, text frames, no extensions).
"""
import base64
import json
import os
import socket
import struct
import sys
import time
import urllib.request

host, port, secret_file, name = sys.argv[1], int(sys.argv[2]), sys.argv[3], sys.argv[4]
secret = open(secret_file).read().strip()
DEADLINE = time.time() + 45


def die(msg):
    sys.stderr.write(msg + "\n")
    sys.exit(1)


sock = socket.create_connection((host, port), timeout=10)
key = base64.b64encode(os.urandom(16)).decode()
sock.sendall(("GET /v1/graphql HTTP/1.1\r\nHost: %s:%d\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n"
              "Sec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n"
              "Sec-WebSocket-Protocol: graphql-transport-ws\r\n\r\n" % (host, port, key)).encode())
buf = b""
while b"\r\n\r\n" not in buf:
    chunk = sock.recv(4096)
    if not chunk:
        die("handshake: connection closed")
    buf += chunk
head, buf = buf.split(b"\r\n\r\n", 1)
if b" 101 " not in head.split(b"\r\n")[0]:
    die("handshake refused: " + head.split(b"\r\n")[0].decode(errors="replace"))


def send(obj):
    data = json.dumps(obj).encode()
    mask = os.urandom(4)
    n = len(data)
    hdr = bytes([0x81])
    if n < 126:
        hdr += bytes([0x80 | n])
    elif n < 65536:
        hdr += bytes([0x80 | 126]) + struct.pack(">H", n)
    else:
        hdr += bytes([0x80 | 127]) + struct.pack(">Q", n)
    sock.sendall(hdr + mask + bytes(b ^ mask[i % 4] for i, b in enumerate(data)))


def need(n):
    global buf
    while len(buf) < n:
        if time.time() > DEADLINE:
            die("timed out waiting for a frame")
        try:
            chunk = sock.recv(65536)
        except socket.timeout:
            continue
        if not chunk:
            die("connection closed")
        buf += chunk
    out, buf = buf[:n], buf[n:]
    return out


def recv():
    """The next text message as a dict (control frames are answered or skipped)."""
    while True:
        b0, b1 = need(2)
        op, n = b0 & 0x0F, b1 & 0x7F
        if n == 126:
            n = struct.unpack(">H", need(2))[0]
        elif n == 127:
            n = struct.unpack(">Q", need(8))[0]
        payload = need(n)
        if op == 0x8:
            die("server closed the socket: " + payload[2:].decode(errors="replace"))
        if op in (0x1, 0x0):
            msg = json.loads(payload)
            if msg.get("type") == "ping":  # protocol-level ping: answer and keep reading
                send({"type": "pong"})
                continue
            if msg.get("type") == "pong":
                continue
            return msg
        # ping/pong/other control frames: ignored (Hasura pings are answered by TCP liveness)


sock.settimeout(2)
send({"type": "connection_init", "payload": {"headers": {"x-hasura-admin-secret": secret}}})
msg = recv()
if msg.get("type") != "connection_ack":
    die("no connection_ack: %s" % msg.get("type"))
send({"id": "1", "type": "subscribe", "payload": {
    "query": "subscription { smoke_items(order_by: {id: desc}, limit: 1) { id name } }"}})
msg = recv()
if msg.get("type") != "next":
    die("first message is not next: %s" % json.dumps(msg)[:200])
req = urllib.request.Request(
    "http://%s:%d/v1/graphql" % (host, port),
    data=json.dumps({"query": "mutation($n: String!) { insert_smoke_items_one(object: {name: $n}) { id } }",
                     "variables": {"n": name}}).encode(),
    headers={"Content-Type": "application/json", "x-hasura-admin-secret": secret})
try:
    out = json.load(urllib.request.urlopen(req, timeout=20))
except Exception as exc:  # noqa: BLE001
    die("insert failed: %s" % type(exc).__name__)
if "errors" in out:
    die("insert failed: %s" % json.dumps(out["errors"])[:200])
while True:
    msg = recv()
    if msg.get("type") == "next":
        rows = msg["payload"]["data"]["smoke_items"]
        if rows and rows[0]["name"] == name:
            print("SUBSCRIPTION-EVENT id=%s name=%s" % (rows[0]["id"], name))
            sys.exit(0)
    elif msg.get("type") == "error":
        die("subscription error: %s" % json.dumps(msg)[:200])
