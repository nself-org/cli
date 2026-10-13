package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/agentsetup"
)

// runMCPSetupFlags runs `mcp setup` with the given flags and returns stdout,
// stderr and the error. Flags are reset before and after, so calls do not leak.
func runMCPSetupFlags(t *testing.T, flags map[string]string) (string, string, error) {
	t.Helper()
	reset := func() {
		for _, n := range []string{"client", "write", "agents-md", "json"} {
			f := mcpSetupCmd.Flags().Lookup(n)
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		}
	}
	reset()
	t.Cleanup(reset)
	for k, v := range flags {
		if err := mcpSetupCmd.Flags().Set(k, v); err != nil {
			t.Fatal(err)
		}
	}
	var errBuf bytes.Buffer
	mcpSetupCmd.SetErr(&errBuf)
	var out bytes.Buffer
	mcpSetupCmd.SetOut(&out)
	stdout, err := captureStdout(t, func() error { return runMCPSetup(mcpSetupCmd, nil) })
	return out.String() + stdout, errBuf.String(), err
}

func TestMCPSetupCommand(t *testing.T) {
	if mcpSetupCmd.Parent() != mcpCmd {
		t.Fatal("setup is not under mcp")
	}
	project := makeNSelfProject(t, "PROJECT_NAME=surf\nENV=dev\n")

	// Print: the compact claude-code config, cwd note on stderr, nothing written.
	out, errOut, err := runMCPSetupFlags(t, nil)
	if err != nil || strings.TrimSpace(out) != `{"mcpServers":{"nself":{"command":"nself","args":["mcp"]}}}` {
		t.Fatalf("print: %q %v", out, err)
	}
	if !strings.Contains(errOut, "project directory") {
		t.Errorf("no cwd note on stderr: %q", errOut)
	}
	if _, statErr := os.Stat(filepath.Join(project, ".mcp.json")); statErr == nil {
		t.Fatal("print must not write")
	}

	// Write twice: created once, second run byte-identical.
	if _, _, err = runMCPSetupFlags(t, map[string]string{"write": "true"}); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(filepath.Join(project, ".mcp.json"))
	out, _, err = runMCPSetupFlags(t, map[string]string{"write": "true"})
	second, _ := os.ReadFile(filepath.Join(project, ".mcp.json"))
	if err != nil || !bytes.Equal(first, second) || !strings.Contains(out, "unchanged") {
		t.Fatalf("second write: %q %v", out, err)
	}

	// Cursor writes under .cursor/.
	if _, _, err = runMCPSetupFlags(t, map[string]string{"client": "cursor", "write": "true"}); err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(filepath.Join(project, ".cursor", "mcp.json")); statErr != nil {
		t.Fatal("cursor config not written")
	}

	// codex and generic print only: --write is E401.
	for _, c := range []string{"codex", "generic"} {
		_, _, err = runMCPSetupFlags(t, map[string]string{"client": c, "write": "true"})
		if err == nil || !strings.Contains(err.Error(), "E401") || !strings.Contains(err.Error(), "print-only") {
			t.Fatalf("%s --write: %v", c, err)
		}
	}
	out, _, err = runMCPSetupFlags(t, map[string]string{"client": "codex"})
	if err != nil || !strings.Contains(out, "[mcp_servers.nself]") {
		t.Fatalf("codex print: %q %v", out, err)
	}
	if _, _, err = runMCPSetupFlags(t, map[string]string{"client": "vim"}); err == nil || !strings.Contains(err.Error(), "E401") {
		t.Fatalf("unknown client: %v", err)
	}

	// --json: one envelope with client, path, content, written.
	out, _, err = runMCPSetupFlags(t, map[string]string{"json": "true"})
	var doc struct {
		Command string
		Data    mcpSetupResult
	}
	if err != nil || json.Unmarshal([]byte(out), &doc) != nil || doc.Command != "mcp setup" || doc.Data.Client != "claude-code" || doc.Data.Path != ".mcp.json" || doc.Data.Written || doc.Data.Content == "" {
		t.Fatalf("json: %q %v", out, err)
	}
}

func TestMCPSetupAgentsMD(t *testing.T) {
	project := makeNSelfProject(t, "PROJECT_NAME=surf\nENV=dev\n")
	printed, _, err := runMCPSetupFlags(t, map[string]string{"agents-md": "true"})
	if err != nil || !agentsetup.Unedited(printed) {
		t.Fatalf("print: %v", err)
	}
	for _, verb := range []string{"init", "start", "mcp", "exec"} {
		if !strings.Contains(printed, "- `nself "+verb+"`:") {
			t.Errorf("AGENTS.md lacks canon verb %s", verb)
		}
	}
	if _, _, err = runMCPSetupFlags(t, map[string]string{"agents-md": "true", "write": "true"}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(project, "AGENTS.md")); string(b) != printed {
		t.Fatal("--agents-md --write must write exactly what --agents-md prints")
	}
	edited := "# mine\n"
	if err = os.WriteFile(filepath.Join(project, "AGENTS.md"), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err = runMCPSetupFlags(t, map[string]string{"agents-md": "true", "write": "true"}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(project, "AGENTS.md")); string(b) != edited {
		t.Fatal("hand-written AGENTS.md was overwritten")
	}
}

func TestMCPSetupWriteNeedsProject(t *testing.T) {
	t.Chdir(t.TempDir())
	if _, _, err := runMCPSetupFlags(t, map[string]string{"write": "true"}); err == nil || !strings.Contains(err.Error(), "E401") {
		t.Fatalf("write outside a project: %v", err)
	}
}
