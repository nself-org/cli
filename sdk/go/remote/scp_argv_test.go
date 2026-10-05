package remote

// Purpose: tests for the scp option contract (scpOptions) and the D4 pinned
// known-hosts path. scp and ssh are stubs or counted no-ops; no host is used.

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// scpValueShort lists the scp(1) options that take a value; scpPlainShort the
// ones that do not (man scp, SYNOPSIS). Any other letter is unknown to scp.
const (
	scpValueShort = "cDFiJloPSX"
	scpPlainShort = "346ABCOpqRrsTv"
)

// simulateScp parses argv like scp (BSD getopt: options end at "--" or at the
// first operand; a value-taking letter last in a bundle takes the next
// element, otherwise the rest of the bundle). It returns the index where
// options ended, the operands, the indexes eaten as values and any option
// letter scp does not define.
func simulateScp(args []string) (end int, operands []string, eaten []int, unknown string) {
	i := 0
	for ; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			return i, args[i+1:], eaten, unknown
		}
		if len(a) < 2 || a[0] != '-' {
			return i, args[i:], eaten, unknown
		}
		for j := 1; j < len(a); j++ {
			switch {
			case strings.IndexByte(scpValueShort, a[j]) >= 0:
				if j == len(a)-1 && i+1 < len(args) {
					i++
					eaten = append(eaten, i)
				}
				j = len(a)
			case strings.IndexByte(scpPlainShort, a[j]) >= 0:
			default:
				unknown += string(a[j])
			}
		}
	}
	return i, nil, eaten, unknown
}

// checkScpArgv asserts the scp invariants for one captured exec: scp's own
// parser reaches the sdk's "--" at the expected index, no caller element was
// swallowed as the "--" or an operand, the operands are exactly [src dst], no
// ssh-only flag is present and the D4 block comes first.
func checkScpArgv(t testing.TB, c captured, d4 []string, src, dst string) {
	t.Helper()
	n := len(c.args)
	if n < len(d4)+3 || c.args[n-3] != "--" || c.args[n-2] != src || c.args[n-1] != dst {
		t.Fatalf("scp argv %q: tail is not -- src dst", c.args)
	}
	if !reflect.DeepEqual(c.args[:len(d4)], d4) {
		t.Fatalf("scp argv %q: D4 block is not first", c.args)
	}
	end, operands, eaten, unknown := simulateScp(c.args)
	if end != n-3 {
		t.Fatalf("scp argv %q: option parsing ended at %d, want %d (the sdk \"--\")", c.args, end, n-3)
	}
	for _, e := range eaten {
		if e >= n-3 {
			t.Fatalf("scp argv %q: element %d was consumed as an option value", c.args, e)
		}
	}
	if !reflect.DeepEqual(operands, []string{src, dst}) {
		t.Fatalf("scp argv %q: operands %q, want [src dst]", c.args, operands)
	}
	if unknown != "" {
		t.Fatalf("scp argv %q: option letters %q are not scp options", c.args, unknown)
	}
	for _, a := range c.args[:n-3] {
		if a == "-T" || a == "-a" || a == "-x" {
			t.Fatalf("scp argv %q holds ssh-only flag %s", c.args, a)
		}
	}
}

func TestCopyToPortFormKeepsDashDash(t *testing.T) {
	d4 := CIOptions("n1", "/pin/kh", Version{9, 6})
	cases := map[string][]string{
		"ssh -p":    {"-p", "22"},
		"Port key":  {"-o", "Port=2222"},
		"attached":  {"-oPort=2222"},
		"-p and -i": {"-i", "/k/id", "-p", "2200", "-4"},
	}
	for name, tail := range cases {
		got := captureExecs(t)
		tg := Target{Dest: "h1", Options: append(append([]string{}, d4...), tail...)}
		if err := CopyTo(context.Background(), tg, "./src.txt", "/opt/dst"); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		c := (*got)[0]
		checkScpArgv(t, c, d4, "./src.txt", "h1:/opt/dst")
		if name == "ssh -p" {
			if !hasArg(c.args, "-P") || hasArg(c.args, "-p") {
				t.Errorf("%s: port not translated to -P: %q", name, c.args)
			}
			if i := indexOf(c.args, "-P"); c.args[i+1] != "22" {
				t.Errorf("%s: -P value %q, want 22", name, c.args[i+1])
			}
		}
	}
}

func TestScpOldBehaviorWasUnsafe(t *testing.T) {
	// Self-check of the simulator: a raw `-p 22` makes "22" an operand and the
	// sdk's "--" an operand too, which is the round-3 reviewer's repro.
	_, operands, _, _ := simulateScp([]string{"-p", "22", "--", "./src.txt", "h1:/dst"})
	if len(operands) != 4 || operands[0] != "22" {
		t.Fatalf("simulator does not model -p as a no-value flag: %q", operands)
	}
}

func TestCopyToDropsSSHOnlyFlags(t *testing.T) {
	d4 := CIOptions("n1", "/pin/kh", Version{9, 6})
	got := captureExecs(t)
	tg := Target{Dest: "h1", Options: append(CISSHFlags(), d4...)}
	if err := CopyTo(context.Background(), tg, "./a", "/opt/a"); err != nil {
		t.Fatal(err)
	}
	checkScpArgv(t, (*got)[0], d4, "./a", "h1:/opt/a")
	// Also in the middle of a tail, never as an -o/-i value.
	got2 := captureExecs(t)
	tg = Target{Dest: "h1", Options: append(append([]string{}, d4...), "-i", "/k/-T", "-v")}
	if err := CopyTo(context.Background(), tg, "./a", "/opt/a"); err != nil {
		t.Fatal(err)
	}
	checkScpArgv(t, (*got2)[0], d4, "./a", "h1:/opt/a")
}

func TestScpOptionsRefusesUnknown(t *testing.T) {
	for _, o := range [][]string{{"-F", "x"}, {"-S", "x"}, {"-p"}, {"-o"}, {"--", "x"}, {"-vv"}, {"x"}} {
		if _, err := scpOptions(o); err == nil {
			t.Errorf("scpOptions(%q) accepted", o)
		}
	}
}

func TestD4KnownHostsPathMustBeLiteral(t *testing.T) {
	for _, p := range []string{"/tmp/kh-%h", "%d/kh", "/tmp/%%", "~/kh", "~root/kh", "/tmp/$HOME/kh", "/tmp/${X}", "/dev/null", "none", "-x"} {
		opts := CIOptions("n1", p, Version{9, 6})
		if err := checkOptions(opts); err != nil {
			continue
		}
		// A refused D4 block falls through to the allowlist, which refuses
		// UserKnownHostsFile in every form: nothing may be accepted.
		t.Errorf("pinned path %q accepted", p)
	}
	for _, p := range []string{"/pin/known_hosts", "/var/lib/nself/kh.d/n1", "./kh", "kh_1.txt"} {
		if err := checkOptions(CIOptions("n1", p, Version{9, 6})); err != nil {
			t.Errorf("literal path %q refused: %v", p, err)
		}
	}
}
