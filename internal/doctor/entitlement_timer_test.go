package doctor

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/maintenance"
)

func TestDoctorE132NoTimer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no timer backend on Windows")
	}
	oldW, oldS := timerWanted, timerStatus
	t.Cleanup(func() { timerWanted, timerStatus = oldW, oldS })
	for _, tc := range []struct {
		name   string
		wanted bool
		status maintenance.TimerStatus
		want   string // "" = no finding
	}{
		{"active", true, maintenance.TimerStatus{Active: true, Backend: "cron"}, ""},
		{"not installed", true, maintenance.TimerStatus{Backend: "cron", Reason: maintenance.ReasonNotInstalled}, "reason: not-installed"},
		{"unsafe path", true, maintenance.TimerStatus{Backend: "cron", Reason: maintenance.ReasonUnsafePath}, "reason: unsafe-path"},
		{"no backend", true, maintenance.TimerStatus{Reason: maintenance.ReasonNoBackend}, "reason: no-backend"},
		{"not wanted (free stack, opt-out, v1.4, custom plugin dir)", false, maintenance.TimerStatus{Reason: maintenance.ReasonNotInstalled}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			timerWanted = func() bool { return tc.wanted }
			timerStatus = func(string) maintenance.TimerStatus { return tc.status }
			got := EntitlementTimerChecks(context.Background(), t.TempDir())
			if tc.want == "" {
				if len(got) != 0 {
					t.Fatalf("unexpected finding %+v", got)
				}
				return
			}
			if len(got) != 1 || got[0].Status != "warn" || !strings.Contains(got[0].Message, "E132") || !strings.Contains(got[0].Message, tc.want) {
				t.Fatalf("finding = %+v, want E132 %s", got, tc.want)
			}
			if e := errs.Registry["E132"]; e.Exit != 1 {
				t.Fatalf("E132 registry entry = %+v", e)
			}
		})
	}
}

// TestDoctorE132Wanted checks the real predicate: v1.5, a licensed plugin in the default
// plugin directory, autorefresh not disabled.
func TestDoctorE132Wanted(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("NSELF_PLUGIN_DIR", "")
	t.Setenv("NSELF_ENTITLEMENT_AUTOREFRESH", "")
	dir := filepath.Join(home, ".nself", "plugins", "fixture")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(`{"license":"licensed"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	compattest.Both(t, func(t *testing.T) {
		if got := timerWanted(); got != compat.V15() {
			t.Fatalf("timerWanted = %v with V15 = %v", got, compat.V15())
		}
		if compat.V15() {
			t.Setenv("NSELF_ENTITLEMENT_AUTOREFRESH", "false")
			if timerWanted() {
				t.Fatal("opt-out still wants a timer")
			}
			t.Setenv("NSELF_ENTITLEMENT_AUTOREFRESH", "")
			t.Setenv("NSELF_PLUGIN_DIR", t.TempDir())
			if timerWanted() {
				t.Fatal("custom plugin directory still wants a timer")
			}
		}
	})
}

// TestDeepChecksRegistersE132 guards the registration next to the E126 check: DeepChecks needs
// Docker and a live stack, so the call site itself is asserted.
func TestDeepChecksRegistersE132(t *testing.T) {
	src, err := os.ReadFile("deep.go")
	if err != nil {
		t.Fatal(err)
	}
	e126, e132 := strings.Index(string(src), "EntitlementFileChecks(ctx, projectDir)"), strings.Index(string(src), "EntitlementTimerChecks(ctx, projectDir)")
	if e126 < 0 || e132 < e126 || e132-e126 > 200 {
		t.Fatalf("EntitlementTimerChecks must be registered right after EntitlementFileChecks in deep.go (%d, %d)", e126, e132)
	}
}
