package commands

// Purpose: assert that `nself doctor` and `nself start` reach the SAME verdict
//   about the same port on the same machine, in both directions.
// Inputs:  a temp project dir plus a substituted projectPortConflicts.
// Outputs: assertions on doctor's check results and start's returned error.
// Constraints: no Docker daemon — the shared resolve step is faked.
//
// The regression: doctor probed a default port list with no ownership filter
// while start filtered by ownership, so a healthy stack produced six doctor
// warnings (80/443/5432/8080/4000 plus admin 3021) naming its own containers
// while start reported none. Those six were part of "Doctor found 6
// warning(s)" in E2E golden-path run 35530146490.

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/docker"
)

// newProjectDir creates a directory that looks like a built project, so
// projectPortInputs reports hasProject and the shared step is consulted.
func newProjectDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatalf("writing compose file: %v", err)
	}
	return dir
}

// listeningPort opens a real loopback listener and returns its port. Tests use
// a genuinely occupied port so that a doctor which stopped consulting the
// shared ownership filter would actually fail them — a stub port that nothing
// holds would pass either way, on any machine.
func listeningPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	return ln.Addr().(*net.TCPAddr).Port
}

// stubPortReport substitutes the shared resolve-and-probe step for the test.
func stubPortReport(t *testing.T, rep docker.ProjectPortReport) {
	t.Helper()
	prev := projectPortConflicts
	projectPortConflicts = func(context.Context, string, []string, ...string) docker.ProjectPortReport {
		return rep
	}
	t.Cleanup(func() { projectPortConflicts = prev })
}

func warnCount(results []doctorCheckResult) int {
	n := 0
	for _, r := range results {
		if r.Status == "warn" {
			n++
		}
	}
	return n
}

// TestDoctorPorts_HealthyStackReportsNoConflicts is the regression: the stack's
// own ports are in use, owned by the project, and must produce zero warnings.
func TestDoctorPorts_HealthyStackReportsNoConflicts(t *testing.T) {
	// A port that really is occupied, and really is ours.
	own := listeningPort(t)
	stubPortReport(t, docker.ProjectPortReport{
		Ports:        []int{own},
		ServiceNames: map[int]string{own: "postgres"},
		// The shared step already excluded every port this project owns.
		Conflicts: nil,
	})

	results := checkPorts(context.Background(), newProjectDir(t), false)

	if got := warnCount(results); got != 0 {
		t.Fatalf("a healthy stack must produce 0 port warnings, got %d: %v", got, results)
	}
	if len(results) != 1 || results[0].Status != "pass" {
		t.Fatalf("expected a single pass result, got %v", results)
	}
}

// TestDoctorPorts_ForeignListenerStillWarns — the fix must not be a silenced
// check.
func TestDoctorPorts_ForeignListenerStillWarns(t *testing.T) {
	own := listeningPort(t)
	foreign := listeningPort(t)
	stubPortReport(t, docker.ProjectPortReport{
		Ports:        []int{own, foreign},
		ServiceNames: map[int]string{own: "postgres", foreign: "hasura"},
		Conflicts:    []docker.PortConflict{{Port: foreign, InUse: true}},
	})

	results := checkPorts(context.Background(), newProjectDir(t), false)

	if got := warnCount(results); got != 1 {
		t.Fatalf("a foreign listener must still warn; got %d warnings: %v", got, results)
	}
	if !strings.Contains(results[0].Name, strconv.Itoa(foreign)) {
		t.Errorf("the warning must name the conflicting port %d, got %q", foreign, results[0].Name)
	}
}

// TestDoctorPorts_OwnershipUnknownWarnsOnceNotPerPort — when ownership cannot
// be established, doctor must say so once rather than emit a conflict per port
// it cannot attribute.
func TestDoctorPorts_OwnershipUnknownWarnsOnceNotPerPort(t *testing.T) {
	stubPortReport(t, docker.ProjectPortReport{
		Ports:        docker.ReservedPorts,
		ServiceNames: docker.DefaultPortServiceNames(),
		OwnershipErr: context.DeadlineExceeded,
	})

	results := checkPorts(context.Background(), newProjectDir(t), false)

	if len(results) != 1 {
		t.Fatalf("want exactly one result when ownership is unknown, got %d: %v", len(results), results)
	}
	if results[0].Status != "warn" || !strings.Contains(results[0].Message, "skipped") {
		t.Errorf("want a single skipped-check warning, got %+v", results[0])
	}
}

// TestDoctorAndStartAgreeOnTheSamePort is the cross-command guard. Doctor
// warning about a port that start is happy to bind (or vice versa) is the bug
// class this whole change exists to remove.
func TestDoctorAndStartAgreeOnTheSamePort(t *testing.T) {
	// One real listener, examined twice: once attributed to this project and
	// once to a foreign process. Both commands must agree each time.
	port := listeningPort(t)

	cases := []struct {
		name      string
		owned     bool
		wantIssue bool
	}{
		{name: "port held by this project's own container", owned: true, wantIssue: false},
		{name: "port held by a foreign process", owned: false, wantIssue: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := newProjectDir(t)
			var conflicts []docker.PortConflict
			if !tc.owned {
				conflicts = []docker.PortConflict{{Port: port, InUse: true}}
			}
			stubPortReport(t, docker.ProjectPortReport{
				Ports:        []int{port},
				ServiceNames: map[int]string{port: "hasura"},
				Conflicts:    conflicts,
			})

			doctorIssue := warnCount(checkPorts(context.Background(), dir, false)) > 0
			startIssue := checkStartPorts(context.Background(), startOpts{}, dir, nil) != nil

			if doctorIssue != startIssue {
				t.Fatalf("doctor and start disagree about port %d: doctor warns=%v, start fails=%v",
					port, doctorIssue, startIssue)
			}
			if doctorIssue != tc.wantIssue {
				t.Fatalf("want issue=%v for %q, got %v", tc.wantIssue, tc.name, doctorIssue)
			}
		})
	}
}

// TestProjectPortInputs_NoProjectDirectory — ReadComposeManifest succeeds even
// with no manifest (it returns the default filename), so hasProject must come
// from the filesystem. Getting this wrong would suppress doctor's port check
// in any directory that was never built.
func TestProjectPortInputs_NoProjectDirectory(t *testing.T) {
	t.Parallel()

	_, _, hasProject := projectPortInputs(t.TempDir())
	if hasProject {
		t.Fatal("an empty directory is not a built project")
	}

	_, _, hasProject = projectPortInputs(newProjectDir(t))
	if !hasProject {
		t.Fatal("a directory containing docker-compose.yml is a built project")
	}
}
