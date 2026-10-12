// Purpose: typed failures for the control API (error envelope and 428 confirmation).
// Inputs: decoded envelopes and HTTP metadata. Outputs: NselfError, ConfirmationRequired.
// Constraints: no runtime dependencies; constructors never read the network.

import type { Confirmation, ErrorDetail } from "../generated/types";

/** Error classes the CLI error envelope can carry, plus a client-side protocol class. */
export type ErrorClass = ErrorDetail["class"];

/** Raised for every error envelope and for responses the client cannot decode. */
export class NselfError extends Error {
  /** CLI error code such as E426, or CLIENT_PROTOCOL for an undecodable response. */
  readonly code: string;
  readonly class: ErrorClass;
  /** Exit code the CLI would have returned (X-Nself-Exit-Code when the body lacks one). */
  readonly exitCode: number;
  readonly httpStatus: number;
  readonly remediation?: string;
  readonly docsUrl?: string;
  /** Command the failing envelope names, when present. */
  readonly command?: string;
  readonly requestId?: string;

  constructor(
    detail: Pick<ErrorDetail, "code" | "message" | "class" | "exit_code"> & Partial<ErrorDetail>,
    meta: { httpStatus: number; command?: string; requestId?: string },
  ) {
    super(detail.message);
    this.name = "NselfError";
    this.code = detail.code;
    this.class = detail.class;
    this.exitCode = detail.exit_code;
    this.httpStatus = meta.httpStatus;
    this.remediation = detail.remediation;
    this.docsUrl = detail.docs_url;
    this.command = meta.command;
    this.requestId = meta.requestId;
  }
}

/**
 * Raised on HTTP 428. The server issued a nonce or plan id; confirm() re-sends the
 * same request carrying it and resolves to the command data.
 */
export class ConfirmationRequired<T = unknown> extends Error {
  readonly confirmation: Confirmation;
  readonly command?: string;
  private readonly resend: (confirm: string) => Promise<T>;

  constructor(confirmation: Confirmation, resend: (confirm: string) => Promise<T>, command?: string) {
    super(confirmation.reason);
    this.name = "ConfirmationRequired";
    this.confirmation = confirmation;
    this.command = command;
    this.resend = resend;
  }

  /** Re-send the original request with the server's confirm value. Single use. */
  confirm(): Promise<T> {
    return this.resend(this.confirmation.confirm);
  }
}
