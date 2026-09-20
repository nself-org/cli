package docker

// Purpose: pin the decision that `nself doctor` got wrong — a port held by the
//   project's OWN compose stack is not a conflict, while a port held by a
//   foreign process still is.
// Inputs:  injected probe/holder functions, plus one real OS listener.
// Outputs: assertions on filterPortConflicts and ProjectPortReport.
// Constraints: no Docker daemon, no network beyond a loopback listener.
//
// The regression: doctor probed the default port list with no ownership filter,
// so a healthy stack reported its own nginx/postgres/hasura/auth containers as
// conflicts on 80/443/5432/8080/4000 — "Doctor found 6 warning(s)" on a green
// E2E golden-path run. Silencing the check would have traded false positives
// for false negatives, so both directions are asserted here.

import (
	"errors"
	"fmt"
	"net"
	"testing"
)

// fakeProbe reports the given ports as in use and everything else as free.
func fakeProbe(inUse ...int) func(int) (bool, error) {
	set := make(map[int]bool, len(inUse))
	for _, p := range inUse {
		set[p] = true
	}
	return func(p int) (bool, error) { return set[p], nil }
}

func noHolder(int) string { return "" }

func portsOf(conflicts []PortConflict) []int {
	out := make([]int, 0, len(conflicts))
	for _, c := range conflicts {
		out = append(out, c.Port)
	}
	return out
}

// TestFilterPortConflicts_OwnPortsAreNotConflicts is the regression. Every one
// of these ports is in use, and every one is bound by this project.
func TestFilterPortConflicts_OwnPortsAreNotConflicts(t *testing.T) {
	t.Parallel()

	stack := []int{80, 443, 5432, 8080, 4000, 3021}
	owned := map[int]bool{}
	for _, p := range stack {
		owned[p] = true
	}

	got, err := filterPortConflicts(stack, owned, fakeProbe(stack...), noHolder)
	if err != nil {
		t.Fatalf("filterPortConflicts: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("a healthy stack must report zero port conflicts; got %v", portsOf(got))
	}
}

// TestFilterPortConflicts_ForeignPortStillWarns is the other direction. The
// fix must not be a silenced check.
func TestFilterPortConflicts_ForeignPortStillWarns(t *testing.T) {
	t.Parallel()

	// 5432 is ours; 8080 is somebody else's.
	owned := map[int]bool{5432: true}

	got, err := filterPortConflicts([]int{5432, 8080}, owned, fakeProbe(5432, 8080), noHolder)
	if err != nil {
		t.Fatalf("filterPortConflicts: %v", err)
	}
	if want := []int{8080}; len(got) != 1 || got[0].Port != want[0] {
		t.Fatalf("expected the foreign port %v to be reported, got %v", want, portsOf(got))
	}
}

// TestFilterPortConflicts_RealListenerIsCaught uses an actual OS listener, so
// the probe itself is exercised rather than a stub of it.
func TestFilterPortConflicts_RealListenerIsCaught(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	port := ln.Addr().(*net.TCPAddr).Port

	// Unowned: a genuine external listener must be reported.
	got, err := filterPortConflicts([]int{port}, nil, CheckPort, noHolder)
	if err != nil {
		t.Fatalf("filterPortConflicts: %v", err)
	}
	if len(got) != 1 || got[0].Port != port {
		t.Fatalf("a real listener on %d must be reported as a conflict; got %v", port, portsOf(got))
	}

	// Same listener, now attributed to this project: not a conflict.
	got, err = filterPortConflicts([]int{port}, map[int]bool{port: true}, CheckPort, noHolder)
	if err != nil {
		t.Fatalf("filterPortConflicts: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("port %d is owned by this project and must not be a conflict; got %v", port, portsOf(got))
	}
}

// TestFilterPortConflicts_SkipsDockerInternalHoldersOnly checks that the
// transient-holder exemption is narrow: Docker Desktop's own processes are
// skipped, an unidentifiable holder is not.
func TestFilterPortConflicts_SkipsDockerInternalHoldersOnly(t *testing.T) {
	t.Parallel()

	holders := map[int]string{
		5432: "com.docker.backend",
		8080: "", // could not identify — must NOT be silently skipped
		4000: "node",
	}
	got, err := filterPortConflicts([]int{5432, 8080, 4000}, nil,
		fakeProbe(5432, 8080, 4000),
		func(p int) string { return holders[p] })
	if err != nil {
		t.Fatalf("filterPortConflicts: %v", err)
	}
	if len(got) != 2 || got[0].Port != 8080 || got[1].Port != 4000 {
		t.Fatalf("want [8080 4000] reported and the docker-internal hold skipped; got %v", portsOf(got))
	}
}

// TestFilterPortConflicts_ProbeErrorPropagates — a probe that cannot answer
// must not be read as "free".
func TestFilterPortConflicts_ProbeErrorPropagates(t *testing.T) {
	t.Parallel()

	boom := errors.New("probe failed")
	_, err := filterPortConflicts([]int{5432}, nil,
		func(int) (bool, error) { return false, boom }, noHolder)
	if !errors.Is(err, boom) {
		t.Fatalf("expected the probe error to propagate, got %v", err)
	}
}

func TestCheckPortsUnowned_ReportsRealListener(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	port := ln.Addr().(*net.TCPAddr).Port

	got, err := CheckPortsUnowned([]int{port})
	if err != nil {
		t.Fatalf("CheckPortsUnowned: %v", err)
	}
	if len(got) != 1 || got[0].Port != port {
		t.Fatalf("want port %d reported, got %v", port, portsOf(got))
	}
}

// TestProjectPortConflicts_ReportsOwnershipFailureWithoutConflicts — when the
// ownership query fails, the report must carry the failure and NOT a list of
// conflicts it could not verify. Reporting them is exactly the false-positive
// behaviour being removed.
func TestProjectPortConflicts_ReportsOwnershipFailureWithoutConflicts(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rep := ProjectPortConflicts(t.Context(), dir, nil, fmt.Sprintf("%s/docker-compose.yml", dir))

	if !rep.OwnershipUnknown() {
		t.Fatalf("expected ownership to be unknown in an empty dir; got err=%v", rep.OwnershipErr)
	}
	if len(rep.Conflicts) != 0 {
		t.Errorf("must not report conflicts it cannot attribute; got %v", portsOf(rep.Conflicts))
	}
	if !rep.UsedFallbackPorts() {
		t.Errorf("compose config is unreadable here, so the default port list must be the fallback")
	}
	if len(rep.Ports) == 0 {
		t.Errorf("a port check with an empty list cannot fail — the fallback list must be populated")
	}
}

func TestProjectPortReport_ServiceNameFallsBackToUnknown(t *testing.T) {
	t.Parallel()

	rep := ProjectPortReport{ServiceNames: map[int]string{5432: "postgres"}}
	if got := rep.ServiceName(5432); got != "postgres" {
		t.Errorf("ServiceName(5432) = %q, want postgres", got)
	}
	if got := rep.ServiceName(9999); got != "unknown service" {
		t.Errorf("ServiceName(9999) = %q, want \"unknown service\"", got)
	}
}
