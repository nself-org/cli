package commands

// Purpose: Build a single merged .env snapshot for a deploy target that is
//          guaranteed to match the same file cascade config.Load used to
//          generate the docker-compose.yml being pushed in the same
//          operation (gap #13 in
//          ~/Sites/nself/.claude/planning/nself-cli-gaps-from-ntask-dogfood.md).
// Inputs:  workdir (project root) and target ("local"|"staging"|"prod").
// Outputs: Path to a temp merged-env file in workdir plus a cleanup func, or
//          an error.
// Constraints: Merge order MUST mirror deployEnvCascadeFiles/config.Load
//              exactly (later files override earlier keys) so the pushed env
//              is provably the same resolution the build step used — this
//              is the whole point of the fix, not just "push more files".
// SPORT: cli/cmd/commands — see gap #13.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// writeResolvedDeployEnv merges every file in target's env cascade (in the
// same precedence order config.Load uses) into one snapshot file inside
// workdir, and returns its path plus a cleanup function that removes it.
//
// The snapshot is intentionally named with a "nself-deploy-env-" prefix and a
// timestamp so a crashed/interrupted deploy never leaves an ambiguous file
// behind, and so it never collides with a real .env* file a project might
// have on disk.
func writeResolvedDeployEnv(workdir, target string) (path string, cleanup func(), err error) {
	merged := map[string]string{}
	var order []string

	for _, f := range deployEnvCascadeFiles(workdir, target) {
		pairs, readErr := readEnvFileOverrides(f)
		if readErr != nil {
			return "", func() {}, fmt.Errorf("reading %s: %w", f, readErr)
		}
		for _, kv := range pairs {
			if _, seen := merged[kv.key]; !seen {
				order = append(order, kv.key)
			}
			merged[kv.key] = kv.value // later files override earlier ones
		}
	}

	snapshotName := fmt.Sprintf(".nself-deploy-env-%s-%d.tmp", target, time.Now().UnixNano())
	snapshotPath := filepath.Join(workdir, snapshotName)

	var b strings.Builder
	b.WriteString("# GENERATED — resolved deploy env snapshot, do not commit or hand-edit.\n")
	fmt.Fprintf(&b, "# Merged from the same cascade config.Load used to build docker-compose.yml for target=%s.\n", target)
	for _, k := range order {
		b.WriteString(k)
		b.WriteString("=")
		enc, ok := encodeEnvValue(merged[k])
		if !ok {
			return "", func() {}, fmt.Errorf("the value of %s cannot be written to the deploy env file without changing it (godotenv cannot represent it); change the value", k)
		}
		b.WriteString(enc)
		b.WriteString("\n")
	}

	if err := os.WriteFile(snapshotPath, []byte(b.String()), 0o600); err != nil {
		return "", func() {}, fmt.Errorf("writing resolved env snapshot: %w", err)
	}

	cleanup = func() { _ = os.Remove(snapshotPath) }
	return snapshotPath, cleanup, nil
}

// encodeEnvValue renders v so godotenv (what config.Load and the remote nself
// both use) reads back exactly v. It tries bare, single-quoted, then
// double-quoted (with \, ", $ and line breaks escaped) and keeps the first
// form that round-trips through godotenv itself. ok is false when none does
// (godotenv cannot represent a few values, e.g. one ending in a backslash);
// the caller must then fail rather than ship a different value.
func encodeEnvValue(v string) (enc string, ok bool) {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "$", `\$`)
	for _, c := range []string{v, "'" + v + "'", `"` + r.Replace(v) + `"`} {
		if m, err := godotenv.Unmarshal("K=" + c + "\nZ=1\n"); err == nil && m["K"] == v && m["Z"] == "1" {
			return c, true
		}
	}
	return "", false
}

// envKV is a single KEY=VALUE pair read from an .env file.
type envKV struct {
	key   string
	value string
}

// readEnvFileOverrides parses path with godotenv, the same reader config.Load
// uses (export prefixes, inline comments, quoting and escapes all behave as in
// the build). Pairs come back in sorted key order for a deterministic snapshot.
// A missing file returns an empty slice, matching config.Load's "each file is
// optional" semantics.
func readEnvFileOverrides(path string) ([]envKV, error) {
	m, err := godotenv.Read(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]envKV, 0, len(keys))
	for _, k := range keys {
		out = append(out, envKV{key: k, value: m[k]})
	}
	return out, nil
}
