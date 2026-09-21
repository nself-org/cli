package build

// orchestrator_build_config_test.go — regression coverage for the --check
// read-only contract enforced in orchestrator_build_config.go's Step 1.5.
//
// Purpose: prove that buildState.loadValidateConfig(), the function backing
// `nself build --check`, never mutates the project tree it inspects.
// Inputs:  a minimal temp project directory.
// Outputs: pass/fail — a byte-for-byte tree snapshot taken before and after
//          the call must be identical, and .env.secrets must not appear.
// Constraints: exercises Steps 1-4 only (config load, secret persistence,
//              permission fix, validate, nginx preflight, --check exit) —
//              not the full Build() pipeline, matching this package's
//              existing convention (see orchestrator_test.go's header) of
//              not booting SSL/nginx/compose generation in a bare temp dir.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadValidateConfig_CheckIsReadOnly is the regression test for the
// 2026-09-20 production incident: a `nself build --check` run against
// /opt/nself-web wrote freshly-generated PLUGIN_INTERNAL_SECRET and
// NOTIFY_INTERNAL_SECRET values into backend/.env.secrets, because Step 1.5
// (persistGeneratedSecrets) and the .env permission-fix loop ran
// unconditionally, before the --check early-return at Step 3. "--check"
// means "validate only" — it must be a pure read.
func TestLoadValidateConfig_CheckIsReadOnly(t *testing.T) {
	// Isolate from the caller's environment so an ambient
	// HASURA_GRAPHQL_JWT_SECRET / ENV doesn't change what gets loaded.
	t.Setenv("HASURA_GRAPHQL_JWT_SECRET", "")
	t.Setenv("ENV", "dev")

	dir := t.TempDir()
	testWriteFile(t, filepath.Join(dir, ".env"), "PROJECT_NAME=check-readonly\n", 0600)

	before := snapshotTree(t, dir)

	st := &buildState{workdir: dir, opts: BuildOptions{Check: true}}
	result, err := st.loadValidateConfig()
	if err != nil {
		t.Fatalf("loadValidateConfig with --check: %v", err)
	}
	if result == nil {
		t.Fatal("expected a non-nil BuildResult from the --check early-exit path")
	}

	after := snapshotTree(t, dir)
	if diff := diffTrees(before, after); diff != "" {
		t.Fatalf("--check must not write to the project tree it validates, but it changed:\n%s", diff)
	}
	if _, statErr := os.Stat(filepath.Join(dir, ".env.secrets")); statErr == nil {
		t.Fatalf(".env.secrets must not be created by a --check run")
	}
}

// TestLoadValidateConfig_NonCheckStillPersistsSecrets is the control case:
// without --check, Step 1.5 must still run so a normal `nself build`
// continues to persist auto-generated secrets exactly as before this fix.
func TestLoadValidateConfig_NonCheckStillPersistsSecrets(t *testing.T) {
	t.Setenv("HASURA_GRAPHQL_JWT_SECRET", "")
	t.Setenv("ENV", "dev")

	dir := t.TempDir()
	testWriteFile(t, filepath.Join(dir, ".env"), "PROJECT_NAME=check-control\n", 0600)

	st := &buildState{workdir: dir, opts: BuildOptions{Check: false}}
	if _, err := st.loadValidateConfig(); err != nil {
		t.Fatalf("loadValidateConfig without --check: %v", err)
	}

	if _, statErr := os.Stat(filepath.Join(dir, ".env.secrets")); statErr != nil {
		t.Fatalf("expected .env.secrets to be written on a non-check build: %v", statErr)
	}
}

// snapshotTree records every regular file under dir, keyed by its path
// relative to dir, with its full content — enough to detect any add,
// remove, or modify between two calls.
func snapshotTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	snap := make(map[string]string)
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		snap[rel] = string(data)
		return nil
	})
	return snap
}

// diffTrees returns a human-readable diff between two snapshotTree results,
// or "" when they are identical. "+" = added, "-" = removed, "~" = changed.
func diffTrees(before, after map[string]string) string {
	var out strings.Builder
	for path, content := range after {
		if prior, ok := before[path]; !ok {
			out.WriteString("+ " + path + "\n")
		} else if prior != content {
			out.WriteString("~ " + path + "\n")
		}
	}
	for path := range before {
		if _, ok := after[path]; !ok {
			out.WriteString("- " + path + "\n")
		}
	}
	return out.String()
}
