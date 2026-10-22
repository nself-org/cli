import { liveInheritedEnv } from "./live_env";

it.each(["NSELF_TOKEN", "XDG_CONFIG_HOME", "SECRETS_AGE_KEY_PATH"])("does not inherit %s into the live server", (key) => {
  const env = { [key]: "/fixture/private", PATH: "/fixture/bin" };
  expect(liveInheritedEnv(env)).toEqual({ PATH: "/fixture/bin" });
  expect(env[key]).toBe("/fixture/private");
});
