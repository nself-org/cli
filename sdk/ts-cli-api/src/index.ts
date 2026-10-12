// Purpose: public entry of @nself/cli-api.
// Inputs: createClient options. Outputs: one typed method per control-API operation.
// Constraints: operations come from ./generated; this file stays hand-written.

import { createOperations } from "./generated/client";
import type { Operations } from "./generated/client";
import { createTransport } from "./runtime/transport";
import type { ClientOptions, Transport } from "./runtime/transport";

/** Typed operations plus the transport-level stream() and openapi(). */
export type Client = Operations & Pick<Transport, "stream" | "openapi">;

export function createClient(options: ClientOptions): Client {
  const transport = createTransport(options);
  return { ...createOperations(transport), stream: transport.stream, openapi: transport.openapi };
}

export * from "./runtime";
export type { Operations } from "./generated/client";
export * from "./generated/types";
