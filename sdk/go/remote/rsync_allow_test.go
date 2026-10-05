package remote

// Purpose: tests for the rsync option allowlist and the transport-first argv.
// No test connects to a host; ssh and rsync are stubs or counted no-ops.

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// TestRsyncTrailingValueFlagCannotSwallowTransport are the round-2 reviewer's
// repros: a value-taking flag placed last used to swallow the sdk's -e.
func TestRsyncTrailingValueFlagCannotSwallowTransport(t *testing.T) {
	n := countExecs(t)
	tg := ciTarget("node.invalid")
	for _, args := range [][]string{
		{"-az", "-T"}, {"-azT"}, {"-T"}, {"--exclude"}, {"--temp-dir"}, {"--log-file"},
		{"-az", "--exclude"}, {"--files-from"}, {"-B"}, {"-f"}, {"-M"},
	} {
		if err := Rsync(context.Background(), tg, args, "/tmp/a", "/dst"); err == nil {
			t.Errorf("Rsync args %q accepted", args)
		}
	}
	if *n != 0 {
		t.Fatalf("%d execs before refusal", *n)
	}
}

func TestRsyncAllowlist(t *testing.T) {
	ok := []string{
		"-a", "-az", "-avzh", "-rltpgoD", "-n", "-ui", "-c", "-vv",
		"--delete", "--checksum", "--partial", "--progress", "--compress", "--archive",
		"--recursive", "--times", "--perms", "--dry-run", "--itemize-changes", "--stats",
		"--human-readable", "--delete-after", "--mkpath",
		"--exclude=.git", "--exclude=a b", "--include=*.go", "--chmod=D755,F644",
		"--chown=deploy:deploy", "--timeout=30", "--bwlimit=1.5M", "--info=progress2,stats",
	}
	for _, a := range ok {
		if err := checkRsyncArgs([]string{a}); err != nil {
			t.Errorf("%q refused: %v", a, err)
		}
	}
	bad := []string{
		"", "-", "--", "x", "a:b", "-e", "-ze", "-eX", "--rsh", "--rsh=ssh", "--rs=x",
		"--rsync-path=sh", "--rsync-p=sh", "-M", "-zM", "--remote-option=-x", "--remote-o=x",
		"--files-from=/etc/passwd", "--files-from", "--include-from=/etc/passwd",
		"--exclude-from=/etc/passwd", "--password-file=/x", "--log-file=/x", "--write-batch=/x",
		"--read-batch=/x", "--only-write-batch=/x", "-T", "--temp-dir=/x", "-s", "--protect-args",
		"--secluded-args", "--no-protect-args", "--daemon", "--config=/x", "--server", "--sender",
		"--old-args", "--del", "--delete=x", "--exclude", "--chmod", "--chmod=a b", "--chown=$(id)",
		"--timeout=1;id", "--info=a`id`", "--bwlimit=", "-P", "-e=x", "--archive=x",
		"--exclude=a\nb", "--exclude=a\x00", "-a\n", "---x", "--Delete",
	}
	for _, a := range bad {
		if err := checkRsyncArgs([]string{a}); err == nil {
			t.Errorf("%q accepted", a)
		}
	}
}

func TestRsyncTransportComesFirstAndHoldsD4(t *testing.T) {
	log := stubTools(t, map[string]string{"rsync": "exit 0"})
	tg := ciTarget("h1")
	tg.Options = tg.Options[3:] // rsync -e never carries CISSHFlags
	args := []string{"-az", "--exclude=.git", "--delete"}
	if err := Rsync(context.Background(), tg, args, "./s", "/opt/x"); err != nil {
		t.Fatal(err)
	}
	got := readCalls(t, log)[0].args
	if got[0] != "-e" {
		t.Fatalf("argv does not start with -e: %q", got)
	}
	for i := 0; i < len(tg.Options); i += 2 {
		if !strings.Contains(got[1], tg.Options[i+1]) {
			t.Errorf("transport lacks D4 option %s", tg.Options[i+1])
		}
	}
	want := append(append([]string{}, args...), "--", "./s", "h1:/opt/x")
	if !reflect.DeepEqual(got[2:], want) {
		t.Fatalf("argv tail = %q, want %q", got[2:], want)
	}
}

// TestSimulatorCatchesTheOldShape proves the fuzz parser model flags the
// round-1 bug: the transport placed after a trailing value-taking flag.
func TestSimulatorCatchesTheOldShape(t *testing.T) {
	_, ops, eaten := simulateRsync([]string{"-az", "-T", "-e", "ssh x", "--", "s", "d"})
	if !reflect.DeepEqual(eaten, []int{2, 3}) || len(ops) != 2 {
		t.Fatalf("simulator: eaten %v operands %q", eaten, ops)
	}
	if rsh, _, eaten := simulateRsync([]string{"-e", "ssh x", "-az", "--", "s", "d"}); rsh != "ssh x" || !reflect.DeepEqual(eaten, []int{1}) {
		t.Fatalf("simulator: new shape rsh %q eaten %v", rsh, eaten)
	}
}
