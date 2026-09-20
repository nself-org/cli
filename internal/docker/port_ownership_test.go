package docker

// Purpose: pin the two behaviours that made `nself start` refuse to start over
// its own running containers.
//
// Inputs:  a workdir whose compose project cannot be queried; env file lists.
// Outputs: assertions on the returned error and on the compose args.
// Constraints: must not require a running Docker daemon.
//
// The failure being guarded: installing a plugin drops a
// docker-compose.plugin.yml that refers to ${DOCKER_NETWORK}. The port check
// invoked docker compose WITHOUT --env-file, so that resolved to empty, docker
// rejected the project ("service ai refers to undefined network : invalid
// compose project"), the ps query failed, and OwnedHostPorts reported an empty
// set. Every port the project itself already bound then read as a foreign
// conflict and start aborted with "port conflicts detected: 6 port(s) in use".

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestOwnedHostPorts_ReturnsErrorInsteadOfClaimingNothingIsOwned(t *testing.T) {
	// A workdir with no compose file at all: the ps query cannot succeed.
	// The old code answered (empty set, nil error), which reads as "this
	// project owns no ports" — a confident wrong answer that turns every one
	// of its own ports into a conflict.
	dir := t.TempDir()

	owned, err := OwnedHostPorts(context.Background(), dir, nil,
		filepath.Join(dir, "docker-compose.yml"))

	if err == nil {
		t.Fatalf("expected an error when the compose query fails, got owned=%v err=nil", owned)
	}
	if owned != nil {
		t.Errorf("on failure the owned set must be nil, not an empty map that reads as \"owns nothing\"; got %v", owned)
	}
}

func TestCheckAllPortsFiltered_PropagatesOwnershipFailure(t *testing.T) {
	// Degrading to unfiltered behaviour here is what blocked a legitimate
	// start. The caller must be told it could not determine ownership so it
	// can warn and skip, rather than report false conflicts.
	dir := t.TempDir()

	conflicts, err := CheckAllPortsFiltered(context.Background(), []int{5432}, dir, nil,
		filepath.Join(dir, "docker-compose.yml"))

	if err == nil {
		t.Fatalf("expected the ownership failure to propagate, got conflicts=%v err=nil", conflicts)
	}
	if conflicts != nil {
		t.Errorf("must not report conflicts it cannot verify, got %v", conflicts)
	}
}

func TestComposeArgs_IncludeEveryEnvFile(t *testing.T) {
	// --env-file is what lets a plugin compose file resolve ${DOCKER_NETWORK}.
	// Dropping it is the root cause, so assert the flag is emitted per file
	// and in order.
	c := NewCompose("docker-compose.yml", "plugin.yml")
	c.EnvFiles = []string{".env", ".env.computed"}

	got := strings.Join(c.buildBaseArgs(), " ")

	for _, want := range []string{
		"--env-file .env",
		"--env-file .env.computed",
		"-f docker-compose.yml",
		"-f plugin.yml",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("compose args missing %q; got: %s", want, got)
		}
	}
}

func TestComposeArgs_OmitEnvFileFlagWhenNoneSet(t *testing.T) {
	c := NewCompose("docker-compose.yml")

	if got := strings.Join(c.buildBaseArgs(), " "); strings.Contains(got, "--env-file") {
		t.Errorf("no env files set, so no --env-file flag should appear; got: %s", got)
	}
}
