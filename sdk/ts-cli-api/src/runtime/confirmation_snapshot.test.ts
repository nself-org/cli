import { ConfirmationRequired } from "./errors";
import { createTransport, type FetchLike } from "./transport";

it("replays the first serialized request after nested caller input mutates", async () => {
  const sent: string[] = [];
  const request = { args: [{ key: "original" }], flags: { nested: { value: "original" } } };
  const fetch: FetchLike = async (_url, init) => {
    sent.push(init.body!);
    if (sent.length === 1) {
      request.args[0].key = "during fetch";
      return new Response(JSON.stringify({ data: { confirm: "nonce" } }), { status: 428 });
    }
    return new Response(JSON.stringify({ data: { ok: true } }), { status: 200 });
  };
  const transport = createTransport({ token: "test", fetch });
  const error = await transport.call("/test", request).catch((e: unknown) => e);
  expect(error).toBeInstanceOf(ConfirmationRequired);
  request.args.push({ key: "added" });
  request.flags.nested.value = "after response";
  await (error as ConfirmationRequired).confirm();
  expect(JSON.parse(sent[1])).toEqual({ ...JSON.parse(sent[0]), confirm: "nonce" });
});
