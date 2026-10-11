package commands

// plugin_install_track_test.go — P7-PLUG-78: `plugin install` (v1.5) tracks the
// installed plugin's manifest tables after readiness; failure is E140 (exit 2).
// Constraints: package tests run with tracking switched off (init below) so the
// older readiness tests never reach Hasura; each test here switches it back on.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/database"
	"github.com/nself-org/cli/internal/errs"
	"github.com/spf13/cobra"
)

func init() {
	// The readiness tests of P7-PLUG-19 run with a stub stack and no Hasura.
	pluginTrack = func(*cobra.Command, string, []string) error { return nil }
}

// realTracking runs the real tracking step for the test, with Hasura stubbed.
func realTracking(t *testing.T, track func([]database.PluginTables) error, exportBody string, exportErr error) (calls, exports *int) {
	t.Helper()
	oldTrack, oldExport, oldStep, oldState := bundleTrackTables, bundleExportMetadata, pluginTrack, pluginStackState
	calls, exports = new(int), new(int)
	bundleTrackTables = func(_ context.Context, _ *config.Config, p []database.PluginTables) (database.TrackResult, error) {
		*calls++
		if track != nil {
			return database.TrackResult{}, track(p)
		}
		return database.TrackResult{}, nil
	}
	bundleExportMetadata = func(context.Context, *config.Config) ([]byte, error) {
		*exports++
		return []byte(exportBody), exportErr
	}
	pluginTrack = pluginInstallTracking
	// Follow the readiness stub (stubReadinessSeams) unless a test overrides it.
	pluginStackState = func(ctx context.Context, p string) (bool, error) { return pluginStackRunning(ctx, p), nil }
	t.Cleanup(func() {
		bundleTrackTables, bundleExportMetadata, pluginTrack, pluginStackState = oldTrack, oldExport, oldStep, oldState
	})
	return calls, exports
}

const fineMeta = `{"sources":[{"name":"default","kind":"postgres","tables":[{"table":{"schema":"public","name":"np_fine_items"}}]}]}`

// TestPluginInstallTracksV15: after readiness ok the plugin's tables go to the
// tracker, then export_metadata verifies them; no error.
func TestPluginInstallTracksV15(t *testing.T) {
	compattest.Set(t, true)
	pluginDir := readinessProject(t, "fine", 1, 1)
	stubReadinessSeams(t, true)
	var got []database.PluginTables
	calls, exports := realTracking(t, func(p []database.PluginTables) error { got = p; return nil }, fineMeta, nil)
	c, _ := readinessCmd()
	if err := pluginInstallReadiness(c, pluginDir, []string{"fine"}); err != nil {
		t.Fatal(err)
	}
	if *calls != 1 || *exports != 1 || len(got) != 1 || got[0].Plugin != "fine" || strings.Join(got[0].Tables, ",") != "np_fine_items" {
		t.Fatalf("tracker calls=%d exports=%d tables=%+v", *calls, *exports, got)
	}
	// Third-party or unknown names and plugins without tables never reach Hasura.
	*calls, *exports = 0, 0
	if err := pluginInstallReadiness(c, pluginDir, []string{"https://example.invalid/p.tgz", "ghost"}); err != nil || *calls != 0 || *exports != 0 {
		t.Fatalf("no members: err=%v calls=%d exports=%d", err, *calls, *exports)
	}
}

// TestPluginInstallTrackFailsE140: a missing table, or a table Hasura does not
// list after the call, is E140 exit 2 naming the plugin and its tables.
func TestPluginInstallTrackFailsE140(t *testing.T) {
	compattest.Set(t, true)
	pluginDir := readinessProject(t, "fine", 1, 1)
	stubReadinessSeams(t, true)
	c, _ := readinessCmd()

	realTracking(t, func([]database.PluginTables) error {
		return &database.TrackError{Plugin: "fine", Table: "public.np_fine_items", Err: errors.New("plugin fine: table public.np_fine_items does not exist in Postgres")}
	}, fineMeta, nil)
	err := pluginInstallReadiness(c, pluginDir, []string{"fine"})
	d := errs.Describe(err)
	if d == nil || d.Code != "E140" || d.ExitCode != 2 {
		t.Fatalf("err = %v detail = %+v, want E140 exit 2", err, d)
	}
	if !strings.Contains(d.Message, "fine") || !strings.Contains(d.Message, "public.np_fine_items") || !strings.Contains(d.Cause, "does not exist in Postgres") {
		t.Errorf("message %q / cause %q must name the plugin, its tables and the cause", d.Message, d.Cause)
	}

	// The tracker succeeds but Hasura still lacks the table: verification fails.
	realTracking(t, nil, `{"sources":[{"tables":[]}]}`, nil)
	err = pluginInstallReadiness(c, pluginDir, []string{"fine"})
	if d := errs.Describe(err); d == nil || d.Code != "E140" || d.ExitCode != 2 || !strings.Contains(d.Message, "public.np_fine_items") {
		t.Fatalf("verification failure: err = %v detail = %+v", err, d)
	}
}

// exitCoder mimics an *exec.ExitError (docker) buried in the cause chain.
type exitCoder struct{}

func (exitCoder) Error() string { return "exit status 1" }
func (exitCoder) ExitCode() int { return 1 }

// TestPluginInstallTrackHasuraDownE140: Hasura unreachable or the admin call
// failing is E140 with the underlying error, exit 2 even when a docker exit
// status sits in the chain; it is never a silent skip.
func TestPluginInstallTrackHasuraDownE140(t *testing.T) {
	compattest.Set(t, true)
	pluginDir := readinessProject(t, "fine", 1, 1)
	stubReadinessSeams(t, true)
	c, _ := readinessCmd()

	realTracking(t, func([]database.PluginTables) error {
		return fmt.Errorf("track plugin tables: export metadata: hasura metadata request failed: dial tcp 127.0.0.1:8080: connection refused: %w", exitCoder{})
	}, "", nil)
	err := pluginInstallReadiness(c, pluginDir, []string{"fine"})
	d := errs.Describe(err)
	if d == nil || d.Code != "E140" || d.ExitCode != 2 {
		t.Fatalf("err = %v detail = %+v, want E140 exit 2", err, d)
	}
	if !strings.Contains(d.Message, "fine") || !strings.Contains(d.Cause, "connection refused") {
		t.Errorf("message %q / cause %q must name the plugin and carry the underlying error", d.Message, d.Cause)
	}
	if got := errs.ExitCodeFor(err); got != 2 {
		t.Errorf("exit = %d, want 2", got)
	}
	var coder exitCoder
	if !errors.As(err, &coder) {
		t.Error("the underlying error must stay reachable with errors.As")
	}

	// The verification read failing is E140 too.
	realTracking(t, nil, "", errors.New("hasura metadata API returned 403: access denied"))
	if d := errs.Describe(pluginInstallReadiness(c, pluginDir, []string{"fine"})); d == nil || d.Code != "E140" || !strings.Contains(d.Cause, "403") {
		t.Fatalf("export failure detail = %+v", d)
	}
}

// TestPluginInstallTrackingV14Unchanged: v1.4 sends no Hasura request and
// prints exactly what it printed before tracking existed.
func TestPluginInstallTrackingV14Unchanged(t *testing.T) {
	compattest.Set(t, false)
	pluginDir := readinessProject(t, "fine", 1, 1)
	stubReadinessSeams(t, true)
	c, withStderr := readinessCmd()
	if err := pluginInstallReadiness(c, pluginDir, []string{"fine"}); err != nil {
		t.Fatal(err)
	}
	baseline := withStderr.String()

	calls, exports := realTracking(t, nil, fineMeta, nil)
	c, withTracking := readinessCmd()
	if err := pluginInstallReadiness(c, pluginDir, []string{"fine"}); err != nil {
		t.Fatalf("v1.4 with tracking compiled in: %v", err)
	}
	if *calls != 0 || *exports != 0 {
		t.Fatalf("v1.4 sent Hasura requests: track=%d export=%d", *calls, *exports)
	}
	if withTracking.String() != baseline {
		t.Fatalf("v1.4 output changed: %q vs %q", withTracking.String(), baseline)
	}
}

// TestPluginInstallTrackSkipsWhenStackDown: v1.5 with the stack down tracks
// nothing and says so (readiness says the same about migrations).
func TestPluginInstallTrackSkipsWhenStackDown(t *testing.T) {
	compattest.Set(t, true)
	pluginDir := readinessProject(t, "fine", 1, 1)
	stubReadinessSeams(t, false)
	calls, exports := realTracking(t, nil, fineMeta, nil)
	c, stderr := readinessCmd()
	if err := pluginInstallReadiness(c, pluginDir, []string{"fine"}); err != nil {
		t.Fatal(err)
	}
	if *calls != 0 || *exports != 0 || !strings.Contains(stderr.String(), "not running") || !strings.Contains(stderr.String(), "rerun `nself plugin install fine`") {
		t.Fatalf("calls=%d exports=%d stderr=%q", *calls, *exports, stderr.String())
	}
}

// TestPluginInstallTrackDockerErrorIsE140: docker ps failing in the tracking
// step is not "stack down": it is E140 (exit 2) naming the plugin and the
// docker error, and no Hasura request is sent.
func TestPluginInstallTrackDockerErrorIsE140(t *testing.T) {
	compattest.Set(t, true)
	pluginDir := readinessProject(t, "fine", 1, 1)
	stubReadinessSeams(t, true)
	calls, exports := realTracking(t, nil, fineMeta, nil)
	pluginStackState = func(context.Context, string) (bool, error) {
		return false, errors.New("Cannot connect to the Docker daemon at unix:///var/run/docker.sock")
	}
	c, _ := readinessCmd()
	err := pluginInstallReadiness(c, pluginDir, []string{"fine"})
	d := errs.Describe(err)
	if d == nil || d.Code != "E140" || d.ExitCode != 2 {
		t.Fatalf("err = %v detail = %+v, want E140 exit 2", err, d)
	}
	if !strings.Contains(d.Message, "fine") || !strings.Contains(d.Message, "public.np_fine_items") || !strings.Contains(d.Cause, "Docker daemon") {
		t.Errorf("message %q / cause %q must name the plugin, its tables and the docker error", d.Message, d.Cause)
	}
	if *calls != 0 || *exports != 0 {
		t.Fatalf("Hasura requests sent: track=%d export=%d", *calls, *exports)
	}
}

// TestPluginInstallTrackSkipsNonBootMember: a member with tables but no
// migrations.apply: boot is never tracked (no Hasura request) and the install
// says so (kills a filter that only checks len(Tables) > 0).
func TestPluginInstallTrackSkipsNonBootMember(t *testing.T) {
	compattest.Set(t, true)
	pluginDir := readinessProject(t, "legacy", 1, 1)
	path := filepath.Join(pluginDir, "legacy", "plugin.json")
	raw, _ := os.ReadFile(path)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	delete(m, "migrations")
	out, _ := json.Marshal(m)
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
	stubReadinessSeams(t, true)
	calls, exports := realTracking(t, nil, fineMeta, nil)
	c, stderr := readinessCmd()
	if err := pluginInstallReadiness(c, pluginDir, []string{"legacy"}); err != nil {
		t.Fatal(err)
	}
	if *calls != 0 || *exports != 0 || !strings.Contains(stderr.String(), "legacy") || !strings.Contains(stderr.String(), "not tracked") {
		t.Fatalf("calls=%d exports=%d stderr=%q, want no Hasura request and a note naming legacy", *calls, *exports, stderr.String())
	}
}
