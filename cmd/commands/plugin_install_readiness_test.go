package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"

	"github.com/spf13/cobra"
)

// readinessProject makes a project dir (cwd) and a plugin dir holding a
// boot-migrations plugin whose /health is served by an httptest server that
// reports applied of expected.
func readinessProject(t *testing.T, name string, applied, expected int) (pluginDir string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "internal", "plugin", "manifestv2", "testdata", "v2", "full.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", t.TempDir())
	proj := t.TempDir()
	if err := os.WriteFile(filepath.Join(proj, ".env"), []byte("PROJECT_NAME=rdy\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(proj)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"status":"ok","migrations":{"applied":%d,"expected":%d}}`, applied, expected)
	}))
	t.Cleanup(srv.Close)
	var port int
	if _, err := fmt.Sscanf(srv.URL[strings.LastIndex(srv.URL, ":")+1:], "%d", &port); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	m["name"], m["schema"], m["tables"], m["port"] = name, "np_"+name, []string{"np_" + name + "_items"}, port
	m["routes"] = []map[string]any{{"path": "/" + name, "upstream_port": port}}
	m["service"] = map[string]any{"compose": "docker-compose.plugin.yml", "healthcheck": "/health", "kind": "compose", "port": port}
	out, _ := json.Marshal(m)
	pluginDir = t.TempDir()
	if err := os.MkdirAll(filepath.Join(pluginDir, name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, name, "plugin.json"), out, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NSELF_PLUGIN_READY_TIMEOUT", "200ms")
	return pluginDir
}

func stubReadinessSeams(t *testing.T, running bool) (started *int32) {
	t.Helper()
	oldStart, oldRun, oldEvery := pluginStartStack, pluginStackRunning, pluginReadyEvery
	var n int32
	pluginStartStack = func(context.Context) error { atomic.AddInt32(&n, 1); return nil }
	pluginStackRunning = func(context.Context, string) bool { return running }
	pluginReadyEvery = 10 * time.Millisecond
	t.Cleanup(func() { pluginStartStack, pluginStackRunning, pluginReadyEvery = oldStart, oldRun, oldEvery })
	return &n
}

func readinessCmd() (*cobra.Command, *bytes.Buffer) {
	c := &cobra.Command{Use: "install"}
	var buf bytes.Buffer
	c.SetErr(&buf)
	c.SetOut(&buf)
	return c, &buf
}

// TestPluginInstallReadinessE118: a plugin whose migration never applies fails
// the install with E118 (exit 2) in v1.5 after the timeout; v1.4 warns and exits 0.
func TestPluginInstallReadinessE118(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		pluginDir := readinessProject(t, "stuck", 0, 1)
		started := stubReadinessSeams(t, true)
		c, stderr := readinessCmd()
		begin := time.Now()
		err := pluginInstallReadiness(c, pluginDir, []string{"stuck"})
		if time.Since(begin) < 150*time.Millisecond {
			t.Errorf("returned in %s: it did not wait for the timeout", time.Since(begin))
		}
		if compat.V15() {
			d := errs.Describe(err)
			if err == nil || d == nil || d.Code != "E118" || d.ExitCode != 2 {
				t.Fatalf("v1.5: err = %v detail = %+v, want E118 exit 2", err, d)
			}
			if !strings.Contains(err.Error(), "stuck") || *started != 1 {
				t.Errorf("v1.5: error %q should name the plugin; start calls = %d (want 1)", err, *started)
			}
			return
		}
		if err != nil {
			t.Fatalf("v1.4: err = %v, want nil", err)
		}
		if !strings.Contains(stderr.String(), "warning:") || !strings.Contains(stderr.String(), "stuck") || *started != 0 {
			t.Errorf("v1.4: stderr = %q, start calls = %d (want a warning naming the plugin and no start)", stderr.String(), *started)
		}
	})
}

func TestPluginInstallReadinessOKAndSkips(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		pluginDir := readinessProject(t, "fine", 1, 1)
		stubReadinessSeams(t, true)
		c, _ := readinessCmd()
		if err := pluginInstallReadiness(c, pluginDir, []string{"fine"}); err != nil {
			t.Fatalf("ready plugin: %v", err)
		}
		// Third-party URL, unknown name: nothing to wait on.
		if err := pluginInstallReadiness(c, pluginDir, []string{"https://example.invalid/p.tgz", "ghost"}); err != nil {
			t.Fatalf("no members: %v", err)
		}
	})
}

// TestPluginInstallReadinessStackDown: v1.5 with the stack down says so and does not wait or start.
func TestPluginInstallReadinessStackDown(t *testing.T) {
	compattest.Set(t, true)
	pluginDir := readinessProject(t, "stuck", 0, 1)
	started := stubReadinessSeams(t, false)
	c, stderr := readinessCmd()
	if err := pluginInstallReadiness(c, pluginDir, []string{"stuck"}); err != nil {
		t.Fatal(err)
	}
	if *started != 0 || !strings.Contains(stderr.String(), "next `nself start`") {
		t.Errorf("start=%d stderr=%q", *started, stderr.String())
	}
}

// TestPluginInstallReadinessOutsideProject: no project, no wait.
func TestPluginInstallReadinessOutsideProject(t *testing.T) {
	compattest.Set(t, true)
	pluginDir := readinessProject(t, "stuck", 0, 1)
	t.Chdir(t.TempDir())
	started := stubReadinessSeams(t, true)
	c, _ := readinessCmd()
	if err := pluginInstallReadiness(c, pluginDir, []string{"stuck"}); err != nil || *started != 0 {
		t.Fatalf("err=%v started=%d", err, *started)
	}
}

// TestPluginInstallReadinessStartError: a failing start is returned with the plugin named.
func TestPluginInstallReadinessStartError(t *testing.T) {
	compattest.Set(t, true)
	pluginDir := readinessProject(t, "stuck", 0, 1)
	stubReadinessSeams(t, true)
	pluginStartStack = func(context.Context) error { return errors.New("compose failed") }
	c, _ := readinessCmd()
	err := pluginInstallReadiness(c, pluginDir, []string{"stuck"})
	if err == nil || !strings.Contains(err.Error(), "stuck") || !strings.Contains(err.Error(), "compose failed") {
		t.Fatalf("err = %v", err)
	}
}
