// Purpose: prove the contract-test environment drops every inherited nself, XDG and secrets variable.
// Inputs: synthetic environments. Outputs: assertions on the filtered copy.
// Constraints: no process spawns; all values are synthetic.

import { isolatedEnv } from "./isolated-env";

describe("isolatedEnv", () => {
  it("drops SECRETS_AGE_KEY_PATH so `secrets init` cannot reuse a developer's key", () => {
    const env = isolatedEnv({ SECRETS_AGE_KEY_PATH: "/home/dev/age-key.txt", PATH: "/usr/bin" });
    expect(env).not.toHaveProperty("SECRETS_AGE_KEY_PATH");
    expect(env.PATH).toBe("/usr/bin");
  });

  it("drops NSELF_ and XDG_ variables and keeps unrelated ones", () => {
    const env = isolatedEnv({ NSELF_TOKEN: "t", XDG_CONFIG_HOME: "/x", TZ: "UTC", HOME: "/h" });
    expect(Object.keys(env).sort()).toEqual(["HOME", "TZ"]);
  });

  it("skips undefined values", () => {
    expect(isolatedEnv({ A: undefined, B: "1" })).toEqual({ B: "1" });
  });
});
