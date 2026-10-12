// Purpose: HTTP transport for the control API: JSON calls, 428 handling, NDJSON streams.
// Inputs: base URL, bearer token, optional fetch. Outputs: call(), stream(), openapi().
// Constraints: zero dependencies; fetch is injectable; the token is only sent as a header.

import type { Confirmation } from "../generated/types";
import { ConfirmationRequired, NselfError } from "./errors";
import { readLines } from "./ndjson";
import type { Envelope } from "./types";

/** Minimal fetch surface used here (structurally compatible with the global fetch). */
export type FetchLike = (
  url: string,
  init: { method: string; headers: Record<string, string>; body?: string; signal?: AbortSignal },
) => Promise<ResponseLike>;

export interface ResponseLike {
  status: number;
  headers: { get(name: string): string | null };
  text(): Promise<string>;
  body: { getReader(): { read(): Promise<{ done: boolean; value?: Uint8Array }>; cancel(): Promise<void> } } | null;
}

export interface ClientOptions {
  /** Default http://127.0.0.1:3857 */
  baseUrl?: string;
  /** Machine token (the `nself admin serve` bearer token). */
  token: string;
  /** Defaults to the global fetch. */
  fetch?: FetchLike;
}

/** Facts about a response, for callers that need more than `data`. */
export interface ResponseInfo {
  httpStatus: number;
  /** X-Nself-Exit-Code: state exits 10-12 arrive with a data envelope. */
  exitCode?: number;
  requestId?: string;
  warnings: string[];
}

export interface CallOptions {
  signal?: AbortSignal;
  /**
   * Called once per response with the headers. For streams it runs before the body is read,
   * so exitCode stays undefined: the stream's exit code is an HTTP trailer fetch does not expose.
   */
  onResponse?: (info: ResponseInfo) => void;
}

/** The request body the server accepts. */
export interface CommandRequest {
  args?: readonly unknown[];
  flags?: object;
  confirm?: string;
}

/** One NDJSON line: every line is a v1 envelope; the last is a result or an error. */
export interface StreamItem {
  final: boolean;
  envelope: Envelope;
}

export interface Transport {
  call<T>(path: string, request: CommandRequest | undefined, options?: CallOptions): Promise<T>;
  stream(path: string, request?: CommandRequest, options?: CallOptions): AsyncGenerator<StreamItem, void, undefined>;
  openapi(options?: CallOptions): Promise<unknown>;
}

const DEFAULT_BASE_URL = "http://127.0.0.1:3857";
const NDJSON = "application/x-ndjson";

function clientError(message: string, httpStatus: number, exitCode = 1): NselfError {
  return new NselfError({ code: "CLIENT_PROTOCOL", message, class: "other", exit_code: exitCode }, { httpStatus });
}

function infoOf(res: ResponseLike, env?: Envelope): ResponseInfo {
  const raw = res.headers.get("x-nself-exit-code");
  const exit = raw === null || raw === "" ? undefined : Number(raw);
  return {
    httpStatus: res.status,
    exitCode: exit !== undefined && Number.isFinite(exit) ? exit : undefined,
    requestId: res.headers.get("x-nself-request-id") ?? undefined,
    warnings: env?.meta?.warnings ?? [],
  };
}

function fail(env: Envelope, res: ResponseLike): NselfError {
  const info = infoOf(res);
  if (!env.error) return clientError(`unexpected response (HTTP ${res.status})`, res.status, info.exitCode ?? 1);
  return new NselfError(env.error, { httpStatus: res.status, command: env.command, requestId: info.requestId });
}

function decode(text: string, res: ResponseLike): Envelope {
  try {
    const v: unknown = JSON.parse(text);
    if (typeof v === "object" && v !== null) return v as Envelope;
  } catch {
    // fall through to the protocol error
  }
  throw clientError(`response is not a JSON envelope (HTTP ${res.status})`, res.status);
}

export function createTransport(options: ClientOptions): Transport {
  const base = (options.baseUrl ?? DEFAULT_BASE_URL).replace(/\/+$/, "");
  const found = options.fetch ?? (globalThis as { fetch?: FetchLike }).fetch;
  if (!found) throw new Error("@nself/cli-api: no fetch available (Node >= 20 has one; or pass options.fetch)");
  const doFetch: FetchLike = found;
  if (!options.token) throw new Error("@nself/cli-api: options.token is required");

  const headers = (accept: string, json: boolean): Record<string, string> => ({
    Authorization: `Bearer ${options.token}`,
    Accept: accept,
    ...(json ? { "Content-Type": "application/json" } : {}),
  });

  const post = (path: string, request: CommandRequest | undefined, accept: string, signal?: AbortSignal) =>
    doFetch(base + path, { method: "POST", headers: headers(accept, true), body: JSON.stringify(request ?? {}), signal });

  async function call<T>(path: string, request: CommandRequest | undefined, opts: CallOptions = {}): Promise<T> {
    const res = await post(path, request, "application/json", opts.signal);
    const env = decode(await res.text(), res);
    opts.onResponse?.(infoOf(res, env));
    if (res.status === 428 && env.data) {
      const resend = (confirm: string) => call<T>(path, { ...request, confirm }, opts);
      throw new ConfirmationRequired<T>(env.data as unknown as Confirmation, resend, env.command);
    }
    if (res.status >= 400 || env.error) throw fail(env, res);
    if (env.data === undefined) throw clientError(`response envelope has no data (HTTP ${res.status})`, res.status);
    return env.data as T;
  }

  async function* stream(path: string, request?: CommandRequest, opts: CallOptions = {}): AsyncGenerator<StreamItem, void, undefined> {
    const res = await post(path, request, NDJSON, opts.signal);
    const type = (res.headers.get("content-type") ?? "").split(";")[0].trim().toLowerCase();
    opts.onResponse?.(infoOf(res));
    if (type !== NDJSON) {
      // A refusal before the first byte (including 406 E430 for a document-only command); a JSON 2xx is passed through as the final item.
      const env = decode(await res.text(), res);
      if (res.status >= 400 || env.error) throw fail(env, res);
      yield { final: true, envelope: env };
      return;
    }
    if (!res.body) throw clientError("stream response has no body", res.status);
    let sawFinal = false;
    for await (const line of readLines(res.body.getReader())) {
      const env = decode(line, res);
      const final = env.error !== undefined || env.data?.type === "result";
      if (env.error) throw fail(env, res);
      sawFinal = sawFinal || final;
      yield { final, envelope: env };
    }
    if (!sawFinal) throw clientError("stream ended without a final envelope", res.status);
  }

  async function openapi(opts: CallOptions = {}): Promise<unknown> {
    const res = await doFetch(base + "/openapi.json", { method: "GET", headers: headers("application/json", false), signal: opts.signal });
    const text = await res.text();
    if (res.status >= 400) throw fail(decode(text, res), res);
    return JSON.parse(text);
  }

  return { call, stream, openapi };
}
