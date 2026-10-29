package admin

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// v15Verify returns the exact v1.5 VerifySSHKey argv up to the destination.
func v15Verify(pin, alias string) []string {
	args := []string{
		"-o", "BatchMode=yes", "-o", "ConnectTimeout=10", "-o", "ServerAliveInterval=30",
		"-o", "StrictHostKeyChecking=yes",
		"-o", "ControlMaster=no", "-o", "ControlPath=none",
		"-o", `GlobalKnownHostsFile="` + pin + `"`,
	}
	if alias != "" {
		args = append(args, "-o", "HostKeyAlias="+alias)
	}
	return args
}

// TestAdminSSHV15PinOnlyExact pins the pin-file-only path: known_hosts is
// absent, the key lives in the nSelf pin file under the nself+<host>+<port>
// alias (PinAlias), and the argv adds HostKeyAlias with the quoted pin file. Covers a DNS
// name and a bracketed IPv6 literal (its colons stay in the alias).
func TestAdminSSHV15PinOnlyExact(t *testing.T) {
	cases := []struct{ host, alias, dest string }{
		{"host.test", "nself+host.test+2222", "u@host.test"},
		{"[2001:db8::1]", "nself+2001:db8::1+2222", "u@2001:db8::1"},
	}
	for _, tc := range cases {
		t.Run(tc.host, func(t *testing.T) {
			compattest.Set(t, true)
			e := newArgvEnv(t, "", &hostKey{alias: tc.alias, pin: true})
			if err := callEntry("verify", "u", tc.host, 2222); err != nil {
				t.Fatal(err)
			}
			mustExist(t, e.scanMark(), "SSH key capture must run in v1.5 mode")
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

// TestAdminPinFileOption pins the pure helper: a path is double-quoted into one
// option value, a Windows-shaped path already holding forward slashes passes
// through (the ToSlash result on Windows), and a quote, backslash or control
// character is refused. Backslash refusal applies to the converted path, so on
// POSIX a real backslash is still refused.
func TestAdminPinFileOption(t *testing.T) {
	cases := []struct {
		name, in, want string
		wantErr        bool
	}{
		{"posix plain", "/home/u/.config/nself/deploy_known_hosts", `GlobalKnownHostsFile="/home/u/.config/nself/deploy_known_hosts"`, false},
		{"posix space", "/Users/a b/.config/nself/deploy_known_hosts", `GlobalKnownHostsFile="/Users/a b/.config/nself/deploy_known_hosts"`, false},
		{"windows slashed", "C:/Users/a b/.config/nself/deploy_known_hosts", `GlobalKnownHostsFile="C:/Users/a b/.config/nself/deploy_known_hosts"`, false},
		{"quote", `/Users/a"b/kh`, "", true},
		{"newline", "/Users/a\nb/kh", "", true},
		{"tab", "/Users/a\tb/kh", "", true},
		{"nul", "/Users/a\x00b/kh", "", true},
	}
	// ToSlash is the identity on POSIX, where a backslash is a real path byte and
	// is refused; on Windows it is the separator and is converted.
	if runtime.GOOS == "windows" {
		cases = append(cases, struct {
			name, in, want string
			wantErr        bool
		}{"windows native", `C:\Users\a b\.config\nself\deploy_known_hosts`, `GlobalKnownHostsFile="C:/Users/a b/.config/nself/deploy_known_hosts"`, false})
	} else {
		cases = append(cases, struct {
			name, in, want string
			wantErr        bool
		}{"backslash", `/Users/a\b/kh`, "", true})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := remote.GlobalKnownHostsFileOption(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.want != "" {
				absolute, absErr := filepath.Abs(tc.in)
				if absErr != nil {
					t.Fatal(absErr)
				}
				tc.want = `GlobalKnownHostsFile="` + filepath.ToSlash(absolute) + `"`
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestAdminSSHV15IPPort22PinOnly verifies that an IP literal on port 22 emits
// HostKeyAlias=nself+<ip>+22 so ssh checks the pin alias rather than the bare IP
// (the pin file refuses an IP as an alias). A pin under the shared alias and an
// older release's nself-<ip>-22 pin (migrated once) both reach that alias.
func TestAdminSSHV15IPPort22PinOnly(t *testing.T) {
	for _, alias := range []string{"nself+203.0.113.7+22", "nself-203.0.113.7-22"} {
		t.Run(alias, func(t *testing.T) {
			compattest.Set(t, true)
			e := newArgvEnv(t, "", &hostKey{alias: alias, pin: true})
			if err := callEntry("verify", "u", "203.0.113.7", 22); err != nil {
				t.Fatal(err)
			}
			got := e.argv(t)
			found := false
			for i, w := range got {
				if w == "HostKeyAlias=nself+203.0.113.7+22" {
					found = true
					if i == 0 || got[i-1] != "-o" {
						t.Fatalf("HostKeyAlias not preceded by -o: %q", got)
					}
					break
				}
			}
			if !found {
				t.Fatalf("missing HostKeyAlias for IP port 22 pin-only: %q", got)
			}
		})
	}
}

// TestAdminSSHV15Non22PinAliasFallback verifies that for a non-22 port, the
// admin connect path finds a pin under either the shared alias or the legacy
// v1.4.13 alias.
func TestAdminSSHV15Non22PinAliasFallback(t *testing.T) {
	cases := []struct{ name, alias string }{
		{"new-alias", "nself+host.test+2222"},
		{"legacy-alias", "nself-host.test-2222"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			compattest.Set(t, true)
			e := newArgvEnv(t, "", &hostKey{alias: tc.alias, pin: true})
			if err := callEntry("verify", "u", "host.test", 2222); err != nil {
				t.Fatal(err)
			}
			if joined := strings.Join(e.argv(t), " "); !strings.Contains(joined, "HostKeyAlias=nself+host.test+2222") {
				t.Fatalf("missing HostKeyAlias for %s: %s", tc.name, joined)
			}
		})
	}
}

// TestAdminSSHV15LegacyPinMigrates is D-0463: a key pinned by an older release
// under its own alias (deploy's explicit host:22 form, the host:port form) is
// found, moved to the shared remote.PinAlias once, and the legacy line stays.
func TestAdminSSHV15LegacyPinMigrates(t *testing.T) {
	cases := []struct {
		name, legacy, want string
		port               int
		wantAlias          bool
	}{
		{"explicit 22 from deploy", "nself-host.test-22", "host.test", 22, false},
		{"custom port", "nself-host.test-2222", "nself+host.test+2222", 2222, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			compattest.Set(t, true)
			e := newArgvEnv(t, "", &hostKey{alias: tc.legacy, pin: true})
			if err := callEntry("verify", "u", "host.test", tc.port); err != nil {
				t.Fatalf("legacy pin was refused: %v", err)
			}
			joined := strings.Join(e.argv(t), " ")
			if got := strings.Contains(joined, "HostKeyAlias="+tc.want); got != tc.wantAlias {
				t.Errorf("HostKeyAlias=%s present = %v, want %v: %s", tc.want, got, tc.wantAlias, joined)
			}
			pins, err := os.ReadFile(e.pinFile())
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(pins), tc.want+" ") || !strings.Contains(string(pins), tc.legacy+" ") {
				t.Errorf("pin file lacks the new alias %q or lost the legacy one:\n%s", tc.want, pins)
			}
		})
	}
}
