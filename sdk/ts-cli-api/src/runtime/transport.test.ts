// Purpose: unit tests for the transport, errors and the generated client against a mock fetch.
// Inputs: canned Responses. Outputs: assertions on requests sent and values returned.
// Constraints: no network, no processes; all values are synthetic.

import { createClient } from "../index";
import { ConfirmationRequired, NselfError } from "./errors";
import { readLines } from "./ndjson";
import type { FetchLike } from "./transport";

interface Sent {
  url: string;
  method: string;
  headers: Record<string, string>;
  body?: string;
}

function mock(responses: Array<() => Response>): { fetch: FetchLike; sent: Sent[] } {
  const sent: Sent[] = [];
  let i = 0;
  const fetch: FetchLike = async (url, init) => {
    sent.push({ url, method: init.method, headers: init.headers, body: init.body });
    const make = responses[Math.min(i++, responses.length - 1)];
    return make() as unknown as ReturnType<FetchLike> extends Promise<infer R> ? R : never;
  };
  return { fetch, sent };
}

const json = (status: number, body: unknown, headers: Record<string, string> = {}) => () =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json", ...headers } });

const encoder = new TextEncoder();
function ndjson(chunks: string[], status = 200): () => Response {
  return () =>
    new Response(
      new ReadableStream<Uint8Array>({
        start(controller) {
          for (const c of chunks) controller.enqueue(encoder.encode(c));
          controller.close();
        },
      }),
      { status, headers: { "content-type": "application/x-ndjson" } },
    );
}

const TOKEN = "fake-token-for-tests";
const errorEnvelope = {
  schema_version: "1",
  command: "config get",
  error: { code: "E426", message: "Machine token missing, invalid or insecure", exit_code: 3, class: "auth", remediation: "fix it" },
};

describe("call", () => {
  it("posts the request with the bearer header and returns typed data", async () => {
    const m = mock([json(200, { schema_version: "1", command: "config get", data: { key: "BASE_DOMAIN", value: "x.test", masked: false, file: ".env" } })]);
    const client = createClient({ token: TOKEN, fetch: m.fetch });
    const data = await client.configGet({ args: ["BASE_DOMAIN"] });
    expect(data.value).toBe("x.test");
    expect(m.sent[0].url).toBe("http://127.0.0.1:3857/v1/commands/config/get");
    expect(m.sent[0].method).toBe("POST");
    expect(m.sent[0].headers.Authorization).toBe(`Bearer ${TOKEN}`);
    expect(m.sent[0].headers.Accept).toBe("application/json");
    expect(JSON.parse(m.sent[0].body ?? "")).toEqual({ args: ["BASE_DOMAIN"] });
    expect(m.sent[0].url).not.toContain(TOKEN);
  });

  it("honours baseUrl without a trailing slash and sends {} for an omitted request", async () => {
    const m = mock([json(200, { schema_version: "1", command: "backup", data: {} })]);
    await createClient({ token: TOKEN, baseUrl: "http://example.test:9/", fetch: m.fetch }).backup();
    expect(m.sent[0].url).toBe("http://example.test:9/v1/commands/backup");
    expect(m.sent[0].body).toBe("{}");
  });

  it("throws NselfError with code, class, exitCode and httpStatus from the error envelope", async () => {
    const m = mock([json(401, errorEnvelope, { "x-nself-request-id": "req-1" })]);
    const err = await createClient({ token: TOKEN, fetch: m.fetch }).configGet({ args: ["A"] }).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(NselfError);
    const e = err as NselfError;
    expect([e.code, e.class, e.exitCode, e.httpStatus]).toEqual(["E426", "auth", 3, 401]);
    expect(e.remediation).toBe("fix it");
    expect(e.command).toBe("config get");
    expect(e.requestId).toBe("req-1");
  });

  it("turns a non-JSON body into a CLIENT_PROTOCOL NselfError", async () => {
    const m = mock([() => new Response("<html>bad gateway</html>", { status: 502 })]);
    const e = (await createClient({ token: TOKEN, fetch: m.fetch }).backup().catch((x: unknown) => x)) as NselfError;
    expect(e).toBeInstanceOf(NselfError);
    expect([e.code, e.httpStatus]).toEqual(["CLIENT_PROTOCOL", 502]);
  });

  it("reports exit code, request id and warnings through onResponse", async () => {
    const m = mock([json(200, { schema_version: "1", command: "backup", data: {}, meta: { warnings: ["w1"] } }, { "x-nself-exit-code": "10", "x-nself-request-id": "r" })]);
    let seen: unknown;
    await createClient({ token: TOKEN, fetch: m.fetch }).backup({}, { onResponse: (i) => (seen = i) });
    expect(seen).toEqual({ httpStatus: 200, exitCode: 10, requestId: "r", warnings: ["w1"] });
  });

  it("requires a token", () => {
    expect(() => createClient({ token: "", fetch: mock([]).fetch })).toThrow(/token/);
  });
});

describe("confirmation", () => {
  const confirm = "0".repeat(64);
  const needs = json(428, {
    schema_version: "1",
    command: "config secrets set",
    data: { confirmation_required: true, kind: "request", confirm, side_effect: "destructive", reason: "confirmation required", plan: null },
  });

  it("throws ConfirmationRequired whose confirm() re-sends the same request with the nonce", async () => {
    const m = mock([needs, json(200, { schema_version: "1", command: "config secrets set", data: { ok: true } })]);
    const client = createClient({ token: TOKEN, fetch: m.fetch });
    const err = await client.configSecretsSet({ args: ["K", "v"] }).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ConfirmationRequired);
    const need = err as ConfirmationRequired;
    expect(need.confirmation.side_effect).toBe("destructive");
    expect(need.message).toBe("confirmation required");
    const data = await need.confirm();
    expect(data).toEqual({ ok: true });
    expect(m.sent).toHaveLength(2);
    expect(JSON.parse(m.sent[1].body ?? "")).toEqual({ ...JSON.parse(m.sent[0].body ?? ""), confirm });
    expect(m.sent[1].url).toBe(m.sent[0].url);
  });

  it("surfaces a failure of the confirmed request as NselfError", async () => {
    const m = mock([needs, json(422, { ...errorEnvelope, error: { code: "E400", message: "too short", exit_code: 1, class: "user" } })]);
    const need = (await createClient({ token: TOKEN, fetch: m.fetch }).configSecretsSet({ args: ["K", "v"] }).catch((e: unknown) => e)) as ConfirmationRequired;
    const e = (await need.confirm().catch((x: unknown) => x)) as NselfError;
    expect([e.code, e.class, e.httpStatus]).toEqual(["E400", "user", 422]);
  });
});

describe("stream", () => {
  const rec = (n: number) => JSON.stringify({ schema_version: "1", command: "logs", data: { type: "log", message: `line ${n} é✓` } });
  const result = JSON.stringify({ schema_version: "1", command: "logs", data: { type: "result", lines: 2 } });

  it("yields records then the final envelope, across chunk and UTF-8 boundaries", async () => {
    const whole = `${rec(1)}\n${rec(2)}\n${result}\n`;
    const bytes = encoder.encode(whole);
    const cut = bytes.indexOf(0xc3) + 1; // split inside the two-byte "é"
    const chunks = [bytes.slice(0, cut), bytes.slice(cut, cut + 7), bytes.slice(cut + 7)];
    const resp = () =>
      new Response(new ReadableStream<Uint8Array>({ start: (c) => { chunks.forEach((x) => c.enqueue(x)); c.close(); } }), {
        status: 200,
        headers: { "content-type": "application/x-ndjson" },
      });
    const m = mock([resp]);
    const items = [];
    for await (const item of createClient({ token: TOKEN, fetch: m.fetch }).stream("/v1/commands/logs", { flags: { follow: true } })) items.push(item);
    expect(items.map((i) => i.final)).toEqual([false, false, true]);
    expect(items[0].envelope.data?.message).toBe("line 1 é✓");
    expect(items[2].envelope.data).toEqual({ type: "result", lines: 2 });
    expect(m.sent[0].headers.Accept).toBe("application/x-ndjson");
    expect(JSON.parse(m.sent[0].body ?? "")).toEqual({ flags: { follow: true } });
  });

  it("throws the error envelope that ends a stream", async () => {
    const m = mock([ndjson([`${rec(1)}\n`, JSON.stringify(errorEnvelope) + "\n"])]);
    const seen: unknown[] = [];
    let err: unknown;
    try {
      for await (const item of createClient({ token: TOKEN, fetch: m.fetch }).stream("/v1/commands/logs")) seen.push(item);
    } catch (e) {
      err = e;
    }
    expect(seen).toHaveLength(1);
    expect(err).toBeInstanceOf(NselfError);
    expect((err as NselfError).code).toBe("E426");
  });

  it("fails when the stream ends without a final envelope", async () => {
    const m = mock([ndjson([`${rec(1)}\n`])]);
    const run = async () => {
      for await (const item of createClient({ token: TOKEN, fetch: m.fetch }).stream("/v1/commands/logs")) void item;
    };
    await expect(run()).rejects.toMatchObject({ code: "CLIENT_PROTOCOL" });
  });

  it("raises a pre-stream refusal as NselfError and passes a document answer through as final", async () => {
    const refused = mock([json(404, { ...errorEnvelope, error: { code: "E421", message: "not exposed", exit_code: 1, class: "user" } })]);
    await expect((async () => { for await (const i of createClient({ token: TOKEN, fetch: refused.fetch }).stream("/v1/commands/x")) void i; })()).rejects.toMatchObject({ code: "E421", httpStatus: 404 });
    const doc = mock([json(200, { schema_version: "1", command: "logs", data: { records: [] } })]);
    const items = [];
    for await (const i of createClient({ token: TOKEN, fetch: doc.fetch }).stream("/v1/commands/logs")) items.push(i);
    expect(items).toHaveLength(1);
    expect(items[0].final).toBe(true);
  });
});

describe("readLines", () => {
  it("cancels the reader when the consumer stops early", async () => {
    let cancelled = false;
    const reader = {
      read: async () => ({ done: false, value: encoder.encode("a\nb\n") }),
      cancel: async () => { cancelled = true; },
    };
    for await (const line of readLines(reader)) {
      expect(line).toBe("a");
      break;
    }
    expect(cancelled).toBe(true);
  });
});

describe("openapi", () => {
  it("GETs /openapi.json with the bearer header", async () => {
    const m = mock([json(200, { openapi: "3.1.0" })]);
    expect(await createClient({ token: TOKEN, fetch: m.fetch }).openapi()).toEqual({ openapi: "3.1.0" });
    expect(m.sent[0]).toMatchObject({ method: "GET", url: "http://127.0.0.1:3857/openapi.json" });
    expect(m.sent[0].headers.Authorization).toBe(`Bearer ${TOKEN}`);
  });
});
