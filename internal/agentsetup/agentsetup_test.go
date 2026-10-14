package agentsetup

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files")

var testVerbs = []Verb{{"init", "Initialize a project"}, {"start", "Boot the stack"}, {"mcp", "Start the MCP server"}}

func golden(t *testing.T, name, got string) {
	t.Helper()
	p := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("%v (run with -update)", err)
	}
	if string(want) != got {
		t.Fatalf("%s differs\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func TestAgentSetupClients(t *testing.T) {
	for _, c := range Clients {
		if !Valid(c) {
			t.Errorf("%s not valid", c)
		}
		golden(t, "client-"+c+".golden", Config(c)+"\n")
	}
	if Valid("vim") {
		t.Error("vim should be invalid")
	}
	if Path(ClaudeCode) != ".mcp.json" || Path(Cursor) != ".cursor/mcp.json" || Path(Codex) != "" || Path(Generic) != "" {
		t.Error("client paths")
	}
	if Config(ClaudeCode) != `{"mcpServers":{"nself":{"command":"nself","args":["mcp"]}}}` {
		t.Errorf("claude-code config = %s", Config(ClaudeCode))
	}
	if strings.Contains(Config(Cursor), "token") || strings.Contains(Config(Cursor), "env") {
		t.Error("stdio config must hold no secret")
	}
	if !strings.Contains(Note("/p"), "/p") {
		t.Error("note names the directory")
	}
}

func TestMerge(t *testing.T) {
	cases := []struct {
		name, in string
		changed  bool
		wantErr  bool
		check    func(t *testing.T, out string)
	}{
		{"empty", "", true, false, func(t *testing.T, out string) { golden(t, "merge-new.golden", out) }},
		{"blank", "  \n", true, false, nil},
		{"no mcpServers keeps order", `{"b":1,"a":{"x":[1,2]}}`, true, false, func(t *testing.T, out string) {
			if strings.Index(out, `"b"`) > strings.Index(out, `"a"`) || strings.Index(out, `"a"`) > strings.Index(out, "mcpServers") {
				t.Errorf("key order lost:\n%s", out)
			}
		}},
		{"other server kept", `{"mcpServers":{"zed":{"command":"z"}},"theme":"dark"}`, true, false, func(t *testing.T, out string) {
			golden(t, "merge-other.golden", out)
		}},
		{"nself present is untouched", `{"mcpServers":{"nself":{"command":"custom"}}}`, false, false, nil},
		{"not an object", `[1]`, false, true, nil},
		{"invalid", `{"a":`, false, true, nil},
		{"servers not object", `{"mcpServers":[]}`, false, true, nil},
		{"trailing data", `{"a":1} {"b":2}`, false, true, nil},
		{"trailing bracket", `{"keep":1}]`, false, true, nil},
		{"trailing brace", `{"keep":1}}`, false, true, nil},
		{"trailing whitespace", "{\"keep\":1}\n\n", true, false, nil},
		{"duplicate mcpServers merges into the last", `{"mcpServers":{},"mcpServers":{"other":{"command":"o"}}}`, true, false, func(t *testing.T, out string) {
			first := strings.Index(out, `"mcpServers"`)
			last := strings.LastIndex(out, `"mcpServers"`)
			if first == last || strings.Contains(out[first:last], serverName) || !strings.Contains(out[last:], `"`+serverName+`"`) {
				t.Errorf("nself must land in the last mcpServers only:\n%s", out)
			}
		}},
		{"nself only in the shadowed duplicate", `{"mcpServers":{"nself":{"command":"old"}},"mcpServers":{}}`, true, false, func(t *testing.T, out string) {
			last := strings.LastIndex(out, `"mcpServers"`)
			if !strings.Contains(out[last:], `"`+serverName+`"`) {
				t.Errorf("nself missing from the effective (last) mcpServers:\n%s", out)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, changed, err := Merge([]byte(c.in))
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, c.wantErr)
			}
			if err != nil {
				return
			}
			if changed != c.changed {
				t.Fatalf("changed = %v", changed)
			}
			if !changed && !bytes.Equal(out, []byte(c.in)) {
				t.Fatal("unchanged merge must return the input bytes")
			}
			if c.check != nil {
				c.check(t, string(out))
			}
			// Idempotent: merging the result again changes nothing.
			again, changed2, err := Merge(out)
			if err != nil || changed2 || !bytes.Equal(again, out) {
				t.Fatalf("second merge: changed=%v err=%v", changed2, err)
			}
		})
	}
}

func TestWriteClient(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []string{ClaudeCode, Cursor} {
		p, written, err := WriteClient(dir, c)
		if err != nil || !written {
			t.Fatalf("%s first write: written=%v err=%v", c, written, err)
		}
		first, _ := os.ReadFile(p)
		_, written, err = WriteClient(dir, c)
		second, _ := os.ReadFile(p)
		if err != nil || written || !bytes.Equal(first, second) {
			t.Fatalf("%s second write: written=%v err=%v", c, written, err)
		}
	}
	if _, _, err := WriteClient(dir, Codex); err == nil || !strings.Contains(err.Error(), "print-only") {
		t.Errorf("codex write err = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := WriteClient(dir, ClaudeCode); err == nil {
		t.Error("a malformed existing file must not be clobbered")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, ".mcp.json")); string(b) != "{bad" {
		t.Error("malformed file was changed")
	}
}

func TestWriteRefusesSymlink(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	target := filepath.Join(outside, "x.json")
	if err := os.WriteFile(target, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, ".mcp.json")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if _, _, err := WriteClient(dir, ClaudeCode); err == nil {
		t.Error("symlinked client file must be refused")
	}
	if b, _ := os.ReadFile(target); string(b) != "{}" {
		t.Error("file outside the project was written")
	}
}

func TestAgentsMDRender(t *testing.T) {
	got := Render(testVerbs)
	golden(t, "agents.golden.md", got)
	if got != Render(testVerbs) {
		t.Error("render is not deterministic")
	}
	if !Unedited(got) {
		t.Error("a fresh render must count as unedited")
	}
	first, body, _ := strings.Cut(got, "\n")
	if body != Body(testVerbs) || !strings.HasPrefix(first, "<!-- GENERATED BY nself (mcp setup) sha256:") {
		t.Error("marker line then body")
	}
	for _, bad := range []string{"", "# mine\n", first, first + "\n" + body + "x", strings.Replace(got, "Boot", "Start", 1), strings.Replace(got, "sha256:", "sha256:0", 1)} {
		if Unedited(bad) {
			t.Errorf("edited or foreign content counted as unedited: %q", bad[:min(len(bad), 40)])
		}
	}
	if !MayWrite(false, "") || !MayWrite(true, got) || MayWrite(true, "# mine\n") {
		t.Error("MayWrite")
	}
}

func TestWriteAgentsMD(t *testing.T) {
	dir := t.TempDir()
	p, written, err := WriteAgentsMD(dir, testVerbs)
	if err != nil || !written {
		t.Fatalf("create: %v %v", written, err)
	}
	if _, written, err = WriteAgentsMD(dir, testVerbs); err != nil || written {
		t.Fatalf("identical regenerate must not change the file: %v %v", written, err)
	}
	// An unedited file from an older registry is regenerated.
	old := Render(testVerbs[:1])
	if err := os.WriteFile(p, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, written, err = WriteAgentsMD(dir, testVerbs); err != nil || !written {
		t.Fatalf("stale generated file: %v %v", written, err)
	}
	if b, _ := os.ReadFile(p); string(b) != Render(testVerbs) {
		t.Error("not regenerated")
	}
	// A hand edit is never overwritten.
	edited := strings.Replace(Render(testVerbs), "AGENTS.md", "MY NOTES", 1)
	if err := os.WriteFile(p, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, written, err = WriteAgentsMD(dir, testVerbs); err != nil || written {
		t.Fatalf("edited file: %v %v", written, err)
	}
	if b, _ := os.ReadFile(p); string(b) != edited {
		t.Error("edited file was overwritten")
	}
	// A file nself never generated is never overwritten.
	if err := os.WriteFile(p, []byte("# hand written\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, written, _ = WriteAgentsMD(dir, testVerbs); written {
		t.Error("foreign file was overwritten")
	}
}

func TestWriteRefusesSymlinkedParentAndKeepsMode(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, ".cursor")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if _, _, err := WriteClient(dir, Cursor); err == nil {
		t.Error("a symlinked .cursor must be refused")
	}
	if _, err := os.Stat(filepath.Join(outside, "mcp.json")); err == nil {
		t.Error("file written outside the project")
	}
	// An existing file keeps its permission bits through a merge.
	d2 := t.TempDir()
	p := filepath.Join(d2, ".mcp.json")
	if err := os.WriteFile(p, []byte(`{"other":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, written, err := WriteClient(d2, ClaudeCode); err != nil || !written {
		t.Fatalf("merge: %v %v", written, err)
	}
	if fi, _ := os.Stat(p); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", fi.Mode().Perm())
	}
}
