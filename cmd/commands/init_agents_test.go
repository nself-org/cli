package commands

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/agentsetup"
)

func runInitQuietOff(t *testing.T) string {
	t.Helper()
	root := newInitCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"init", "--non-interactive"})
	if err := root.Execute(); err != nil {
		t.Fatalf("init: %v", err)
	}
	return buf.String()
}

func TestInitScaffoldsAgentsMD(t *testing.T) {
	t.Chdir(t.TempDir())
	restoreProcessEnv(t)
	runInitQuietOff(t)
	first, err := os.ReadFile("AGENTS.md")
	if err != nil {
		t.Fatalf("init did not write AGENTS.md: %v", err)
	}
	if !agentsetup.Unedited(string(first)) || !strings.Contains(string(first), "- `nself start`:") {
		t.Fatalf("AGENTS.md is not the generated file with the canon verbs:\n%s", first)
	}

	// An unedited generated file regenerates identically on a second init.
	if err := os.Remove(".env"); err != nil {
		t.Fatal(err)
	}
	runInitQuietOff(t)
	if again, _ := os.ReadFile("AGENTS.md"); !bytes.Equal(again, first) {
		t.Fatal("unedited AGENTS.md changed on a second init")
	}

	// A hand edit survives.
	edited := strings.Replace(string(first), "# AGENTS.md", "# AGENTS.md (mine)", 1)
	if err := os.WriteFile("AGENTS.md", []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(".env"); err != nil {
		t.Fatal(err)
	}
	runInitQuietOff(t)
	if kept, _ := os.ReadFile("AGENTS.md"); string(kept) != edited {
		t.Fatal("init overwrote an edited AGENTS.md")
	}

	// A file nself did not generate survives too.
	if err := os.WriteFile("AGENTS.md", []byte("# ours\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(".env"); err != nil {
		t.Fatal(err)
	}
	runInitQuietOff(t)
	if kept, _ := os.ReadFile("AGENTS.md"); string(kept) != "# ours\n" {
		t.Fatal("init overwrote a foreign AGENTS.md")
	}
}

func TestInitQuietWritesAgentsMDSilently(t *testing.T) {
	t.Chdir(t.TempDir())
	restoreProcessEnv(t)
	root := newInitCmd()
	root.SetArgs([]string{"init", "--non-interactive", "--quiet"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("AGENTS.md"); err != nil {
		t.Fatalf("quiet init must still scaffold AGENTS.md: %v", err)
	}
}
