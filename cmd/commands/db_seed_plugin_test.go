package commands

// Tests for `nself db seed --plugin / --all-plugins` (P7-PLUG-32). A fake
// runtime stands in for the docker funnel; the plugin dir is a temp dir.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/output"
	"github.com/nself-org/cli/internal/seed"
	"github.com/spf13/pflag"
)

type fakeSeedRT struct {
	services []string
	execs    [][]string
	execErr  error
}

func (f *fakeSeedRT) FindContainer(_ context.Context, svc string) (string, error) {
	f.services = append(f.services, svc)
	return "nself_" + svc, nil
}

func (f *fakeSeedRT) Exec(_ context.Context, _ string, argv []string) (string, string, error) {
	f.execs = append(f.execs, argv)
	return "", "", f.execErr
}

// writeSeedPlugin installs a v2 fixture plugin; seedArgv nil means no seed block.
func writeSeedPlugin(t *testing.T, dir, name string, seedArgv []string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "internal", "plugin", "manifestv2", "testdata", "v2", "full.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	m["name"] = name
	m["schema"] = "np_" + name
	if seedArgv == nil {
		delete(m, "seed")
	} else {
		m["seed"] = map[string]any{"command": seedArgv}
	}
	out, _ := json.Marshal(m)
	root := filepath.Join(dir, name)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "plugin.json"), out, 0o644); err != nil {
		t.Fatal(err)
	}
	frag := "services:\n  " + name + "-svc:\n    image: x\n  other:\n    image: y\n"
	if err := os.WriteFile(filepath.Join(root, "docker-compose.plugin.yml"), []byte(frag), 0o644); err != nil {
		t.Fatal(err)
	}
}

// seedHarness wires the seams and returns the plugin dir, runtime and stdout.
func seedHarness(t *testing.T, env string) (string, *fakeSeedRT, *bytes.Buffer) {
	t.Helper()
	dir := t.TempDir()
	rt := &fakeSeedRT{}
	out := &bytes.Buffer{}
	oldC, oldD, oldR, oldW := pluginSeedConfig, pluginSeedDir, pluginSeedRuntime, pilotWriter
	pluginSeedConfig = func() (*config.Config, error) { return &config.Config{Env: env, ProjectName: "p"}, nil }
	pluginSeedDir = func() string { return dir }
	pluginSeedRuntime = func(string, string) seed.PluginRuntime { return rt }
	pilotWriter = func() output.Writer { return output.Writer{Out: out, Err: &bytes.Buffer{}} }
	output.ResetState()
	t.Cleanup(func() {
		pluginSeedConfig, pluginSeedDir, pluginSeedRuntime, pilotWriter = oldC, oldD, oldR, oldW
		output.ResetState()
		dbSeedCmd.Flags().VisitAll(func(f *pflag.Flag) { _ = f.Value.Set(f.DefValue); f.Changed = false })
		RootCmd.PersistentFlags().VisitAll(func(f *pflag.Flag) { _ = f.Value.Set(f.DefValue); f.Changed = false })
	})
	return dir, rt, out
}

func runSeedFlags(t *testing.T, args ...string) error {
	t.Helper()
	if err := dbSeedCmd.ParseFlags(args); err != nil {
		t.Fatal(err)
	}
	return dbSeedCmd.RunE(dbSeedCmd, dbSeedCmd.Flags().Args())
}

func TestPluginSeedProdGuardE403(t *testing.T) {
	dir, rt, _ := seedHarness(t, "prod")
	writeSeedPlugin(t, dir, "alpha", []string{"alpha", "seed", "--idempotent"})

	err := runSeedFlags(t, "--plugin", "alpha")
	var ce *errs.CLIError
	if !errors.As(err, &ce) || ce.Code != "E403" {
		t.Fatalf("want E403, got %v", err)
	}
	if got := errs.ExitCodeFor(err); got != 4 {
		t.Fatalf("exit code = %d, want 4", got)
	}
	if len(rt.execs) != 0 {
		t.Fatalf("seed ran on prod without --force: %v", rt.execs)
	}

	// --force lifts the refusal, and the argv runs verbatim in the first service.
	dbSeedCmd.Flags().VisitAll(func(f *pflag.Flag) { _ = f.Value.Set(f.DefValue); f.Changed = false })
	if err := runSeedFlags(t, "--plugin", "alpha", "--force"); err != nil {
		t.Fatalf("--force: %v", err)
	}
	want := [][]string{{"alpha", "seed", "--idempotent"}}
	if !reflect.DeepEqual(rt.execs, want) || !reflect.DeepEqual(rt.services, []string{"alpha-svc"}) {
		t.Fatalf("execs=%v services=%v", rt.execs, rt.services)
	}
}

func TestPluginSeedNoCommandE127(t *testing.T) {
	dir, rt, _ := seedHarness(t, "dev")
	writeSeedPlugin(t, dir, "bare", nil)

	err := runSeedFlags(t, "--plugin", "bare")
	var ce *errs.CLIError
	if !errors.As(err, &ce) || ce.Code != "E127" {
		t.Fatalf("want E127, got %v", err)
	}
	if got := errs.ExitCodeFor(err); got != 1 {
		t.Fatalf("exit code = %d, want 1", got)
	}
	if len(rt.execs) != 0 {
		t.Fatalf("exec ran without a seed command: %v", rt.execs)
	}

	dbSeedCmd.Flags().VisitAll(func(f *pflag.Flag) { _ = f.Value.Set(f.DefValue); f.Changed = false })
	err = runSeedFlags(t, "--plugin", "missing")
	if !errors.As(err, &ce) || ce.Code != "E100" {
		t.Fatalf("not installed: want E100, got %v", err)
	}
}

func TestPluginSeedAllPluginsJSON(t *testing.T) {
	dir, rt, out := seedHarness(t, "dev")
	writeSeedPlugin(t, dir, "alpha", []string{"alpha", "seed"})
	writeSeedPlugin(t, dir, "bare", nil)
	writeSeedPlugin(t, dir, "zeta", []string{"zeta", "go; rm -rf /"})

	if err := runSeedFlags(t, "--all-plugins", "--json"); err != nil {
		t.Fatal(err)
	}
	var env struct {
		SchemaVersion string `json:"schema_version"`
		Command       string `json:"command"`
		Data          struct {
			Plugins []struct{ Name, Result string } `json:"plugins"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("one envelope expected: %v\n%s", err, out.String())
	}
	got := map[string]string{}
	for _, p := range env.Data.Plugins {
		got[p.Name] = p.Result
	}
	want := map[string]string{"alpha": "seeded", "bare": "skipped", "zeta": "seeded"}
	if env.Command != "db seed" || !reflect.DeepEqual(got, want) || len(env.Data.Plugins) != 3 {
		t.Fatalf("envelope = %+v", env)
	}
	// The shell metacharacters stay one argv element: no shell parsing.
	if len(rt.execs) != 2 || !reflect.DeepEqual(rt.execs[1], []string{"zeta", "go; rm -rf /"}) {
		t.Fatalf("execs = %v", rt.execs)
	}
}

func TestPluginSeedFailureIsCoded(t *testing.T) {
	dir, rt, _ := seedHarness(t, "dev")
	rt.execErr = errors.New("boom")
	writeSeedPlugin(t, dir, "alpha", []string{"alpha"})
	err := runSeedFlags(t, "--all-plugins")
	var ce *errs.CLIError
	if !errors.As(err, &ce) || ce.Code != "E250" {
		t.Fatalf("want E250, got %v", err)
	}
}
