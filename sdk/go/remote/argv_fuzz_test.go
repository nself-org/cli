package remote

// Purpose: fuzz the argv builders of Run, RunArgv, CopyTo and Rsync.
// Invariants for every call that reaches exec:
//   - ssh/scp: the Target's options come first and in order, then (for a
//     caller option tail) only allowlisted elements, then the sdk's "--";
//     nothing after "--" starts with '-'.
//   - rsync: argv[0] is -e and argv[1] is the sdk transport, which holds every
//     D4 option; the caller's elements are allowlisted, and a simulation of
//     rsync's own option parser finds that no caller element consumed the
//     sdk's -e, "--", the source or the destination.
// Calls that break a precondition must be refused before exec instead.

import (
	"context"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

type captured struct {
	tool string
	args []string
}

func captureExecs(t testing.TB) *[]captured {
	t.Helper()
	var got []captured
	orig := commandContext
	commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		got = append(got, captured{name, append([]string(nil), args...)})
		return exec.CommandContext(ctx, "true")
	}
	t.Cleanup(func() { commandContext = orig })
	return &got
}

// checkSSHArgv asserts prefix, then "--", then no element starting with '-'.
func checkSSHArgv(t *testing.T, c captured, prefix []string) {
	t.Helper()
	if len(c.args) < len(prefix) || !reflect.DeepEqual(c.args[:len(prefix)], prefix) {
		t.Fatalf("%s argv %q does not start with %q", c.tool, c.args, prefix)
	}
	if len(c.args) < len(prefix)+1 || c.args[len(prefix)] != "--" {
		t.Fatalf("%s argv %q: the sdk \"--\" does not follow the options", c.tool, c.args)
	}
	for _, a := range c.args[len(prefix)+1:] {
		if len(a) > 0 && a[0] == '-' {
			t.Fatalf("%s argv %q: element %q after \"--\" starts with '-'", c.tool, c.args, a)
		}
	}
}

// rsyncValueShort and rsyncValueLong over-approximate the rsync options that
// take a separate value; a caller element that is one of them (without an
// attached value) would swallow the next argv element.
const rsyncValueShort = "eMTBf@"

var rsyncValueLong = strings.Fields(`rsh rsync-path remote-option temp-dir partial-dir files-from
	include-from exclude-from exclude include filter password-file log-file log-file-format
	write-batch read-batch only-write-batch chmod chown usermap groupmap timeout bwlimit info debug
	compare-dest copy-dest link-dest backup-dir suffix block-size port address sockopts out-format
	max-size min-size max-delete modify-window checksum-choice compress-choice compress-level
	skip-compress stop-after stop-at iconv outbuf early-input copy-as dparam contimeout`)

// simulateRsync parses argv like rsync: options until "--", a value-taking
// option eating the next element when it has no attached value. It returns the
// -e value, the operands and the indexes of elements eaten as values.
func simulateRsync(args []string) (rsh string, operands []string, eaten []int) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		take := func() {
			if i+1 < len(args) {
				i++
				eaten = append(eaten, i)
			}
		}
		switch {
		case a == "--":
			return rsh, args[i+1:], eaten
		case strings.HasPrefix(a, "--"):
			name, _, hasVal := strings.Cut(a[2:], "=")
			for _, l := range rsyncValueLong {
				if !hasVal && (name == l || len(name) > 0 && strings.HasPrefix(l, name)) {
					take()
					break
				}
			}
		case strings.HasPrefix(a, "-") && len(a) > 1:
			for j := 1; j < len(a); j++ {
				if strings.IndexByte(rsyncValueShort, a[j]) < 0 {
					continue
				}
				if j == len(a)-1 {
					if a[j] == 'e' && i+1 < len(args) {
						rsh = args[i+1]
					}
					take()
				} else if a[j] == 'e' {
					rsh = a[j+1:]
				}
				break
			}
		default:
			operands = append(operands, a)
		}
	}
	return rsh, operands, eaten
}

// checkRsyncArgv asserts the rsync invariants for one captured exec.
func checkRsyncArgv(t *testing.T, c captured, opts, caller []string, src, dstOperand string) {
	t.Helper()
	transport := "ssh " + strings.Join(opts, " ")
	if len(c.args) < 2 || c.args[0] != "-e" || c.args[1] != transport {
		t.Fatalf("rsync argv %q does not start with -e <sdk transport>", c.args)
	}
	for _, o := range opts {
		if !strings.Contains(c.args[1], o) {
			t.Fatalf("transport %q lacks D4 option %q", c.args[1], o)
		}
	}
	if err := checkRsyncArgs(caller); err != nil {
		t.Fatalf("rsync ran with a non-allowlisted caller arg: %v", err)
	}
	n := len(c.args)
	if n < 5 || c.args[n-3] != "--" || c.args[n-2] != src {
		t.Fatalf("rsync argv %q: tail is not -- src dst", c.args)
	}
	if !reflect.DeepEqual(c.args[2:n-3], caller) {
		t.Fatalf("rsync argv %q: caller elements altered or reordered", c.args)
	}
	rsh, operands, eaten := simulateRsync(c.args)
	if rsh != transport {
		t.Fatalf("rsync argv %q: parsed rsh %q is not the sdk transport", c.args, rsh)
	}
	if !reflect.DeepEqual(eaten, []int{1}) {
		t.Fatalf("rsync argv %q: elements %v consumed as option values; only -e's (1) may be", c.args, eaten)
	}
	if !reflect.DeepEqual(operands, []string{src, c.args[n-1]}) || c.args[n-1] != dstOperand {
		t.Fatalf("rsync argv %q: operands %q, want [src dst]", c.args, operands)
	}
}

func FuzzArgvBuilders(f *testing.F) {
	seeds := [][6]string{
		{"h1", "ls", "./a", "/opt/a", "-az", "ConnectTimeout=5"},
		{"deploy@2001:db8::1", "id", "./a", "/opt/a", "--delete", "Port=22"},
		{"node.invalid:", "-x", "-rf", "/srv/../a", "--", "ProxyCommand=x"},
		{"h1", "x", "./a", "/opt/a", "-az\x00--", "ControlPath=/tmp/x"},
		{"h1", `\';id #`, "evil:file", "-x", "other:/etc", "UserKnownHostsFile=/dev/null"},
		{"h1", "x", "./a", "/opt/a", "-T", "-F x"},
		{"h1", "x", "./a", "/opt/a", "--exclude", "Port 22"},
		{"h1", "x", "./a", "/opt/a", "-azT", "connecttimeout = 5"},
	}
	for _, s := range seeds {
		f.Add(s[0], s[1], s[2], s[3], s[4], s[5])
	}
	opts := CIOptions("n1", "/pin/known_hosts", Version{9, 6})
	got := captureExecs(f)
	ctx := context.Background()
	f.Fuzz(func(t *testing.T, dest, command, local, remote, rsyncArg, sshOpt string) {
		*got = (*got)[:0]
		tg := Target{Dest: dest, Options: opts}
		// Two more Targets carry a caller option tail after the D4 block.
		tails := [][]string{{"-o", sshOpt}, {"-o" + sshOpt}}

		_, _ = Run(ctx, tg, command)
		_, _ = RunArgv(ctx, tg, command, rsyncArg)
		_ = CopyTo(ctx, tg, local, remote)
		_ = Rsync(ctx, tg, []string{rsyncArg}, local, remote)
		sshN := len(*got)
		for _, tail := range tails {
			tt := Target{Dest: dest, Options: append(append([]string{}, opts...), tail...)}
			_, _ = Run(ctx, tt, command)
			_ = Rsync(ctx, tt, []string{rsyncArg}, local, remote)
		}

		for i, c := range *got {
			switch c.tool {
			case "ssh", "scp":
				if i < sshN {
					checkSSHArgv(t, c, opts)
				}
			case "rsync":
				dst := scpOperand(dest, remote)
				if i < sshN {
					checkRsyncArgv(t, c, opts, []string{rsyncArg}, local, dst)
				} else {
					all := c.optsFromTransport()
					if len(all) < len(opts) || !reflect.DeepEqual(all[:len(opts)], opts) {
						t.Fatalf("rsync transport %q: D4 block is not first", c.args[1])
					}
					checkRsyncArgv(t, c, all, []string{rsyncArg}, local, dst)
				}
			default:
				t.Fatalf("unexpected tool %q", c.tool)
			}
		}
		// ssh runs with a tail: D4 first, then an allowlisted tail, then "--".
		for _, c := range (*got)[sshN:] {
			if c.tool != "ssh" {
				continue
			}
			end := indexOf(c.args, "--")
			if end < len(opts) || !reflect.DeepEqual(c.args[:len(opts)], opts) {
				t.Fatalf("ssh argv %q: D4 block is not first", c.args)
			}
			if err := checkCallerOptions(c.args[len(opts):end]); err != nil {
				t.Fatalf("ssh ran with a non-allowlisted tail %q: %v", c.args[len(opts):end], err)
			}
			checkSSHArgv(t, c, c.args[:end])
		}
	})
}

// optsFromTransport splits the -e value back into the option elements.
func (c captured) optsFromTransport() []string {
	return strings.Fields(strings.TrimPrefix(c.args[1], "ssh "))
}
