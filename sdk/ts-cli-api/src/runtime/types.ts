// Purpose: the v1 envelope shape shared by documents and stream lines.
// Inputs: none. Outputs: Envelope. Constraints: mirrors schemas/envelope.v1.schema.json.

import type { ErrorDetail } from "../generated/types";

export interface Envelope {
  schema_version: string;
  command: string;
  data?: { type?: string; [key: string]: unknown } & Record<string, unknown>;
  error?: ErrorDetail;
  meta?: { warnings?: string[]; deprecations?: Array<{ old: string; new: string; removal_at: string }> };
}
