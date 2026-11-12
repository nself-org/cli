package commands

// Wire-21: a single-host remote deploy, driven through the cobra root by argv,
// checks health on the deploy host over the deploy transport and installs no
// systemd or launchd unit on the operator machine.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"
)

const wire21Host = "u@sim.example.test"

// wire21Deploy runs `nself deploy qa` through RootCmd against the sim host the
// push fixture puts on PATH (ssh and rsync log their argv). doctorFails makes
// the host's `nself doctor` exit 1 naming postgres. The build is stubbed: it
// needs docker and is not part of the seam under test.
func wire21Deploy(t *testing.T, doctorFails bool) (log, bin, stderr string, err error) {
	t.Helper()
	_, log, bin, _ = pushFixture(t)
	if doctorFails {
		ssh := "#!/bin/sh\necho \"ssh $*\" >> '" + log + "'\ncase \"$*\" in *'nself doctor'*) echo 'postgres unhealthy'; exit 1;; esac\nexit 0\n"
		if werr := os.WriteFile(filepath.Join(bin, "ssh"), []byte(ssh), 0o700); werr != nil {
			t.Fatal(werr)
		}
	}
	// Probes for a local unit install: any of these on PATH logs its call.
	probe := filepath.Join(bin, "local-unit.log")
	for _, n := range []string{"systemctl", "launchctl", "sudo"} {
		if werr := os.WriteFile(filepath.Join(bin, n), []byte("#!/bin/sh\necho \""+n+" $*\" >> '"+probe+"'\n"), 0o700); werr != nil {
			t.Fatal(werr)
		}
	}
	pinFakeHostKeys(t, bin, wire21Host)
	t.Setenv("NSELF_DEPLOY_HOST_QA", wire21Host)
	ob := deployBuildStepFn
	deployBuildStepFn = func(_ context.Context, _ string, _ bool, s []deployStep) ([]deployStep, error) { return s, nil }
	reset := func() {
		deployCmd.Flags().VisitAll(func(f *pflag.Flag) { _ = f.Value.Set(f.DefValue); f.Changed = false })
		RootCmd.SetArgs(nil)
	}
	t.Cleanup(func() { deployBuildStepFn = ob; reset() })
	reset()
	RootCmd.SetArgs([]string{"deploy", "qa"})
	_, stderr = captureStd(t, func() { err = RootCmd.Execute() })
	return log, bin, stderr, err
}

// TestWire21RemoteHealthOnTarget: the health gate is `nself doctor` on the sim
// host, in the pushed project dir, over the strict deploy transport; a host
// whose doctor fails fails the deploy and names the service.
func TestWire21RemoteHealthOnTarget(t *testing.T) {
	t.Run("healthy host", func(t *testing.T) {
		log, _, stderr, err := wire21Deploy(t, false)
		if err != nil {
			t.Fatalf("deploy qa: %v\n%s", err, stderr)
		}
		b, _ := os.ReadFile(log)
		var doctor []string
		for _, l := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(l, "ssh ") && strings.Contains(l, "nself doctor") {
				doctor = append(doctor, l)
			}
		}
		if len(doctor) != 1 {
			t.Fatalf("want exactly one nself doctor on the host, got %d:\n%s", len(doctor), b)
		}
		d := doctor[0]
		if !strings.Contains(d, "sim.example.test") || !strings.Contains(d, "cd '/opt/nself' && nself doctor") {
			t.Errorf("doctor did not run in the project dir on the sim host: %s", d)
		}
		if !strings.Contains(d, "StrictHostKeyChecking=yes") {
			t.Errorf("health call is not on the strict deploy transport: %s", d)
		}
	})
	t.Run("unhealthy host fails the deploy", func(t *testing.T) {
		_, _, stderr, err := wire21Deploy(t, true)
		if err == nil || !strings.Contains(err.Error(), "health check on") || !strings.Contains(err.Error(), "postgres") {
			t.Fatalf("want the host health failure naming postgres, got %v\n%s", err, stderr)
		}
	})
}

// TestWire21NoLocalTimerInstalled: the same deploy calls no systemctl,
// launchctl or sudo on this machine, attempts no timer install, and leaves the
// disk-cleanup unit paths exactly as they were.
func TestWire21NoLocalTimerInstalled(t *testing.T) {
	units := []string{"/etc/systemd/system/nself-disk-cleanup.service", "/etc/systemd/system/nself-disk-cleanup.timer", "/Library/LaunchDaemons/org.nself.disk-cleanup.plist"}
	before := map[string]time.Time{}
	for _, u := range units {
		if fi, err := os.Stat(u); err == nil {
			before[u] = fi.ModTime()
		}
	}
	_, bin, stderr, err := wire21Deploy(t, false)
	if err != nil {
		t.Fatalf("deploy qa: %v\n%s", err, stderr)
	}
	if b, rerr := os.ReadFile(filepath.Join(bin, "local-unit.log")); rerr == nil {
		t.Errorf("deploy touched the local service manager:\n%s", b)
	}
	if strings.Contains(stderr, "maintenance timer") {
		t.Errorf("deploy attempted a local timer install:\n%s", stderr)
	}
	for _, u := range units {
		fi, serr := os.Stat(u)
		was, existed := before[u]
		switch {
		case serr == nil && !existed:
			t.Errorf("deploy created %s on the operator machine", u)
		case serr == nil && !fi.ModTime().Equal(was):
			t.Errorf("deploy rewrote %s on the operator machine", u)
		}
	}
}
