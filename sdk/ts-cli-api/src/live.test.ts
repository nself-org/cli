// Purpose: contract test of the client against a real `nself admin serve` on a throwaway project.
// Inputs: NSELF_BIN (path of a built nself). Outputs: assertions on real responses.
// Constraints: isolated HOME and project dir; a docker shim feeds `logs`; all values are synthetic.

import { execFileSync, spawn } from "node:child_process";
import type { ChildProcess } from "node:child_process";
import { chmodSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { ConfirmationRequired, NselfError, createClient } from "./index";
import type { Client } from "./index";

const bin = process.env.NSELF_BIN;
const suite = bin ? describe : describe.skip;

jest.setTimeout(30000); // real process spawns on a loaded CI host

const SHIM = `#!/bin/sh
case "$*" in
  "compose ps -q") echo fixture-container ;;
  compose*logs*) printf 'api  | 2026-01-01T00:00:00Z first line\\napi  | second line\\n' ;;
esac
`;

function freePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const s = createServer();
    s.once("error", reject);
    s.listen(0, "127.0.0.1", () => {
      const { port } = s.address() as { port: number };
      s.close(() => resolve(port));
    });
  });
}

suite("live nself admin serve", () => {
  let root = "";
  let server: ChildProcess | undefined;
  let client: Client;
  let baseUrl = "";
  let token = "";

  beforeAll(async () => {
    root = mkdtempSync(join(tmpdir(), "cli-api-live-"));
    const home = join(root, "home");
    const proj = join(root, "proj");
    const shimDir = join(root, "bin");
    for (const d of [home, proj, shimDir]) mkdirSync(d, { recursive: true });
    writeFileSync(join(shimDir, "docker"), SHIM);
    chmodSync(join(shimDir, "docker"), 0o755);
    const env = { ...process.env, HOME: home, NSELF_V15: "1", NSELF_PLUGIN_DIR: join(home, "plugins"), PATH: `${shimDir}:${process.env.PATH ?? ""}` };
    execFileSync(bin as string, ["init", "--non-interactive", "--domain", "fixture.test"], { cwd: proj, env, stdio: "pipe" });
    try {
      execFileSync("age", ["--version"], { stdio: "pipe" });
    } catch {
      throw new Error("the live suite needs `age` on PATH (brew install age / apt-get install age)");
    }
    execFileSync(bin as string, ["config", "secrets", "init"], { cwd: proj, env, stdio: "pipe" });
    const port = await freePort();
    baseUrl = `http://127.0.0.1:${port}`;
    const child = spawn(bin as string, ["admin", "serve", "--port", String(port)], { cwd: proj, env, stdio: ["ignore", "pipe", "pipe"] });
    server = child;
    await new Promise<void>((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error("admin serve did not start")), 30000);
      let seen = "";
      const onData = (b: Buffer) => {
        seen += b.toString();
        if (seen.includes("listening on")) {
          clearTimeout(timer);
          resolve();
        }
      };
      child.stdout?.on("data", onData);
      child.stderr?.on("data", onData);
      child.once("exit", (code) => reject(new Error(`admin serve exited early (${code}): ${seen}`)));
    });
    token = readFileSync(join(home, ".nself", "mcp-token"), "utf8").trim();
    client = createClient({ baseUrl, token });
  }, 60000);

  afterAll(async () => {
    if (server && server.exitCode === null) {
      const exited = new Promise((r) => server?.once("exit", r));
      server.kill("SIGTERM");
      await exited;
    }
    if (root) rmSync(root, { recursive: true, force: true });
  });

  it("configGet returns typed data", async () => {
    const data = await client.configGet({ args: ["BASE_DOMAIN"] });
    expect(data).toMatchObject({ key: "BASE_DOMAIN", value: "fixture.test", masked: false });
  });

  it("an unknown key throws a user-class NselfError", async () => {
    const e = (await client.configGet({ args: ["NO_SUCH_KEY_X"] }).catch((x: unknown) => x)) as NselfError;
    expect(e).toBeInstanceOf(NselfError);
    expect([e.code, e.class, e.exitCode, e.httpStatus]).toEqual(["E400", "user", 1, 422]);
  });

  it("a rejected token throws an auth-class NselfError with code, class, exitCode and httpStatus", async () => {
    const bad = createClient({ baseUrl, token: "wrong-token-for-live-test" });
    const e = (await bad.configGet({ args: ["BASE_DOMAIN"] }).catch((x: unknown) => x)) as NselfError;
    expect(e).toBeInstanceOf(NselfError);
    expect([e.code, e.class, e.exitCode, e.httpStatus]).toEqual(["E426", "auth", 3, 401]);
  });

  it("a destructive operation yields ConfirmationRequired and confirm() re-sends it", async () => {
    const request = { args: ["FIXTURE_SECRET_A", "fake-fixture-value-fake-fixture-value"] as [string, string] };
    const need = (await client.configSecretsSet(request).catch((x: unknown) => x)) as ConfirmationRequired;
    expect(need).toBeInstanceOf(ConfirmationRequired);
    expect(need.confirmation).toMatchObject({ confirmation_required: true, side_effect: "destructive" });
    const before = await client.configSecretsList();
    expect((before.secrets ?? []).map((x) => x.name)).not.toContain("FIXTURE_SECRET_A");
    expect(await need.confirm()).toBeDefined();
    const after = await client.configSecretsList();
    expect((after.secrets ?? []).map((x) => x.name)).toContain("FIXTURE_SECRET_A");
  });

  it("a stream operation yields records then the final envelope", async () => {
    const items = [];
    for await (const item of client.stream("/v1/commands/logs", { flags: { follow: true } })) items.push(item);
    expect(items.length).toBeGreaterThanOrEqual(3);
    expect(items.slice(0, -1).every((i) => !i.final)).toBe(true);
    expect(items[0].envelope.data?.message).toBe("first line");
    const last = items[items.length - 1];
    expect(last.final).toBe(true);
    expect(last.envelope.data).toMatchObject({ type: "result", lines: 2 });
  });

  it("openapi() returns the served contract", async () => {
    const doc = (await client.openapi()) as { openapi: string };
    expect(doc.openapi).toBe("3.1.0");
  });
});
