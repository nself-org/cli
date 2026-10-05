package simharness

// Purpose: fault injection: Pause/Unpause (frozen processes), Netem (delay and loss), Partition/Heal (network detach).
// Inputs: node names and fault parameters.
// Outputs: the node's behaviour changes; Heal and Unpause undo it, Close removes everything.
// Constraints: Netem needs NET_ADMIN in that node's NodeSpec.CapAdd and tc in the image; nothing runs --privileged.

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Pause freezes every process in the node (docker pause). TCP connections are
// accepted by the kernel but sshd never answers, so Banner times out.
func (f *Fleet) Pause(t TB, node string) {
	t.Helper()
	f.simple(t, "pause", node)
}

// Unpause resumes a paused node.
func (f *Fleet) Unpause(t TB, node string) {
	t.Helper()
	f.simple(t, "unpause", node)
}

func (f *Fleet) simple(t TB, verb, node string) {
	t.Helper()
	n := f.Node(t, node)
	if _, err := docker(context.Background(), verb, n.Container); err != nil {
		t.Fatalf("simharness: %v", err)
	}
}

// Netem adds delay and packet loss (loss in percent, 0-100) to everything the
// node sends, on every interface except lo. Delay 0 and loss 0 remove the
// rules. The node needs NET_ADMIN in NodeSpec.CapAdd and tc in its image.
func (f *Fleet) Netem(t TB, node string, delay time.Duration, loss float64) {
	t.Helper()
	n := f.Node(t, node)
	if delay < 0 || loss < 0 || loss > 100 {
		t.Fatalf("simharness: Netem(delay=%v, loss=%v) out of range", delay, loss)
	}
	if !hasCap(n.spec.CapAdd, "NET_ADMIN") {
		t.Fatalf("simharness: node %s needs NET_ADMIN in NodeSpec.CapAdd for Netem", node)
	}
	if _, err := docker(context.Background(), "exec", n.Container, "sh", "-c", netemScript(delay, loss)); err != nil {
		t.Fatalf("simharness: netem on %s: %v", node, err)
	}
}

func hasCap(caps []string, want string) bool {
	for _, c := range caps {
		if strings.EqualFold(strings.TrimPrefix(strings.ToUpper(c), "CAP_"), want) {
			return true
		}
	}
	return false
}

// netemScript builds the in-container shell that applies (or clears) netem on
// every non-loopback interface. Only numbers formatted here reach the shell.
func netemScript(delay time.Duration, loss float64) string {
	body := `tc qdisc del dev "$n" root 2>/dev/null || true`
	if delay > 0 || loss > 0 {
		body = fmt.Sprintf(`tc qdisc replace dev "$n" root netem delay %dms loss %.2f%%`, delay.Milliseconds(), loss)
	}
	return `set -e; for d in /sys/class/net/*; do n=${d##*/}; [ "$n" = lo ] && continue; ` + body + `; done`
}

// Partition cuts the node off: it leaves the fleet network, so other nodes and
// the test host cannot reach it. Its processes keep running. Heal undoes it.
func (f *Fleet) Partition(t TB, node string) {
	t.Helper()
	n := f.Node(t, node)
	if _, err := docker(context.Background(), "network", "disconnect", f.Network, n.Container); err != nil {
		t.Fatalf("simharness: %v", err)
	}
}

// Heal reconnects a partitioned node under its original alias and waits for
// sshd. The published port may change; read it again from Node.Port.
func (f *Fleet) Heal(t TB, node string) {
	t.Helper()
	n := f.Node(t, node)
	if _, err := docker(context.Background(), "network", "connect", "--alias", n.Name, f.Network, n.Container); err != nil {
		t.Fatalf("simharness: %v", err)
	}
	f.settle(t, n)
}
