package mcpgen_test

// Purpose: shared harness of the mcpgen tests: the stub child, the hermetic
// machine registry of the real command tree, a fixture registry with plugin
// commands, and the golden helpers.
// Constraints: no network; HOME and the plugin dir point into a temp dir; the
// stub child is built once from internal/invoke/testdata/stubcli.

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/nself-org/cli/cmd/commands"
	"github.com/nself-org/cli/internal/cmdregistry"
)

var update = flag.Bool("update", false, "rewrite the golden files")

var stubPath string

func TestMain(m *testing.M) {
	flag.Parse()
	dir, err := os.MkdirTemp("", "mcpgen-test-")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", dir)
	os.Setenv("USERPROFILE", dir)
	os.Setenv("NSELF_PLUGIN_DIR", filepath.Join(dir, "plugins"))
	name := "stubcli"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	stubPath = filepath.Join(dir, name)
	build := exec.Command("go", "build", "-o", stubPath, "../invoke/testdata/stubcli")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		os.RemoveAll(dir)
		panic("building the stub child: " + err.Error() + "\n" + string(out))
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

var hermetic struct {
	once sync.Once
	reg  *cmdregistry.Registry
	err  error
}

// hermeticRegistry is the v1.5 machine registry of the real command tree with
// no plugin installed.
func hermeticRegistry(t testing.TB) *cmdregistry.Registry {
	t.Helper()
	hermetic.once.Do(func() { hermetic.reg, hermetic.err = commands.MachineRegistry() })
	if hermetic.err != nil {
		t.Fatalf("machine registry: %v", hermetic.err)
	}
	return hermetic.reg
}

// golden compares got with testdata/<name>, or rewrites it under -update.
func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/mcpgen/ -run %s -update)", err, t.Name())
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s differs from the committed golden; run go test ./internal/mcpgen/ -run %s -update and review the diff", name, t.Name())
	}
}

func marshalIndent(t testing.TB, v any) []byte {
	t.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
