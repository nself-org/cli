package docker

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/ports"
)

// ReservedPorts lists ports used by the nSelf stack that must be checked
// for conflicts before starting services.
var ReservedPorts = []int{
	80, 443, 5432, 8080, 4000, 6379,
	9000, 9001, 7700, 3021, 1025, 8025,
	3008, 5000,
}

// PortConflict records whether a specific port is already in use.
type PortConflict struct {
	Port  int
	InUse bool
}

// CheckPort probes a single TCP port on localhost using DialTimeout.
// It returns true when the port is in use (connection succeeded) and
// false when the port is available (connection refused or timed out).
//
// The error result is structurally always nil — see the dial branch below.
// Callers keep the two-value signature because it matches CheckAllPorts and
// CheckAllPortsFiltered, whose errors come from the compose ownership query,
// not from probing.
func CheckPort(port int) (bool, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	conn, err := net.DialTimeout("tcp", addr, 1*time.Second)
	if err != nil {
		// KEEP. A failed dial IS how a free port answers — ECONNREFUSED and
		// the timeout are the expected results, not failures, so there is no
		// error here worth reporting for the overwhelming majority of calls.
		//
		// A local-resource failure (EMFILE, ENOMEM) would also land here and
		// read as "available", and that is the direction to be wrong in: the
		// caller proceeds to start, and docker refuses the bind with the real
		// conflict named. The opposite default — treating an indeterminate
		// probe as a conflict — is precisely what kept the ɳTask staging stack
		// down, and what checkStartPorts now guards against by warning and
		// skipping when ownership cannot be established. A port check that is
		// occasionally too permissive costs one clear error from docker; one
		// that is too strict blocks a correct start with a wrong explanation.
		return false, nil
	}
	_ = conn.Close()
	// Connection succeeded — something is listening.
	return true, nil
}

// CheckAllPorts probes every port in the slice and returns only the
// entries where the port is already in use. A nil slice means no
// conflicts were found.
func CheckAllPorts(ports []int) ([]PortConflict, error) {
	var conflicts []PortConflict
	for _, p := range ports {
		inUse, err := CheckPort(p)
		if err != nil {
			return nil, fmt.Errorf("checking port %d: %w", p, err)
		}
		if inUse {
			conflicts = append(conflicts, PortConflict{Port: p, InUse: true})
		}
	}
	return conflicts, nil
}

// OwnedHostPorts queries the running compose stack in workdir and returns the
// set of host-side port numbers currently bound by nself's own containers.
// These ports should not be reported as conflicts on startup — they are already
// owned by nself and will be reused by docker compose up.
//
// envFiles are passed to docker compose as --env-file. They are REQUIRED once
// any plugin compose file is in play: an installed plugin's
// docker-compose.plugin.yml refers to ${DOCKER_NETWORK}, and without the env
// file that resolves to empty, so docker rejects the whole project with
// "service X refers to undefined network : invalid compose project" and this
// query fails.
//
// If the query fails the error is RETURNED rather than swallowed. Reporting
// "nothing is owned" on a failure turns "I could not tell" into a confident
// wrong answer: every port the project itself already binds is then reported
// as a conflict, and `nself start` aborts against its own running stack.
func OwnedHostPorts(ctx context.Context, workdir string, envFiles []string, composeFiles ...string) (map[int]bool, error) {
	c := NewCompose(composeFiles...)
	c.EnvFiles = envFiles
	containers, err := c.ComposePs(ctx, workdir)
	if err != nil {
		return nil, fmt.Errorf("listing running compose containers: %w", err)
	}

	owned := make(map[int]bool)
	for _, info := range containers {
		for _, p := range info.Ports {
			// Port strings from ComposePs look like "0.0.0.0:5432->5432/tcp"
			// or "127.0.0.1:5432->5432/tcp". Extract the host port number.
			hostPort := extractComposeHostPort(p)
			if hostPort > 0 {
				owned[hostPort] = true
			}
		}
	}
	return owned, nil
}

// extractComposeHostPort parses a Docker Compose port string of the form
// "[ip:]hostPort->containerPort[/proto]" and returns the host-side port
// as an integer. Returns 0 if the string cannot be parsed.
func extractComposeHostPort(s string) int {
	// Strip protocol suffix: "5432/tcp" → "5432".
	if idx := strings.LastIndex(s, "/"); idx >= 0 {
		// Only strip if it looks like a proto suffix (after "->").
		if arrowIdx := strings.Index(s, "->"); arrowIdx >= 0 && idx > arrowIdx {
			s = s[:idx]
		}
	}

	// Split on "->"; left side is [ip:]hostPort.
	parts := strings.SplitN(s, "->", 2)
	if len(parts) < 1 {
		return 0
	}
	hostPart := parts[0]

	// If the host part contains ":" it may be "ip:port" or just "port".
	if colIdx := strings.LastIndex(hostPart, ":"); colIdx >= 0 {
		hostPart = hostPart[colIdx+1:]
	}

	port, err := strconv.Atoi(strings.TrimSpace(hostPart))
	if err != nil || port <= 0 || port > 65535 {
		return 0
	}
	return port
}

// CheckAllPortsFiltered probes every port in the slice and returns only entries
// where the port is in use by a process that is NOT part of nself's own running
// compose stack. This prevents false positives where nself's own containers
// (e.g. postgres bound to 127.0.0.1:5432) are reported as conflicts on restart.
//
// workdir, envFiles and composeFiles are forwarded to OwnedHostPorts. If that
// query fails the error is returned rather than degrading to unfiltered
// behaviour: unfiltered means the project's own ports read as conflicts, which
// blocks a legitimate start instead of catching a real one.
func CheckAllPortsFiltered(ctx context.Context, portList []int, workdir string, envFiles []string, composeFiles ...string) ([]PortConflict, error) {
	owned, err := OwnedHostPorts(ctx, workdir, envFiles, composeFiles...)
	if err != nil {
		// Do not fall through to an empty set. Without knowing which ports the
		// project already owns, every one of them looks like a foreign
		// conflict and start would refuse over its own containers. Let the
		// caller decide (warn and skip) rather than emit false conflicts.
		return nil, err
	}

	return filterPortConflicts(portList, owned, CheckPort, holderProcessName)
}

// filterPortConflicts is the single decision for "is this port actually a
// problem". Every caller — `nself start`'s pre-flight and `nself doctor`'s
// port checks — routes through here so the two cannot disagree about the same
// port on the same machine.
//
// A port counts as a conflict only when it is in use AND not held by this
// project's own compose stack AND not one of Docker Desktop's transient
// internal holders. probe and holder are injected so the decision is testable
// without a Docker daemon or a real listener.
func filterPortConflicts(portList []int, owned map[int]bool, probe func(int) (bool, error), holder func(int) string) ([]PortConflict, error) {
	var conflicts []PortConflict
	for _, p := range portList {
		inUse, err := probe(p)
		if err != nil {
			return nil, fmt.Errorf("checking port %d: %w", p, err)
		}
		if !inUse || owned[p] {
			continue
		}
		// Port is in use by something outside the current compose stack.
		// On macOS, Docker Desktop holds ports briefly after container removal
		// via com.docker.backend (the hypervisor process). These are transient
		// Docker-internal holds — not real conflicts. Skip them so that
		// stop→start and restart workflows don't false-positive.
		if isDockerInternalHolder(holder(p)) {
			continue
		}
		conflicts = append(conflicts, PortConflict{Port: p, InUse: true})
	}
	return conflicts, nil
}

// holderProcessName returns the name of the process holding port, or "" when
// it cannot be determined. An unknown holder is not treated as Docker-internal
// — silently skipping what we could not identify would hide real conflicts.
func holderProcessName(port int) string {
	holder, _ := ports.WhoHoldsPort(port)
	if holder == nil {
		return ""
	}
	return holder.Name
}

// isDockerInternalHolder returns true when the process name belongs to Docker
// Desktop's internal infrastructure on macOS. These processes hold port
// reservations briefly after containers stop and should not be treated as
// external conflicts.
func isDockerInternalHolder(name string) bool {
	dockerInternalNames := []string{
		"com.docker.backend",
		"com.docker.hyperkit",
		"com.docker.vpnkit",
		"vpnkit-bridge",
	}
	for _, n := range dockerInternalNames {
		if strings.EqualFold(name, n) {
			return true
		}
	}
	return false
}

// CheckPortsUnowned probes portList with an empty ownership set, for the case
// where there is demonstrably no compose project in the workdir and so nothing
// of ours can be holding a port. It routes through the same decision as the
// filtered path, so Docker Desktop's transient internal holders are still not
// reported as conflicts.
func CheckPortsUnowned(portList []int) ([]PortConflict, error) {
	return filterPortConflicts(portList, nil, CheckPort, holderProcessName)
}
