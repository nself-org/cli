package docker

// compose_states_regress.go — the post-gate recheck of a service's containers.
//
// Purpose: after every gate has passed, tell whether a container that passed
// has since crashed, been replaced, vanished or gone unhealthy.
// Constraints: pure function over two state snapshots; no docker calls.

import "fmt"

// Regressed compares the containers a service's gate saw (before) with a later
// read (after). It returns ErrServiceCrashed, with the container named, when:
//   - an after container is restarting, exited non-zero, or is not in before (a
//     replica replaced or added after the gate);
//   - a container that was running at its gate started again (a new StartedAt)
//     or is now paused or dead;
//   - Docker reports any after container unhealthy, whatever the gate waited
//     for, or a container that was healthy at its gate is no longer running and
//     healthy;
//   - any before container, in any state, is missing from after.
//
// Containers match by ID, or by Name when the ID is empty (old compose). Rows
// without a StartedAt cannot be compared by start time and are judged on state,
// identity and health. Exited 0 for a container that was never healthy is a
// finished run, not a crash; running/starting staying running/starting is fine.
func Regressed(before, after []ContainerState) error {
	key := func(c ContainerState) string {
		if c.ID != "" {
			return c.ID
		}
		return c.Name
	}
	seen := map[string]ContainerState{}
	for _, b := range before {
		seen[key(b)] = b
	}
	present := map[string]bool{}
	for _, a := range after {
		present[key(a)] = true
		b, known := seen[key(a)]
		switch {
		case a.State == "restarting":
			return fmt.Errorf("%w (%s is restarting after its gate passed)", ErrServiceCrashed, a.Name)
		case a.State == "exited" && a.ExitCode != 0:
			return fmt.Errorf("%w (%s exited %d after its gate passed)", ErrServiceCrashed, a.Name, a.ExitCode)
		case len(before) > 0 && !known:
			return fmt.Errorf("%w (%s was not seen by its gate)", ErrServiceCrashed, a.Name)
		case known && b.State == "running" && b.StartedAt != "" && a.StartedAt != "" && a.StartedAt != b.StartedAt:
			return fmt.Errorf("%w (%s restarted after its gate passed)", ErrServiceCrashed, a.Name)
		case known && b.State == "running" && (a.State == "paused" || a.State == "dead"):
			return fmt.Errorf("%w (%s is %s after its gate passed)", ErrServiceCrashed, a.Name, a.State)
		case a.Health == "unhealthy", known && b.Health == "healthy" && (a.State != "running" || a.Health != "healthy"):
			return fmt.Errorf("%w (%s is %s/%s after its gate passed)", ErrServiceCrashed, a.Name, a.State, a.Health)
		}
	}
	for _, b := range before {
		if !present[key(b)] {
			return fmt.Errorf("%w (%s is gone after its gate passed)", ErrServiceCrashed, b.Name)
		}
	}
	return nil
}
