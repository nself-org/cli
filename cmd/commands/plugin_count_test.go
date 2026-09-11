package commands

// Purpose: verify `nself plugin count` is registered with the expected
// flags, and that its human and --json output both surface the advertised
// headline number from the embedded internal/plugin/count artifact.
// Constraints: no network, no sibling registry checkouts — the command
// reads only the embedded artifact.

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/plugin/count"
)

// newPluginCountTestCmd returns an isolated invocation of pluginCountCmd
// with stdout captured, mirroring newOutdatedTestCmd in
// plugin_outdated_test.go.
func newPluginCountTestCmd(t *testing.T, jsonOut bool) (run func() error, stdout func() string) {
	t.Helper()
	cmd := pluginCountCmd
	if err := cmd.Flags().Set("json", boolStr(jsonOut)); err != nil {
		t.Fatalf("setting --json: %v", err)
	}
	t.Cleanup(func() {
		cmd.Flags().Set("json", "false") //nolint:errcheck
	})
	cmd.SetContext(context.Background())

	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	run = func() error {
		err := runPluginCount(cmd, nil)
		_ = w.Close()
		os.Stdout = origStdout
		return err
	}
	stdout = func() string {
		var buf strings.Builder
		tmp := make([]byte, 8192)
		n, _ := r.Read(tmp)
		buf.Write(tmp[:n])
		return buf.String()
	}
	return run, stdout
}

// TestPluginCountRegistered verifies the count subcommand is wired up
// under `plugin` with the expected flag and RunE handler.
func TestPluginCountRegistered(t *testing.T) {
	sub := assertCmd(t, pluginCmd, "count")
	if sub.RunE == nil {
		t.Error("plugin count: missing RunE handler")
	}
	if !hasBoolFlag(sub, "json") {
		t.Error("plugin count: missing --json flag")
	}
}

// TestPluginCount_HumanOutputShowsAdvertised verifies the default table
// output surfaces the advertised number as the headline, matching the
// embedded artifact.
func TestPluginCount_HumanOutputShowsAdvertised(t *testing.T) {
	a, err := count.Load()
	if err != nil {
		t.Fatalf("count.Load() error = %v", err)
	}

	run, stdout := newPluginCountTestCmd(t, false)
	if err := run(); err != nil {
		t.Fatalf("runPluginCount error = %v", err)
	}

	out := stdout()
	wantLine := "Advertised: " + strconv.Itoa(a.Advertised)
	if !strings.Contains(out, wantLine) {
		t.Errorf("expected output to contain %q, got: %q", wantLine, out)
	}
	if !strings.Contains(out, "de-duplicated") {
		t.Errorf("expected output to explain the advertised number, got: %q", out)
	}
}

// TestPluginCount_JSONOutputMatchesEmbeddedArtifact verifies --json prints
// the embedded artifact verbatim (parseable, same advertised value as
// count.Load()).
func TestPluginCount_JSONOutputMatchesEmbeddedArtifact(t *testing.T) {
	run, stdout := newPluginCountTestCmd(t, true)
	if err := run(); err != nil {
		t.Fatalf("runPluginCount --json error = %v", err)
	}

	var a count.Artifact
	if jsonErr := json.Unmarshal([]byte(stdout()), &a); jsonErr != nil {
		t.Fatalf("--json output did not parse as JSON: %v\noutput: %q", jsonErr, stdout())
	}

	loaded, err := count.Load()
	if err != nil {
		t.Fatalf("count.Load() error = %v", err)
	}
	if a.Advertised != loaded.Advertised {
		t.Errorf("--json advertised = %d, count.Load() advertised = %d, want equal", a.Advertised, loaded.Advertised)
	}
}
