//go:build !windows

package stack

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/database"
	"github.com/nself-org/cli/internal/plugin/readiness"
)

func TestUpdateStackRuntimeReadinessActivePlugin(t *testing.T) {
	dir := t.TempDir()
	runtimeDockerShim(t, dir)
	plugins := filepath.Join(dir, "plugins")
	active := filepath.Join(plugins, "fixture-full")
	if err := os.MkdirAll(active, 0700); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "plugin", "manifestv2", "testdata", "v2", "full.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(active, "plugin.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plugins, "non-directory"), []byte("ignored"), 0600); err != nil {
		t.Fatal(err)
	}
	r := NewRuntime(dir, &config.Config{PluginSystem: config.PluginSystemConfig{Dir: plugins}}, nil)
	t.Setenv("STACK_TEST_CONFIG", `{"services":{"plugin_fixture-full":{"image":"fixture:old"}}}`)
	t.Setenv("STACK_TEST_PS", `[{"ID":"fixture","Service":"plugin_fixture-full","State":"running","Ports":"3999/tcp"}]`)
	if err := r.readiness(context.Background()); err == nil || !strings.Contains(err.Error(), "published") {
		t.Fatalf("missing published port err=%v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/health" {
			t.Error("incorrect health endpoint")
		}
		_, _ = w.Write([]byte(`{"migrations":{"applied":2,"expected":2}}`))
	}))
	defer server.Close()
	_, port, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("STACK_TEST_PS", fmt.Sprintf(`[{"ID":"fixture","Service":"plugin_fixture-full","State":"running","Ports":"127.0.0.1:%s->3999/tcp"}]`, port))
	// S4: readiness is followed by Hasura table tracking of the active plugin.
	var tracked []database.PluginTables
	oldTrack := runtimeTrackTables
	runtimeTrackTables = func(_ context.Context, _ *config.Config, p []database.PluginTables) (database.TrackResult, error) {
		tracked = p
		return database.TrackResult{}, nil
	}
	t.Cleanup(func() { runtimeTrackTables = oldTrack })
	if err := r.Run(context.Background(), "readiness", &Journal{}); err != nil {
		t.Fatal(err)
	}
	if len(tracked) != 1 || tracked[0].Plugin != "fixture-full" || len(tracked[0].Tables) != 1 || tracked[0].Tables[0] != "np_fixture_full_items" {
		t.Fatalf("readiness did not track the plugin tables: %+v", tracked)
	}
	t.Setenv(readiness.EnvTimeout, "invalid")
	if err := r.readiness(context.Background()); err == nil || !strings.Contains(err.Error(), "plugin readiness timeout") {
		t.Fatalf("err=%v", err)
	}
}

func TestUpdateStackRuntimeRestartPinnedSnapshot(t *testing.T) {
	dir := t.TempDir()
	runtimeDockerShim(t, dir)
	r := NewRuntime(dir, &config.Config{}, nil)
	state, err := r.privateState()
	if err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"services":{"postgres":{"image":"sha256:old","pull_policy":"never"}}}`)
	path := filepath.Join(state, "update-stack-previous-retained.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	j := &Journal{PreviousCompose: path, PreviousComposeSHA256: hex.EncodeToString(sum[:])}
	if err := r.RestartPrevious(context.Background(), j); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"pull", "start"} {
		if err := r.Run(context.Background(), name, j); err != nil {
			t.Fatal(err)
		}
	}
}
