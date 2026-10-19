// Purpose: unit test of the live-suite environment isolation (debt D-1158).
// Inputs: a synthetic parent environment. Outputs: assertions on the child environment.
// Constraints: no process spawns, no disk, no network.

import { isolatedEnv } from "./isolatedEnv";

describe("isolatedEnv", () => {
  const parent = {
    PATH: "/usr/bin",
    HOME: "/home/dev",
    NSELF_TOKEN: "synthetic-token",
    XDG_CONFIG_HOME: "/home/dev/.config",
    SECRETS_AGE_KEY_PATH: "/home/dev/real-age-key.txt",
    KEEP_ME: "yes",
  };

  it("drops SECRETS_AGE_KEY_PATH so the server never reads or creates a key outside the throwaway HOME", () => {
    const env = isolatedEnv(parent, { home: "/tmp/root/home", shimDir: "/tmp/root/bin" });
    expect(env).not.toHaveProperty("SECRETS_AGE_KEY_PATH");
  });

  it("still drops NSELF_* and XDG_* and keeps unrelated variables", () => {
    const env = isolatedEnv(parent, { home: "/tmp/root/home", shimDir: "/tmp/root/bin" });
    expect(Object.keys(env).filter((k) => k.startsWith("NSELF_") && k !== "NSELF_V15" && k !== "NSELF_PLUGIN_DIR")).toEqual([]);
    expect(Object.keys(env).filter((k) => k.startsWith("XDG_"))).toEqual([]);
    expect(env.KEEP_ME).toBe("yes");
  });

  it("points HOME, plugin dir and PATH at the throwaway root", () => {
    const env = isolatedEnv(parent, { home: "/tmp/root/home", shimDir: "/tmp/root/bin" });
    expect(env).toMatchObject({ HOME: "/tmp/root/home", NSELF_V15: "1", NSELF_PLUGIN_DIR: "/tmp/root/home/plugins", PATH: "/tmp/root/bin:/usr/bin" });
  });
});
