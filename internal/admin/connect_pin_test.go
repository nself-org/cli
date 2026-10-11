package admin

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

// v15Verify returns the exact v1.5 VerifySSHKey argv up to the destination.
func v15Verify(pin, alias string) []string {
	args := []string{
		"-o", "BatchMode=yes", "-o", "ConnectTimeout=10", "-o", "ServerAliveInterval=30",
		"-o", "StrictHostKeyChecking=yes",
		"-o", `GlobalKnownHostsFile="` + pin + `"`,
	}
	if alias != "" {
		args = append(args, "-o", "HostKeyAlias="+alias)
	}
	return args
}

// TestAdminSSHV15PinOnlyExact pins the pin-file-only path: known_hosts is
// absent, the key lives in the nSelf pin file under the nself-<host>-<port>
// alias, and the argv adds HostKeyAlias with the quoted pin file. Covers a DNS
// name and a bracketed IPv6 literal (colons become dashes in the alias).
func TestAdminSSHV15PinOnlyExact(t *testing.T) {
	cases := []struct{ host, alias, dest string }{
		{"host.test", "nself-host.test-2222", "u@host.test"},
		{"[2001:db8::1]", "nself-2001-db8--1-2222", "u@2001:db8::1"},
	}
	for _, tc := range cases {
		t.Run(tc.host, func(t *testing.T) {
			compattest.Set(t, true)
			e := newArgvEnv(t, "", &hostKey{alias: tc.alias, pin: true})
			if err := callEntry("verify", "u", tc.host, 2222); err != nil {
				t.Fatal(err)
			}
			mustExist(t, e.scanMark(), "ssh-keyscan must run in v1.5 mode")
			want := append(v15Verify(e.pinFile(), tc.alias), "-p", "2222", "--", tc.dest, "true")
			got := e.argv(t)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("pin-only argv mismatch\n got: %q\nwant: %q", got, want)
			}
			if strings.Contains(strings.Join(got, " "), "accept-new") {
				t.Fatal("accept-new in pin-only argv")
			}
		})
	}
}

// TestAdminSSHV15PinFileWithSpaceIsOneWord pins the quoting: ssh splits
// GlobalKnownHostsFile on whitespace, so a HOME with a space must still reach
// ssh as one double-quoted path in a single argv word, never a bare path.
func TestAdminSSHV15PinFileWithSpaceIsOneWord(t *testing.T) {
	compattest.Set(t, true)
	home := filepath.Join(t.TempDir(), "a b")
	e := newArgvEnv(t, home, &hostKey{alias: "[host.test]:2222"})
	if err := callEntry("verify", "u", "host.test", 2222); err != nil {
		t.Fatal(err)
	}
	wantWord := `GlobalKnownHostsFile="` + filepath.Join(home, ".config", "nself", "deploy_known_hosts") + `"`
	n := 0
	for _, w := range e.argv(t) {
		if strings.HasPrefix(w, "GlobalKnownHostsFile=") {
			n++
			if w != wantWord {
				t.Fatalf("pin word = %q, want %q", w, wantWord)
			}
		}
	}
	if n != 1 {
		t.Fatalf("GlobalKnownHostsFile words = %d, want 1", n)
	}
}

// TestAdminSSHV15RefusesUnquotablePinPath pins that a HOME whose pin file path
// holds a quote, backslash or control character is refused before the host-key
// scan and before ssh runs; the path cannot be carried in one quoted word.
func TestAdminSSHV15RefusesUnquotablePinPath(t *testing.T) {
	for _, name := range []string{`a"b`, `a\b`, "a\tb"} {
		t.Run(strings.NewReplacer("\t", "tab", `"`, "quote", `\`, "backslash").Replace(name), func(t *testing.T) {
			compattest.Set(t, true)
			e := newArgvEnv(t, filepath.Join(t.TempDir(), name), &hostKey{alias: "[host.test]:2222"})
			for _, entry := range entryNames {
				if err := callEntry(entry, "u", "host.test", 2222); err == nil {
					t.Fatalf("%s accepted an unquotable pin path", entry)
				}
			}
			mustNotExist(t, e.argvPath(), "ssh ran with an unquotable pin path")
			mustNotExist(t, e.scanMark(), "host-key scan ran before the pin path was refused")
		})
	}
}
