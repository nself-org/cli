package sysexec

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestHeadless(t *testing.T) {
	cases := []struct {
		name, headless, ci string
		want               bool
	}{
		{"neither", "", "", false},
		{"headless_1", "1", "", true},
		{"headless_true_upper", "TRUE", "", true},
		{"headless_on", "on", "", true},
		{"ci_1", "", "1", true},
		{"ci_true", "", "true", true},
		{"ci_0", "", "0", false},
		{"headless_0_beats_ci", "0", "1", false},
		{"headless_off_beats_ci", "off", "true", false},
		{"headless_garbage_falls_to_ci", "random", "1", true},
		{"headless_garbage_no_ci", "random", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("NSELF_HEADLESS", tc.headless)
			t.Setenv("CI", tc.ci)
			if got := Headless(); got != tc.want {
				t.Errorf("Headless() NSELF_HEADLESS=%q CI=%q = %v, want %v", tc.headless, tc.ci, got, tc.want)
			}
		})
	}
}

func TestSetRestoresPrevious(t *testing.T) {
	f1, f2 := &Fake{}, &Fake{}
	restore1 := Set(f1)
	restore2 := Set(f2)
	_ = Run(context.Background(), "true")
	if len(f2.Calls()) != 1 || len(f1.Calls()) != 0 {
		t.Fatalf("Set did not route to the newest fake: f1=%d f2=%d", len(f1.Calls()), len(f2.Calls()))
	}
	restore2()
	_ = Run(context.Background(), "true")
	if len(f1.Calls()) != 1 {
		t.Errorf("restore did not bring back the previous runner")
	}
	restore1()
}

func TestFakeRecordsAndReplies(t *testing.T) {
	boom := errors.New("boom")
	f := &Fake{Reply: func(c Call) ([]byte, error) {
		if c.Name == "brew" {
			return []byte("nope"), boom
		}
		return []byte("ok"), nil
	}}
	defer Set(f)()
	out, err := Output(context.Background(), "brew", "list", "dnsmasq")
	if !errors.Is(err, boom) || string(out) != "nope" {
		t.Errorf("reply not honoured: %q %v", out, err)
	}
	if err := Stream(context.Background(), "mkcert", "-CAROOT"); err != nil {
		t.Errorf("Stream: %v", err)
	}
	if !f.Saw("brew list dnsmasq") || !f.Saw("mkcert -CAROOT") {
		t.Errorf("calls not recorded: %v", f.Names())
	}
}

// TestGuard_PanicsOnPrivilegedCommands is the guard's own test, with a
// positive control: osascript and sudo must panic before anything executes,
// and a read-only probe must not.
func TestGuard_PanicsOnPrivilegedCommands(t *testing.T) {
	panicsOn := func(name string, args ...string) (panicked bool) {
		defer func() {
			if r := recover(); r != nil {
				panicked = strings.Contains(r.(string), "privileged command")
			}
		}()
		// The real runner, not a fake: this is the path a forgetful test takes.
		_, _ = execRunner{}.Output(context.Background(), name, args...)
		return false
	}
	for _, c := range [][]string{
		{"osascript", "-e", "return 1"},
		{"/usr/bin/osascript", "-e", "return 1"},
		{"sudo", "-n", "true"},
		{"security", "add-trusted-cert", "x.pem"},
		{"pfctl", "-ef", "/nonexistent"},
		{"launchctl", "load", "-w", "/nonexistent.plist"},
		{"brew", "services", "restart", "dnsmasq"},
		{"brew", "install", "dnsmasq"},
		{"mkcert", "-install"},
		{"killall", "-HUP", "mDNSResponder"},
		// Not on the read-only allowlist: shells, wrappers, long options and
		// verbs a deny-list would have missed.
		{"sh", "-c", "echo hi"},
		{"env", "sudo", "true"},
		{"tee", "/etc/hosts"},
		{"pfctl", "-E"},
		{"iptables", "--append", "OUTPUT"},
		{"systemctl", "try-restart", "dnsmasq"},
		{"brew", "--verbose", "install", "dnsmasq"},
		{"security", "import", "x.p12"},
	} {
		if !panicsOn(c[0], c[1:]...) {
			t.Errorf("guard let a privileged command through: %v", c)
		}
	}
	// Positive control for the control: the same harness reports a safe
	// command as not panicking, so a green run is not a harness that always
	// returns true.
	if panicsOn("true") {
		t.Error("guard panicked on a harmless command")
	}
	for _, c := range [][]string{
		{"brew", "list", "dnsmasq"},
		{"pfctl", "-a", "nself.local", "-sr"},
		{"launchctl", "print", "system/com.nself.portforward"},
		{"security", "verify-cert", "-c", "x.pem"},
		{"mkcert", "-CAROOT"},
		{"iptables", "-t", "nat", "-C", "OUTPUT", "-p", "tcp"},
		{"iptables-save"},
	} {
		if got := Privileged(c[0], c[1:]); got != "" {
			t.Errorf("read-only probe %v flagged as %q", c, got)
		}
	}
}
