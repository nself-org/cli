package controlplane

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// TestRunWithApplyShippingProbes exercises real SSH probes before an env-artifact
// apply: unknown or changing keys abort the run, and pinned keys use strict SSH.
func TestRunWithApplyShippingProbes(t *testing.T) {
	for _, tc := range []struct {
		name       string
		pinned     bool
		changeScan string
	}{
		{name: "unknown host"},
		{name: "reachability key change", pinned: true, changeScan: "1"},
		{name: "docker key change", pinned: true, changeScan: "2"},
		{name: "pinned host", pinned: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inv, compose, _, bin, host := shippingRunFixture(t)
			t.Setenv("NSELF_V15", "1")
			if tc.pinned {
				fp, err := remote.Fingerprint("host.example ssh-ed25519 AQID")
				if err != nil {
					t.Fatal(err)
				}
				if err := TrustHostKey(context.Background(), host, fp); err != nil {
					t.Fatal(err)
				}
			}
			if tc.changeScan != "" {
				count := filepath.Join(bin, "scans")
				script := "#!/bin/sh\necho x >> '" + count + "'\nif [ \"$(wc -l < '" + count + "')\" -le " + tc.changeScan + " ]; then k=AQID; else k=AQIE; fi\nprintf 'host.example ssh-ed25519 %s\\n' \"$k\"\n"
				if err := os.WriteFile(filepath.Join(bin, "ssh-keyscan"), []byte(script), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			opened := false
			ra := &RemoteApply{
				Set: deploy.ArtifactSet{RemotePath: "/opt/nself", EnvFiles: []string{".env.qa"}},
				Open: func(ctx context.Context, _ Server, _, _ string) (deploy.Remote, error) {
					opened = true
					if !isShippingRun(ctx) {
						t.Error("apply context is not marked as shipping")
					}
					return nil, errors.New("stop before artifact copy")
				},
			}
			_, err := RunWithApply(context.Background(), inv, "qa", NewSSHProber(filepath.Dir(compose), false), compose, ra)
			calls, _ := os.ReadFile(filepath.Join(bin, "calls.log"))
			if !tc.pinned || tc.changeScan != "" {
				if err == nil || !strings.Contains(err.Error(), "E487") || opened {
					t.Fatalf("want E487 before apply, got %v (opened=%v)", err, opened)
				}
				if !tc.pinned && len(calls) != 0 {
					t.Fatalf("connected before key verification:\n%s", calls)
				}
			} else if err != nil || !opened {
				t.Fatalf("pinned host did not reach apply: %v (opened=%v)", err, opened)
			}
			for _, line := range strings.Split(strings.TrimSpace(string(calls)), "\n") {
				if line != "" && (strings.Contains(line, "accept-new") || !strings.Contains(line, "StrictHostKeyChecking=yes") || !strings.Contains(line, "ControlMaster=no") || !strings.Contains(line, "ControlPath=none")) {
					t.Errorf("probe connection is not strict: %s", line)
				}
			}
		})
	}
}
