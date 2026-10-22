import { createTransport } from "./transport";
import { NselfError } from "./errors";

const token = "fixture-token";

describe("openapi response contract", () => {
  it.each([200, 401, 502])("reports headers and protocol errors for malformed JSON (%s)", async (status) => {
    const onResponse = jest.fn();
    const t = createTransport({ token, fetch: async () => new Response("{bad", { status, headers: { "x-nself-request-id": "fixture-id" } }) });
    const err = await t.openapi({ onResponse }).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(NselfError);
    expect(err).toMatchObject({ code: "CLIENT_PROTOCOL", httpStatus: status });
    expect(onResponse).toHaveBeenCalledTimes(1);
    expect(onResponse).toHaveBeenCalledWith({ httpStatus: status, requestId: "fixture-id", exitCode: undefined, warnings: [] });
  });
  it("reports successful document response headers", async () => {
    const onResponse = jest.fn();
    const t = createTransport({ token, fetch: async () => new Response('{"openapi":"3.1.0"}') });
    expect(await t.openapi({ onResponse })).toEqual({ openapi: "3.1.0" });
    expect(onResponse).toHaveBeenCalledTimes(1);
  });
});

