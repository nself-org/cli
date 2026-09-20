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
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
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
		b.WriteString(merged[k])
		b.WriteString("\n")
	}

	if err := os.WriteFile(snapshotPath, []byte(b.String()), 0o600); err != nil {
		return "", func() {}, fmt.Errorf("writing resolved env snapshot: %w", err)
	}

	cleanup = func() { _ = os.Remove(snapshotPath) }
	return snapshotPath, cleanup, nil
}

// envKV is a single KEY=VALUE pair read from an .env file, preserving
// insertion order for deterministic snapshot output.
type envKV struct {
	key   string
	value string
}

// readEnvFileOverrides parses path as a simple .env file (KEY=VALUE per
// line, '#' comments, blank lines ignored). Missing files return an empty
// slice, matching the "each file is optional" semantics of config.Load.
// This intentionally does not use godotenv.Overload (which mutates
// os.Environ) — it only needs the raw key/value pairs for the snapshot.
func readEnvFileOverrides(path string) ([]envKV, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			// KEEP. Every .env file in the chain is optional by contract (see
			// config.Load), so "absent" is a supported state and carries no
			// overrides. ENOENT only — an .env.secrets that exists and cannot
			// be opened must error, not deploy a snapshot silently missing the
			// secrets it was supposed to carry.
			return nil, nil
		}
		return nil, err
	}
	defer f.Close() //nolint:errcheck

	var out []envKV
	scanner := bufio.NewScanner(f)
	// .env files (esp. .env.secrets carrying long keys/certs) can exceed the
	// default 64KB scanner buffer; raise it to 1MB per line.
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.Index(line, "=")
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		if key == "" {
			continue
		}
		value := strings.TrimSpace(line[eq+1:])
		// Strip a single layer of matching quotes, mirroring godotenv's
		// handling of quoted values so the snapshot round-trips identically.
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		out = append(out, envKV{key: key, value: value})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
