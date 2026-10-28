// Purpose: build the environment of the throwaway nself the contract tests spawn.
// Inputs: the parent environment. Outputs: a copy with every nself, XDG and secrets variable removed.
// Constraints: test support only; a developer's own token, config or age key path must never reach the fixture.

const DROPPED_PREFIXES = ["NSELF_", "XDG_", "SECRETS_"];

export function isolatedEnv(parent: NodeJS.ProcessEnv): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(parent)) {
    if (v === undefined || DROPPED_PREFIXES.some((p) => k.startsWith(p))) continue;
    out[k] = v;
  }
  return out;
}
