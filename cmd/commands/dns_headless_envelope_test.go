package commands

// Purpose: `nself config trust dns` under a headless run (NSELF_HEADLESS=1 or
// CI=1) skips the /etc/hosts write and warns; its v1.5 JSON envelope must say
// ok:false, not the default ok:true (D-0830). The exit code stays 0 because
// `nself start` calls the same code and relies on skip-and-continue.
// Inputs: the built binary (pilot harness) run against a throwaway project
// whose BASE_DOMAIN is not in /etc/hosts.
// Constraints: nothing is written to /etc/hosts (the headless skip happens
// before any write); HOME is a temp dir; no network.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDNSSetupHeadlessEnvelopeNotOK(t *testing.T) {
	proj := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	env := "PROJECT_NAME=smoke\nENV=dev\nBASE_DOMAIN=d0830-headless.test\nHASURA_GRAPHQL_JWT_SECRET=fixture\n"
	if err := os.WriteFile(filepath.Join(proj, ".env"), []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, headless := range []string{"NSELF_HEADLESS=1", "CI=1"} {
		res := runPilot(t, proj, true, []string{headless}, "config", "trust", "dns", "--json")
		if res.code != 0 {
			t.Fatalf("%s: exit %d, want 0 (skip-and-continue)\n%s", headless, res.code, res.stderr)
		}
		data := validateEnvelope(t, res.stdout, "config trust dns")
		if data["ok"] != false {
			t.Errorf("%s: envelope data.ok = %v, want false (the /etc/hosts write was skipped)\n%s", headless, data["ok"], res.stdout)
		}
	}
}
