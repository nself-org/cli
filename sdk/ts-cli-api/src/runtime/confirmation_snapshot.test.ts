import { createTransport } from "./transport";
import { ConfirmationRequired } from "./errors";
const token = "fixture-token";

describe("confirmation request snapshot", () => {
  it.each(["args", "flags", "argv"] as const)("preserves serialized %s despite caller mutation", async (field) => {
    const sent: string[] = [];
    const confirm = "0".repeat(64);
    const request = { args: ["original"], flags: { nested: ["original"] }, argv: ["original"] };
    const t = createTransport({ token, fetch: async (_url, init) => {
      sent.push(init.body ?? "");
      return sent.length === 1
        ? new Response(JSON.stringify({ data: { confirm, kind: "request", confirmation_required: true } }), { status: 428 })
        : new Response('{"data":{}}');
    } });
    const pending = t.call("/test", request).catch((e: unknown) => e);
    if (field === "flags") request.flags.nested[0] = "changed";
    else request[field][0] = "changed";
    const need = await pending as ConfirmationRequired;
    await need.confirm();
    expect(JSON.parse(sent[1])).toEqual({ ...JSON.parse(sent[0]), confirm });
  });
});
