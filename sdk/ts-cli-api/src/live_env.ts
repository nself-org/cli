// Environment isolation for the throwaway live-test server.
export function liveInheritedEnv(env: NodeJS.ProcessEnv): NodeJS.ProcessEnv {
  return Object.fromEntries(Object.entries(env).filter(([k]) => !k.startsWith("NSELF_") && !k.startsWith("XDG_") && k !== "SECRETS_AGE_KEY_PATH"));
}
