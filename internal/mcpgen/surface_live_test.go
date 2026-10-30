package mcpgen_test

// Purpose: the live tool set of one `nself mcp` process: what is listed before
// a project exists, the refresh after a successful call, and the cases that
// keep the previous tools.
// Constraints: the registry source and the runner are fakes; the project root
// and the plugin directory are temp dirs; no child process runs.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/invoke"
	"github.com/nself-org/cli/internal/mcpgen"
	"github.com/nself-org/cli/schemas"
)

// surfaceRig wires a Surface to fakes and records what it asked of them.
type surfaceRig struct {
	srv       *server.MCPServer
	surf      *mcpgen.Surface
	reg       *cmdregistry.Registry
	pluginDir string

	mu       sync.Mutex
	project  string
	regErr   error
	rebuilds int // Registry(true) calls
	builds   int // every Registry call
	runExit  int
	runErr   error
	ranDir   string
	warnings []string
}

func newSurfaceRig(t *testing.T, project string) *surfaceRig {
	t.Helper()
	r := &surfaceRig{srv: server.NewMCPServer("t", "0"), reg: fixtureRegistry(t, false, false), pluginDir: t.TempDir(), project: project}
	hand := func(s *server.MCPServer) {
		handler := func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) { return nil, nil }
		s.AddTool(mcp.NewTool("zz_hand_read", mcp.WithReadOnlyHintAnnotation(true)), handler)
		s.AddTool(mcp.NewTool("zz_hand_write", mcp.WithReadOnlyHintAnnotation(false)), handler)
	}
	r.surf = mcpgen.NewSurface(mcpgen.SurfaceOptions{
		Server: r.srv, Hand: hand, V15: true, Schema: schemas.Lookup,
		Registry: func(rebuild bool) (*cmdregistry.Registry, error) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.builds++
			if rebuild {
				r.rebuilds++
			}
			if r.regErr != nil {
				return nil, r.regErr
			}
			return r.reg, nil
		},
		Run: func(_ context.Context, _ *cmdregistry.Registry, _ string, _ invoke.Request, dir string) (invoke.Result, error) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.ranDir = dir
			return invoke.Result{ExitCode: r.runExit}, r.runErr
		},
		PluginDir: func() string { return r.pluginDir },
		Project: func() string {
			r.mu.Lock()
			defer r.mu.Unlock()
			return r.project
		},
		Warn: func(s string) { r.warnings = append(r.warnings, s) },
	})
	return r
}

func (r *surfaceRig) setProject(dir string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.project = dir
}

func (r *surfaceRig) listed() map[string]bool {
	out := map[string]bool{}
	for name := range r.srv.ListTools() {
		out[name] = true
	}
	return out
}

func TestSurfaceStartWithoutProject(t *testing.T) {
	r := newSurfaceRig(t, "")
	reg, err := r.surf.Start()
	if err != nil || reg != r.reg {
		t.Fatalf("Start: %v, registry %v", err, reg)
	}
	if len(r.warnings) != 1 || !strings.Contains(r.warnings[0], "no nself project") {
		t.Fatalf("want one no-project warning, got %q", r.warnings)
	}
	got := r.listed()
	if !got["zz_hand_read"] || got["zz_hand_write"] {
		t.Errorf("hand tools without a project: read=%v write=%v", got["zz_hand_read"], got["zz_hand_write"])
	}
	if !got["nself_status"] {
		t.Error("the read-only generated tool nself_status is not listed")
	}
	if got["nself_build"] {
		t.Error("the write tool nself_build is listed without a project")
	}
}

func TestSurfaceStartWithProject(t *testing.T) {
	r := newSurfaceRig(t, t.TempDir())
	if _, err := r.surf.Start(); err != nil {
		t.Fatal(err)
	}
	if len(r.warnings) != 0 {
		t.Errorf("unexpected warnings %q", r.warnings)
	}
	got := r.listed()
	for _, name := range []string{"zz_hand_read", "zz_hand_write", "nself_status", "nself_build"} {
		if !got[name] {
			t.Errorf("%s is not listed with a project", name)
		}
	}
}

func TestSurfaceStartRegistryErrorKeepsHandTools(t *testing.T) {
	r := newSurfaceRig(t, t.TempDir())
	r.regErr = errors.New("registry broke")
	reg, err := r.surf.Start()
	if err == nil || reg != nil || !strings.Contains(err.Error(), "registry broke") {
		t.Fatalf("Start: reg=%v err=%v", reg, err)
	}
	got := r.listed()
	if !got["zz_hand_read"] || !got["zz_hand_write"] || got["nself_status"] {
		t.Errorf("after a failed start only the hand tools stay: %v", got)
	}
}

// A successful call that finds a project where there was none rebuilds the
// registry and lists every tool; the call itself ran in the new project.
func TestSurfaceInvokerRefreshesAfterInit(t *testing.T) {
	r := newSurfaceRig(t, "")
	reg, err := r.surf.Start()
	if err != nil {
		t.Fatal(err)
	}
	if r.listed()["nself_build"] {
		t.Fatal("precondition: nself_build must be hidden without a project")
	}
	invoker := r.surf.Invoker(reg)

	// A call with no change in the inputs does not rebuild.
	if _, err := invoker(context.Background(), "status", invoke.Request{}); err != nil {
		t.Fatal(err)
	}
	if r.rebuilds != 0 || r.ranDir != "" {
		t.Fatalf("no input change: rebuilds=%d dir=%q", r.rebuilds, r.ranDir)
	}

	project := t.TempDir()
	r.setProject(project)
	res, err := invoker(context.Background(), "init", invoke.Request{})
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("init: %+v %v", res, err)
	}
	if r.rebuilds != 1 {
		t.Fatalf("want one rebuild after the project appeared, got %d", r.rebuilds)
	}
	if !r.listed()["nself_build"] || !r.listed()["zz_hand_write"] {
		t.Error("the write tools are still hidden after init")
	}
	// The next call runs in the project root.
	if _, err := invoker(context.Background(), "status", invoke.Request{}); err != nil {
		t.Fatal(err)
	}
	if r.ranDir != project {
		t.Errorf("ran in %q, want %q", r.ranDir, project)
	}
	if r.rebuilds != 1 {
		t.Errorf("inputs unchanged since the rebuild, got %d rebuilds", r.rebuilds)
	}
}

func TestSurfaceInvokerRefreshesOnPluginChange(t *testing.T) {
	r := newSurfaceRig(t, t.TempDir())
	reg, err := r.surf.Start()
	if err != nil {
		t.Fatal(err)
	}
	plug := filepath.Join(r.pluginDir, "fixp")
	if err := os.MkdirAll(plug, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plug, "plugin.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.surf.Invoker(reg)(context.Background(), "add", invoke.Request{}); err != nil {
		t.Fatal(err)
	}
	if r.rebuilds != 1 {
		t.Fatalf("a new plugin entry must trigger one rebuild, got %d", r.rebuilds)
	}
	// Changing the manifest changes the inputs again.
	if err := os.WriteFile(filepath.Join(plug, "plugin.json"), []byte(`{"v":1,"more":"bytes"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.surf.Invoker(reg)(context.Background(), "add", invoke.Request{}); err != nil {
		t.Fatal(err)
	}
	if r.rebuilds != 2 {
		t.Errorf("a changed manifest must trigger a rebuild, got %d", r.rebuilds)
	}
}

// A failed call (an error or a non-zero exit) never refreshes, even when the
// project appeared.
func TestSurfaceInvokerNoRefreshOnFailure(t *testing.T) {
	for name, set := range map[string]func(*surfaceRig){
		"non-zero exit": func(r *surfaceRig) { r.runExit = 1 },
		"runner error":  func(r *surfaceRig) { r.runErr = errors.New("spawn failed") },
	} {
		t.Run(name, func(t *testing.T) {
			r := newSurfaceRig(t, "")
			reg, err := r.surf.Start()
			if err != nil {
				t.Fatal(err)
			}
			set(r)
			r.setProject(t.TempDir())
			res, err := r.surf.Invoker(reg)(context.Background(), "init", invoke.Request{})
			if (err == nil) != (r.runErr == nil) || res.ExitCode != r.runExit {
				t.Fatalf("the runner's result must pass through: %+v %v", res, err)
			}
			if r.rebuilds != 0 || r.listed()["nself_build"] {
				t.Errorf("a failed call refreshed the tools: rebuilds=%d", r.rebuilds)
			}
		})
	}
}

// A failed rebuild keeps the previous tools, warns, and is retried after the
// next successful call.
func TestSurfaceRefreshFailureKeepsPreviousTools(t *testing.T) {
	r := newSurfaceRig(t, "")
	reg, err := r.surf.Start()
	if err != nil {
		t.Fatal(err)
	}
	invoker := r.surf.Invoker(reg)
	r.setProject(t.TempDir())
	r.regErr = errors.New("registry unavailable")
	if _, err := invoker(context.Background(), "init", invoke.Request{}); err != nil {
		t.Fatal(err)
	}
	last := r.warnings[len(r.warnings)-1]
	if !strings.Contains(last, "keeping the previous one") || !strings.Contains(last, "registry unavailable") {
		t.Fatalf("warning %q", last)
	}
	if r.listed()["nself_build"] {
		t.Error("the tool list changed although the rebuild failed")
	}
	r.regErr = nil
	if _, err := invoker(context.Background(), "init", invoke.Request{}); err != nil {
		t.Fatal(err)
	}
	if !r.listed()["nself_build"] {
		t.Error("the next successful call did not retry the rebuild")
	}
}

func TestSurfaceWithoutHandTools(t *testing.T) {
	r := newSurfaceRig(t, t.TempDir())
	surf := mcpgen.NewSurface(mcpgen.SurfaceOptions{
		Server: r.srv, V15: true, Schema: schemas.Lookup,
		Registry:  func(bool) (*cmdregistry.Registry, error) { return r.reg, nil },
		PluginDir: func() string { return filepath.Join(r.pluginDir, "missing") },
		Project:   func() string { return "/p" },
	})
	if _, err := surf.Start(); err != nil {
		t.Fatal(err)
	}
	got := r.listed()
	if got["zz_hand_read"] || !got["nself_build"] {
		t.Errorf("v1.5 surface lists generated tools only: %v", got)
	}
}

func TestRowParamNames(t *testing.T) {
	table, err := mcpgen.LoadTable()
	if err != nil {
		t.Fatal(err)
	}
	withParams := 0
	for i := range table.Rows {
		row := &table.Rows[i]
		names := row.ParamNames()
		if names == nil || len(names) != len(row.Params) {
			t.Fatalf("%s: %d names for %d params", row.Name, len(names), len(row.Params))
		}
		for j, p := range row.Params {
			if names[j] != p.Name {
				t.Errorf("%s: name %d is %q, want %q", row.Name, j, names[j], p.Name)
			}
		}
		if len(names) > 0 {
			withParams++
		}
	}
	if withParams == 0 {
		t.Error("no row of the table has params; the test proves nothing")
	}
}
