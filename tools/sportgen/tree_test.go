package main

// Tree tests: run core-services and f02 with the real servicecatalog and
// cmdinventory built from the current tree, and compare what they write with
// the committed files. The fake-runner tests prove the templating; these prove
// the generated output on this tree equals what is committed.

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// treeRunner runs the generators' build and query commands for real. `go build
// -o bin/X ./tools/X` builds into a temp dir instead of the tree and `bin/X`
// runs that binary, both in the repo root. Only servicecatalog and cmdinventory
// are built: openapigen and facts write into the tree, so they are not run.
type treeRunner struct {
	root, bin string
	built     map[string]bool
}

var treeTools = map[string]bool{"servicecatalog": true, "cmdinventory": true}

func newTreeRunner(t *testing.T) *treeRunner {
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	return &treeRunner{root: root, bin: t.TempDir(), built: map[string]bool{}}
}

func (r *treeRunner) exec(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = r.root
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s %s: %v: %s", name, strings.Join(args, " "), err, errb.String())
	}
	return out.String(), nil
}

func (r *treeRunner) Run(name string, args ...string) (string, error) {
	if name == "go" && len(args) == 5 && args[0] == "build" && args[2] == "-o" {
		tool := filepath.Base(args[3])
		if !treeTools[tool] {
			return "", nil
		}
		r.built[tool] = true
		return r.exec("go", "build", args[1], "-o", filepath.Join(r.bin, tool), args[4])
	}
	if tool := strings.TrimPrefix(name, "bin/"); tool != name && treeTools[tool] {
		if !r.built[tool] {
			return "", fmt.Errorf("%s run before it was built", name)
		}
		return r.exec(filepath.Join(r.bin, tool), args...)
	}
	return "", nil
}

func TestSportGenCoreF02F17TreeCoreServicesEqualsCommitted(t *testing.T) {
	e := newEnv(t)
	e.d.Runner = newTreeRunner(t)
	if code := RunWith(e.d, []string{"core-services"}); code != 0 {
		t.Fatalf("code %d: %s", code, e.err)
	}
	wantEq(t, coreServicesOut, e.read(t, coreServicesOut), readRepo(t, coreServicesOut))
}

func TestSportGenCoreF02F17TreeF02EqualsCommitted(t *testing.T) {
	e := newEnv(t)
	e.d.Runner = newTreeRunner(t)
	// Commands.md is edited in place, so the run starts from the committed page.
	if err := os.WriteFile(filepath.Join(e.d.Root, f02Wiki), []byte(readRepo(t, f02Wiki)), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := RunWith(e.d, []string{"f02"}); code != 0 {
		t.Fatalf("code %d: %s", code, e.err)
	}
	for _, f := range []string{f02JSON, f02Reg, f02Canon, f02Wiki} {
		wantEq(t, f, e.read(t, f), readRepo(t, f))
	}
}

// A plugins value that is an array is refused, not read as plugin names.
func TestSportGenCoreF02F17F17PluginsArrayIsAnError(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "registry.json")
	os.WriteFile(p, []byte(`{"plugins":[{"status":"stable"},{"status":"beta"}]}`), 0o644)
	rows, err := registryPlugins(p)
	if err == nil || !strings.Contains(err.Error(), "plugins is not an object") || rows != nil {
		t.Fatalf("rows %v err %v", rows, err)
	}
}
