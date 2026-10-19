// Purpose: build the isolated child environment for the live suite (debt D-1158).
// Inputs: the parent environment, the throwaway HOME and docker-shim dir. Outputs: the env for spawned nself processes.
// Constraints: pure function; nothing from a developer's own token, config or age key may reach the throwaway server.

import { join } from "node:path";

export type Env = Record<string, string | undefined>;

// Variables that name a developer's own state outside the throwaway HOME.
const DROPPED_EXACT = new Set(["SECRETS_AGE_KEY_PATH"]);
const DROPPED_PREFIXES = ["NSELF_", "XDG_"];

export function isolatedEnv(parent: Env, dirs: { home: string; shimDir: string }): Env {
  const inherited = Object.fromEntries(
    Object.entries(parent).filter(([k]) => !DROPPED_EXACT.has(k) && !DROPPED_PREFIXES.some((p) => k.startsWith(p))),
  );
  return { ...inherited, HOME: dirs.home, NSELF_V15: "1", NSELF_PLUGIN_DIR: join(dirs.home, "plugins"), PATH: `${dirs.shimDir}:${parent.PATH ?? ""}` };
}
