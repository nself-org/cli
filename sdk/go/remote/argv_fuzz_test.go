package remote

// Purpose: fuzz the argv builders of Run, RunArgv, CopyTo and Rsync.
// Invariant for every call that reaches exec: all of the Target's options
// come first and in order, the sdk's "--" follows them, and no element after
// that "--" starts with '-'. Calls that break a precondition must be refused
// before exec instead.

import (
	"context"
	"os/exec"
	"reflect"
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

// checkArgv asserts the invariant on one captured exec. prefix is the
// element list that must start the argv (for rsync, the caller options come
// first, then -e and the ssh option string, so rsync passes its own prefix).
func checkArgv(t *testing.T, c captured, prefix []string) {
	t.Helper()
	if len(c.args) < len(prefix) || !reflect.DeepEqual(c.args[:len(prefix)], prefix) {
		t.Fatalf("%s argv %q does not start with %q", c.tool, c.args, prefix)
	}
	dd := -1
	for i := len(prefix); i < len(c.args); i++ {
		if c.args[i] == "--" {
			dd = i
			break
		}
	}
	if dd < 0 {
		t.Fatalf("%s argv %q has no \"--\" after the options", c.tool, c.args)
	}
	for _, a := range c.args[dd+1:] {
		if len(a) > 0 && a[0] == '-' {
			t.Fatalf("%s argv %q: element %q after \"--\" starts with '-'", c.tool, c.args, a)
		}
	}
}

func FuzzArgvBuilders(f *testing.F) {
	seeds := [][5]string{
		{"h1", "ls", "./a", "/opt/a", "-az"},
		{"deploy@2001:db8::1", "id", "./a", "/opt/a", "--delete"},
		{"node.invalid:", "-x", "-rf", "/srv/../a", "--"},
		{"h1", "x", "./a", "/opt/a", "-az\x00--"},
		{"h1", `\';id #`, "evil:file", "-x", "other:/etc"},
	}
	for _, s := range seeds {
		f.Add(s[0], s[1], s[2], s[3], s[4])
	}
	opts := CIOptions("n1", "/pin/known_hosts", Version{9, 6})
	got := captureExecs(f)
	ctx := context.Background()
	f.Fuzz(func(t *testing.T, dest, command, local, remote, rsyncArg string) {
		*got = (*got)[:0]
		tg := Target{Dest: dest, Options: opts}

		_, _ = Run(ctx, tg, command)
		_, _ = RunArgv(ctx, tg, command, rsyncArg)
		_ = CopyTo(ctx, tg, local, remote)
		_ = Rsync(ctx, tg, []string{rsyncArg}, local, remote)

		for _, c := range *got {
			switch c.tool {
			case "ssh", "scp":
				checkArgv(t, c, opts)
			case "rsync":
				// caller options, then -e "ssh <opts>", then "--".
				checkArgv(t, c, []string{rsyncArg, "-e", "ssh " + joinSpace(opts)})
			default:
				t.Fatalf("unexpected tool %q", c.tool)
			}
		}
	})
}

func joinSpace(a []string) string {
	s := ""
	for i, x := range a {
		if i > 0 {
			s += " "
		}
		s += x
	}
	return s
}
