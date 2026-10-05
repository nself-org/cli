//go:build integration

package simharness

// Purpose: helpers for the Docker-backed tests: ssh from the host, timing, container listing.
// Constraints: INTEGRATION=1 and a Docker daemon are required; the tests fail, never skip, without them.

import (
	"context"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// hostSSH runs argv on the node over ssh from the test host with the fleet key.
func hostSSH(f *Fleet, n *Node, user string, timeout time.Duration, argv ...string) (string, error) {
	args := []string{"-i", f.KeyPath(), "-p", strconv.Itoa(n.Port),
		"-o", "BatchMode=yes", "-o", "IdentitiesOnly=yes",
		"-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null", "-o", "LogLevel=ERROR",
		"-o", "ConnectTimeout=" + strconv.Itoa(int(timeout.Seconds())),
		user + "@" + n.Host, "--"}
	ctx, cancel := context.WithTimeout(context.Background(), timeout+5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ssh", append(args, argv...)...).Output()
	return strings.TrimSpace(string(out)), err
}

// mustSSH fails the test unless the command succeeds.
func mustSSH(t *testing.T, f *Fleet, n *Node, argv ...string) string {
	t.Helper()
	out, err := hostSSH(f, n, n.User, 20*time.Second, argv...)
	if err != nil {
		t.Fatalf("ssh %s %v: %v", n.Name, argv, err)
	}
	return out
}

// medianBanner returns the median time from connect to the SSH banner.
func medianBanner(t *testing.T, n *Node, samples int) time.Duration {
	t.Helper()
	var ds []time.Duration
	for i := 0; i < samples; i++ {
		start := time.Now()
		if _, err := n.Banner(5 * time.Second); err != nil {
			t.Fatalf("banner: %v", err)
		}
		ds = append(ds, time.Since(start))
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
	return ds[len(ds)/2]
}

// fleetObjects lists the containers and networks carrying the fleet label.
func fleetObjects(t *testing.T, id string) []string {
	t.Helper()
	var all []string
	for _, args := range [][]string{
		{"ps", "-a", "--format", "{{.Names}}", "--filter", "label=" + LabelFleet + "=" + id},
		{"network", "ls", "--format", "{{.Name}}", "--filter", "label=" + LabelFleet + "=" + id},
	} {
		out, err := docker(context.Background(), args...)
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, strings.Fields(out)...)
	}
	return all
}

func testCtx() context.Context { return context.Background() }
