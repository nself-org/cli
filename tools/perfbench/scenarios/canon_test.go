package scenarios

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// TestCanonScenarios proves both scenarios register, probe the G8 commands, build
// the 31-plugin HOME and run the binary with that HOME (and NSELF_V15 only for
// the v1.5 scenario). A stub binary records what it saw.
func TestCanonScenarios(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub binary is a shell script")
	}
	names := map[string]bool{}
	for _, n := range Names() {
		names[n] = true
	}
	for _, n := range []string{"cold-start-plugins", "cold-start-plugins-v15"} {
		if !names[n] {
			t.Fatalf("scenario %s is not registered (have %v)", n, Names())
		}
	}

	home := t.TempDir()
	if err := WritePluginHome(home, PluginCount); err != nil {
		t.Fatal(err)
	}
	cmdRe, binRe := regexp.MustCompile(`^[a-z][a-z0-9-]*$`), regexp.MustCompile(`^nself-[a-z][a-z0-9-]*$`)
	entries, _ := filepath.Glob(filepath.Join(home, ".nself", "plugins", "*", "plugin.json"))
	if len(entries) != 31 {
		t.Fatalf("%d manifests, want 31", len(entries))
	}
	for _, f := range entries {
		var m struct {
			Version  int    `json:"manifest_version"`
			Binary   string `json:"binaryName"`
			Type     string `json:"pluginType"`
			CLI      []any  `json:"cliCommands"`
			Commands struct {
				Command, Binary string
				Subcommands     []struct{ Name string }
			} `json:"commands"`
		}
		b, _ := os.ReadFile(f)
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		c := m.Commands
		if m.Version != 2 || m.Type != "cli" || len(m.CLI) == 0 || m.Binary != c.Binary || !cmdRe.MatchString(c.Command) || !binRe.MatchString(c.Binary) || len(c.Subcommands) != 3 {
			t.Errorf("%s: not a v2 commands block with the v1 compatibility keys: %+v", f, m)
		}
		st, err := os.Stat(filepath.Join(home, ".nself", "plugins", "bin", c.Binary))
		if err != nil || st.Mode()&0o111 == 0 {
			t.Errorf("%s: stub binary missing or not executable: %v", c.Binary, err)
		}
	}

	out := filepath.Join(t.TempDir(), "seen.txt")
	stub := filepath.Join(t.TempDir(), "nself")
	script := "#!/bin/sh\n" +
		"n=$(ls \"$HOME/.nself/plugins\" | grep -c cli)\n" +
		"echo \"$1 v15=${NSELF_V15:-} plugins=$n\" >> \"$STUB_OUT\"\n" +
		"[ \"$1\" = status ] && exit 1\nexit 0\n"
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		v15  string
	}{{"cold-start-plugins", ""}, {"cold-start-plugins-v15", "1"}} {
		_ = os.Remove(out)
		sc, _ := Get(c.name)
		samples, err := sc.Run(context.Background(), Config{Bin: stub, Runs: 2, Warmup: 1, Env: []string{"STUB_OUT=" + out}})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		metrics := map[string]int{}
		for _, s := range samples {
			metrics[s.Metric]++
			if s.Unit != "ms" {
				t.Errorf("%s: unit %q", c.name, s.Metric)
			}
		}
		for _, m := range []string{"cold_start.version", "cold_start.help", "cold_start.status"} {
			if metrics[m] != 2 {
				t.Errorf("%s: %d samples of %s, want 2", c.name, metrics[m], m)
			}
		}
		seen, _ := os.ReadFile(out)
		lines := strings.Split(strings.TrimSpace(string(seen)), "\n")
		if len(lines) != 9 { // 3 probes x (1 warm-up + 2 runs)
			t.Fatalf("%s: stub ran %d times, want 9:\n%s", c.name, len(lines), seen)
		}
		for _, l := range lines {
			if !strings.HasSuffix(l, "plugins=31") || !strings.Contains(l, "v15="+c.v15+" ") {
				t.Errorf("%s: stub saw %q, want 31 plugins and v15=%q", c.name, l, c.v15)
			}
		}
	}
}
