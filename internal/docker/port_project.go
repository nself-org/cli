package docker

// Purpose: one resolution of "which host ports does this project care about,
//   and which of them are actually a problem", shared by `nself start`'s
//   pre-flight and `nself doctor`'s port checks.
// Inputs:  workdir, the compose env files, and the compose files the project
//   was built with.
// Outputs: a ProjectPortReport describing the ports checked, their service
//   names, the foreign conflicts found, and which sub-steps degraded.
// Constraints: no side effects; `docker compose config`/`ps` start nothing.
//
// This exists because doctor and start answered the same question differently.
// Doctor probed the default port list with no ownership filter at all, so on a
// healthy stack it reported the project's OWN nginx, postgres, hasura, auth and
// admin containers as conflicts — six warnings on a green run. False positives
// at that volume train people to ignore doctor output entirely, so the two
// commands now share this single path.
// SPORT: CLI-CMD-DOCTOR-001

import "context"

// ProjectPortReport is the outcome of resolving and probing a project's ports.
//
// The degraded cases are reported rather than folded into the happy path:
// "this port is free" and "I could not tell" are different answers, and
// collapsing them is what produced the false conflicts in the first place.
type ProjectPortReport struct {
	// Ports is the list actually probed: the ports the compose config
	// publishes, or ReservedPorts when those could not be read.
	Ports []int
	// ServiceNames maps a probed port to the service that publishes it.
	ServiceNames map[int]string
	// Conflicts holds ports in use by a process outside this project.
	// Meaningless when OwnershipErr is set.
	Conflicts []PortConflict
	// DeclaredErr is set when the published-port list could not be read from
	// compose and Ports fell back to the defaults.
	DeclaredErr error
	// OwnershipErr is set when the project's own bound ports could not be
	// determined. Conflicts is nil in that case: reporting unverified
	// conflicts is worse than reporting none.
	OwnershipErr error
}

// UsedFallbackPorts reports whether Ports is the default list rather than the
// ports this stack actually publishes.
func (r ProjectPortReport) UsedFallbackPorts() bool { return r.DeclaredErr != nil }

// OwnershipUnknown reports whether the ownership filter could not run, in
// which case Conflicts must not be presented to the user as conflicts.
func (r ProjectPortReport) OwnershipUnknown() bool { return r.OwnershipErr != nil }

// ServiceName returns a human name for a probed port, or "unknown service".
func (r ProjectPortReport) ServiceName(port int) string {
	if n := r.ServiceNames[port]; n != "" {
		return n
	}
	return "unknown service"
}

// ProjectPortConflicts resolves the host ports this project publishes and
// probes them, excluding any port the project's own running containers already
// hold. Errors are carried on the report instead of returned so each caller can
// choose how to degrade: start warns and skips the check, doctor warns once
// rather than emitting one false conflict per port.
func ProjectPortConflicts(ctx context.Context, workdir string, envFiles []string, composeFiles ...string) ProjectPortReport {
	var rep ProjectPortReport

	checkPorts, names, derr := DeclaredHostPorts(ctx, workdir, envFiles, composeFiles...)
	if derr != nil || len(checkPorts) == 0 {
		// Fall back to the default list rather than check nothing. Checking an
		// empty list would be a port check that cannot fail, which is worse
		// than one that is occasionally too strict.
		rep.DeclaredErr = derr
		checkPorts = ReservedPorts
		names = DefaultPortServiceNames()
	}
	rep.Ports = checkPorts
	rep.ServiceNames = names

	conflicts, err := CheckAllPortsFiltered(ctx, checkPorts, workdir, envFiles, composeFiles...)
	if err != nil {
		rep.OwnershipErr = err
		return rep
	}
	rep.Conflicts = conflicts
	return rep
}
