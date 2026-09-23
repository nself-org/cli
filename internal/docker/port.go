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
func CheckPort(port int) (bool, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	conn, err := net.DialTimeout("tcp", addr, 1*time.Second)
	if err != nil {
		// Connection refused or timed out — port is available.
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
			// or "127.0.0.1:5432->5432/tcp". Docker collapses consecutive
			// ports into a range ("127.0.0.1:9000-9001->9000-9001/tcp", MinIO
			// API + console), so every port in the range is owned.
			for _, hostPort := range extractComposeHostPorts(p) {
				owned[hostPort] = true
			}
		}
	}
	return owned, nil
}

// maxComposePortRange bounds how many ports one range entry may expand to, so
// a malformed or hostile string cannot allocate an unbounded slice.
const maxComposePortRange = 1024

// extractComposeHostPorts returns every host-side port in a Docker Compose
// port string: one port for "[ip:]5432->5432/tcp", each port for a range
// such as "127.0.0.1:9000-9001->9000-9001/tcp". Returns nil when the string
// cannot be parsed. Before this, a range parsed as 0, so a project's own
// MinIO read as a foreign holder of 9000/9001 and nself start refused to run
// (nself-web prod, 2026-09-23).
func extractComposeHostPorts(s string) []int {
	if p := extractComposeHostPort(s); p > 0 {
		return []int{p}
	}
	hostPart := composeHostPart(s)
	lo, hi, ok := strings.Cut(hostPart, "-")
	if !ok {
		return nil
	}
	first, err1 := strconv.Atoi(strings.TrimSpace(lo))
	last, err2 := strconv.Atoi(strings.TrimSpace(hi))
	if err1 != nil || err2 != nil || first <= 0 || last > 65535 || first > last || last-first >= maxComposePortRange {
		return nil
	}
	out := make([]int, 0, last-first+1)
	for p := first; p <= last; p++ {
		out = append(out, p)
	}
	return out
}

// extractComposeHostPort parses a Docker Compose port string of the form
// "[ip:]hostPort->containerPort[/proto]" and returns the host-side port
// as an integer. Returns 0 if the string cannot be parsed, including a
// range; extractComposeHostPorts handles ranges.
func extractComposeHostPort(s string) int {
	port, err := strconv.Atoi(strings.TrimSpace(composeHostPart(s)))
	if err != nil || port <= 0 || port > 65535 {
		return 0
	}
	return port
}

// composeHostPart returns the host-port field of a Compose port string:
// "127.0.0.1:9000-9001->9000-9001/tcp" -> "9000-9001".
func composeHostPart(s string) string {
	// Strip protocol suffix: "5432/tcp" → "5432".
	if idx := strings.LastIndex(s, "/"); idx >= 0 {
		// Only strip if it looks like a proto suffix (after "->").
		if arrowIdx := strings.Index(s, "->"); arrowIdx >= 0 && idx > arrowIdx {
			s = s[:idx]
		}
	}

	// Split on "->"; left side is [ip:]hostPort.
	parts := strings.SplitN(s, "->", 2)
	hostPart := parts[0]

	// If the host part contains ":" it may be "ip:port" or just "port".
	if colIdx := strings.LastIndex(hostPart, ":"); colIdx >= 0 {
		hostPart = hostPart[colIdx+1:]
	}
	return hostPart
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

	var conflicts []PortConflict
	for _, p := range portList {
		inUse, err := CheckPort(p)
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
		holder, _ := ports.WhoHoldsPort(p)
		if holder != nil && isDockerInternalHolder(holder.Name) {
			continue
		}
		conflicts = append(conflicts, PortConflict{Port: p, InUse: true})
	}
	return conflicts, nil
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
