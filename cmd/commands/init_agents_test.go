package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/agentsetup"
)

// scaffoldLive runs init's AGENTS.md hook the way runInit does, under the live
// command tree (other tests detach the tree's parent pointers).
func scaffoldLive(t *testing.T, dir string, quiet bool) {
	t.Helper()
	reattachRealTree()
	scaffoldAgentsMD(initCmd, dir, quiet)
}

func TestInitScaffoldsAgentsMD(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	scaffoldLive(t, dir, false)
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("init hook did not write AGENTS.md: %v", err)
	}
	if !agentsetup.Unedited(string(first)) {
		t.Fatalf("AGENTS.md is not an unedited generated file:\n%s", first)
	}
	for _, verb := range []string{"init", "start", "stop", "status", "mcp", "exec"} {
		if !strings.Contains(string(first), "- `nself "+verb+"`:") {
			t.Errorf("AGENTS.md lacks canon verb %s", verb)
		}
	}

	// An unedited generated file regenerates identically.
	scaffoldLive(t, dir, false)
	if again, _ := os.ReadFile(path); !bytes.Equal(again, first) {
		t.Fatal("unedited AGENTS.md changed on the second run")
	}

	// A hand edit survives, and so does a file nself never generated.
	for _, mine := range []string{strings.Replace(string(first), "# AGENTS.md", "# AGENTS.md (mine)", 1), "# ours\n"} {
		if err := os.WriteFile(path, []byte(mine), 0o644); err != nil {
			t.Fatal(err)
		}
		scaffoldLive(t, dir, false)
		if kept, _ := os.ReadFile(path); string(kept) != mine {
			t.Fatalf("init overwrote %q", mine[:min(len(mine), 20)])
		}
	}
}

// The same bytes come from `mcp setup --agents-md`.
func TestInitAgentsMDMatchesMCPSetup(t *testing.T) {
	dir := makeNSelfProject(t, "PROJECT_NAME=surf\nENV=dev\n")
	scaffoldLive(t, dir, true)
	written, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("quiet init must still scaffold AGENTS.md: %v", err)
	}
	printed, _, err := runMCPSetupFlags(t, map[string]string{"agents-md": "true"})
	if err != nil || printed != string(written) {
		t.Fatalf("mcp setup --agents-md differs from what init writes (err %v)", err)
	}
}

// A command that is not under the live RootCmd (a test clone) skips the hook.
func TestInitHookSkipsForeignTree(t *testing.T) {
	dir := t.TempDir()
	scaffoldAgentsMD(newInitCmd().Commands()[0], dir, false)
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err == nil {
		t.Fatal("hook ran against a foreign tree")
	}
}
