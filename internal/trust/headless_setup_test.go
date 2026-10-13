// Package trust — headless_setup_test.go: the headless contract for Setup.
//
// With NSELF_HEADLESS=1 or CI=1 every step checks its state first and, when
// the state is missing, makes no system change and reports "skipped
// (headless)" instead of an error. These tests drive the orchestrator through
// its step seams and never execute a command.
package trust

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

// stubSteps replaces the step functions with ones that return the given error
// and restores them on cleanup.
func stubSteps(t *testing.T, err error) {
	t.Helper()
	origDNSD, origMk, origPD := setupDNSDarwinFunc, setupMkcertFunc, setupPortsDarwinFunc
	origDNSL, origPL := setupDNSLinuxFunc, setupPortsLinuxFunc
	t.Cleanup(func() {
		setupDNSDarwinFunc, setupMkcertFunc, setupPortsDarwinFunc = origDNSD, origMk, origPD
		setupDNSLinuxFunc, setupPortsLinuxFunc = origDNSL, origPL
	})
	setupDNSDarwinFunc = func(TrustConfig) (bool, bool, error) { return false, false, err }
	setupMkcertFunc = func(TrustConfig) (bool, error) { return false, err }
	setupPortsDarwinFunc = func(TrustConfig) (bool, error) { return false, err }
	setupDNSLinuxFunc = func(TrustConfig) (bool, error) { return false, err }
	setupPortsLinuxFunc = func(TrustConfig) (bool, error) { return false, err }
}

func TestSetup_SkippedStepsAreNotErrors(t *testing.T) {
	for _, goos := range []string{"darwin", "linux"} {
		t.Run(goos, func(t *testing.T) {
			hermetic(t)
			stubSteps(t, fmt.Errorf("step: %w", ErrSkippedHeadless))
			withFakeOS(t, goos, func() {
				res, err := Setup(TrustConfig{WorkDir: t.TempDir(), BaseDomain: "h.local"})
				if err != nil {
					t.Fatalf("Setup: %v", err)
				}
				if len(res.Errors) != 0 {
					t.Errorf("skipped steps must not be errors, got %v", res.Errors)
				}
				if want := []string{"dns", "ssl", "ports"}; !reflect.DeepEqual(res.Skipped, want) {
					t.Errorf("Skipped = %v, want %v", res.Skipped, want)
				}
				if res.DNSConfigured || res.CertsGenerated || res.PortsConfigured {
					t.Errorf("a skipped step must not report configured: %+v", res)
				}
			})
		})
	}
}

// Positive control: a real failure is still an error and is not mistaken for
// a headless skip.
func TestSetup_RealFailureIsStillAnError(t *testing.T) {
	hermetic(t)
	stubSteps(t, errors.New("brew exploded"))
	withFakeOS(t, "darwin", func() {
		res, err := Setup(TrustConfig{WorkDir: t.TempDir(), BaseDomain: "h.local"})
		if err != nil {
			t.Fatalf("Setup: %v", err)
		}
		if len(res.Errors) != 3 || len(res.Skipped) != 0 {
			t.Errorf("want 3 errors and 0 skipped, got errors=%v skipped=%v", res.Errors, res.Skipped)
		}
	})
}

func TestSetup_RecordsHeadlessMode(t *testing.T) {
	cases := []struct {
		name, headless, ci string
		want               bool
	}{
		{"interactive", "", "", false},
		{"nself_headless", "1", "", true},
		{"ci", "", "1", true},
		{"headless_off_beats_ci", "0", "1", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hermetic(t)
			t.Setenv("NSELF_HEADLESS", tc.headless)
			t.Setenv("CI", tc.ci)
			res, err := Setup(TrustConfig{WorkDir: t.TempDir(), SkipDNS: true, SkipSSL: true, SkipPorts: true})
			if err != nil {
				t.Fatalf("Setup: %v", err)
			}
			if res.Headless != tc.want {
				t.Errorf("Headless = %v, want %v", res.Headless, tc.want)
			}
		})
	}
}
