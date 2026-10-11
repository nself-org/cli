package controlplane

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

// skipOnWindows skips tests that rely on Unix shell-script fakery (PATH override
// with a .sh fake binary) or SSH hostname resolution. Windows doesn't honour
// shebang scripts, so the fake SSH doesn't intercept the call and the real SSH
// tries to resolve the hostname, failing with "No such host is known."
func skipOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("skipped on Windows: requires Unix shell-script PATH override")
	}
}

// fakeHostKeyLine is a synthetic ED25519 known_hosts line (a three-byte blob),
// never a real host key.
const fakeHostKeyLine = "pinned.example ssh-ed25519 AQID"

// stubHostKeyPin lets v1.5 strict host-key checks (P7-DEPL-13) pass offline.
// It writes ssh-keyscan and ssh-keygen fakes into bin, which the caller has
// already put first on PATH, that offer and look up one synthetic key, and pins
// that key in the deploy pin file under a temporary HOME. v1.4 never runs
// either tool, so the same test body holds in both compat modes.
func stubHostKeyPin(t *testing.T, bin string) {
	t.Helper()
	for name, body := range map[string]string{
		"ssh-keyscan": "#!/bin/sh\nprintf '%s\\n' '" + fakeHostKeyLine + "'\n",
		"ssh-keygen":  "#!/bin/sh\nif [ -f \"$4\" ]; then /bin/cat \"$4\"; fi\n",
	} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o700); err != nil {
			t.Fatalf("write fake %s: %v", name, err)
		}
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir on Windows
	pin := DeployKnownHostsPath()
	if err := os.MkdirAll(filepath.Dir(pin), 0o700); err != nil {
		t.Fatalf("pin dir: %v", err)
	}
	if err := os.WriteFile(pin, []byte(fakeHostKeyLine+"\n"), 0o600); err != nil {
		t.Fatalf("write pin file: %v", err)
	}
}

// hostKeyModes pairs each compat mode with the StrictHostKeyChecking value the
// ssh argv carries for an unclassified (prod-class) target once its key is
// pinned: accept-new in v1.4, yes in v1.5.
var hostKeyModes = []struct {
	name   string
	v15    bool
	policy string
}{{"v1.4", false, "StrictHostKeyChecking=accept-new"}, {"v1.5", true, "StrictHostKeyChecking=yes"}}

// TestDrainHelperPresent verifies that Drain returns DrainOK when the nself-lb
// helper is present and exits 0.
func TestDrainHelperPresent(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()

	keyFile := filepath.Join(dir, "id_ed25519")
	if err := os.WriteFile(keyFile, []byte("fake key"), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	t.Setenv("NSELF_SSH_KEY_LB", keyFile)

	// Fake SSH that exits 0 unconditionally.
	fakeSSH := filepath.Join(dir, "ssh")
	if err := os.WriteFile(fakeSSH, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("write fake ssh: %v", err)
	}
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+":"+origPath)
	stubHostKeyPin(t, dir)

	srv := Server{
		Name:       "lb-01",
		Host:       "ubuntu@lb.example.com",
		SSHKeyRef:  "NSELF_SSH_KEY_LB",
		RemotePath: "/opt/nself",
	}

	result, err := Drain(context.Background(), srv, "myapp")
	if err != nil {
		t.Fatalf("Drain: unexpected error: %v", err)
	}
	if result != DrainOK {
		t.Errorf("Drain: got %v, want DrainOK", result)
	}
}

// TestEnableHelperPresent verifies that Enable returns DrainOK when the helper
// exits 0.
func TestEnableHelperPresent(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()

	keyFile := filepath.Join(dir, "id_ed25519")
	if err := os.WriteFile(keyFile, []byte("fake key"), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	t.Setenv("NSELF_SSH_KEY_ENABLE", keyFile)

	fakeSSH := filepath.Join(dir, "ssh")
	if err := os.WriteFile(fakeSSH, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("write fake ssh: %v", err)
	}
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+":"+origPath)
	stubHostKeyPin(t, dir)

	srv := Server{
		Name:       "lb-01",
		Host:       "ubuntu@lb.example.com",
		SSHKeyRef:  "NSELF_SSH_KEY_ENABLE",
		RemotePath: "/opt/nself",
	}

	result, err := Enable(context.Background(), srv, "myapp")
	if err != nil {
		t.Fatalf("Enable: unexpected error: %v", err)
	}
	if result != DrainOK {
		t.Errorf("Enable: got %v, want DrainOK", result)
	}
}

// TestDrainHelperAbsent verifies that Drain returns DrainHelperAbsent (and nil
// error) when SSH exits 127 (command not found on remote).
func TestDrainHelperAbsent(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()

	keyFile := filepath.Join(dir, "id_ed25519")
	if err := os.WriteFile(keyFile, []byte("fake key"), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	t.Setenv("NSELF_SSH_KEY_ABSENT_LB", keyFile)

	// Fake SSH that exits 127 — "command not found" simulation.
	fakeSSH := filepath.Join(dir, "ssh")
	if err := os.WriteFile(fakeSSH, []byte("#!/bin/sh\nexit 127\n"), 0o700); err != nil {
		t.Fatalf("write fake ssh: %v", err)
	}
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+":"+origPath)
	stubHostKeyPin(t, dir)

	srv := Server{
		Name:       "lb-absent",
		Host:       "ubuntu@lb.example.com",
		SSHKeyRef:  "NSELF_SSH_KEY_ABSENT_LB",
		RemotePath: "/opt/nself",
	}

	result, err := Drain(context.Background(), srv, "myapp")
	if err != nil {
		t.Fatalf("Drain (helper absent): expected nil error, got: %v", err)
	}
	if result != DrainHelperAbsent {
		t.Errorf("Drain (helper absent): got %v, want DrainHelperAbsent", result)
	}
}

// TestEnableHelperAbsent verifies the same graceful-degrade for Enable.
func TestEnableHelperAbsent(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()

	keyFile := filepath.Join(dir, "id_ed25519")
	if err := os.WriteFile(keyFile, []byte("fake key"), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	t.Setenv("NSELF_SSH_KEY_ABSENT_EN", keyFile)

	fakeSSH := filepath.Join(dir, "ssh")
	if err := os.WriteFile(fakeSSH, []byte("#!/bin/sh\nexit 127\n"), 0o700); err != nil {
		t.Fatalf("write fake ssh: %v", err)
	}
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+":"+origPath)
	stubHostKeyPin(t, dir)

	srv := Server{
		Name:       "lb-absent",
		Host:       "ubuntu@lb.example.com",
		SSHKeyRef:  "NSELF_SSH_KEY_ABSENT_EN",
		RemotePath: "/opt/nself",
	}

	result, err := Enable(context.Background(), srv, "myapp")
	if err != nil {
		t.Fatalf("Enable (helper absent): expected nil error, got: %v", err)
	}
	if result != DrainHelperAbsent {
		t.Errorf("Enable (helper absent): got %v, want DrainHelperAbsent", result)
	}
}

// TestDrainFailedNonZero verifies that non-127 SSH failures return DrainFailed
// with a non-nil error.
func TestDrainFailedNonZero(t *testing.T) {
	dir := t.TempDir()

	keyFile := filepath.Join(dir, "id_ed25519")
	if err := os.WriteFile(keyFile, []byte("fake key"), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	t.Setenv("NSELF_SSH_KEY_FAIL_LB", keyFile)

	// Fake SSH that exits 1 — generic failure.
	fakeSSH := filepath.Join(dir, "ssh")
	if err := os.WriteFile(fakeSSH, []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatalf("write fake ssh: %v", err)
	}
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+":"+origPath)
	stubHostKeyPin(t, dir)

	srv := Server{
		Name:       "lb-fail",
		Host:       "ubuntu@lb.example.com",
		SSHKeyRef:  "NSELF_SSH_KEY_FAIL_LB",
		RemotePath: "/opt/nself",
	}

	result, err := Drain(context.Background(), srv, "myapp")
	if err == nil {
		t.Fatal("Drain (failed): expected non-nil error, got nil")
	}
	if result != DrainFailed {
		t.Errorf("Drain (failed): got %v, want DrainFailed", result)
	}
}

// TestDrainMissingSSHKeyRef verifies that a missing SSHKeyRef env var returns
// DrainFailed immediately without calling SSH.
func TestDrainMissingSSHKeyRef(t *testing.T) {
	srv := Server{
		Name:       "lb-nokey",
		Host:       "ubuntu@lb.example.com",
		SSHKeyRef:  "NSELF_SSH_KEY_DEFINITELY_NOT_SET_XYZ",
		RemotePath: "/opt/nself",
	}

	// Ensure the var is unset.
	t.Setenv("NSELF_SSH_KEY_DEFINITELY_NOT_SET_XYZ", "")

	result, err := Drain(context.Background(), srv, "myapp")
	if err == nil {
		t.Fatal("Drain (no key ref): expected non-nil error, got nil")
	}
	if result != DrainFailed {
		t.Errorf("Drain (no key ref): got %v, want DrainFailed", result)
	}
}

// TestLBHelperArgVector verifies that runLBHelper passes the expected SSH args
// including the correct remote helper invocation and EIE hygiene (no sudo,
// no osascript), in both compat modes.
func TestLBHelperArgVector(t *testing.T) {
	skipOnWindows(t)
	for _, m := range hostKeyModes {
		t.Run(m.name, func(t *testing.T) {
			compattest.Set(t, m.v15)
			dir := t.TempDir()

			keyFile := filepath.Join(dir, "id_ed25519")
			if err := os.WriteFile(keyFile, []byte("fake key"), 0o600); err != nil {
				t.Fatalf("write key: %v", err)
			}
			t.Setenv("NSELF_SSH_KEY_ARGVEC", keyFile)

			// Fake SSH that captures argv to a file.
			argsFile := filepath.Join(dir, "args.txt")
			script := `#!/bin/sh
echo "$@" > ` + argsFile + `
exit 0
`
			fakeSSH := filepath.Join(dir, "ssh")
			if err := os.WriteFile(fakeSSH, []byte(script), 0o700); err != nil {
				t.Fatalf("write fake ssh: %v", err)
			}
			t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
			stubHostKeyPin(t, dir)

			srv := Server{
				Name:       "lb-vec",
				Host:       "ubuntu@lb-vec.example.com",
				SSHKeyRef:  "NSELF_SSH_KEY_ARGVEC",
				RemotePath: "/opt/nself",
			}

			if _, err := Drain(context.Background(), srv, "webapp"); err != nil {
				t.Fatalf("Drain: %v", err)
			}

			argsRaw, err := os.ReadFile(argsFile)
			if err != nil {
				t.Fatalf("read args file: %v", err)
			}
			args := string(argsRaw)

			// Verify required SSH flags.
			for _, want := range []string{
				"-i", "BatchMode=yes", m.policy, "ConnectTimeout=5", "ForwardAgent=no",
				"ubuntu@lb-vec.example.com",
				"/opt/nself/bin/nself-lb", "drain", "webapp",
			} {
				if !containsFold(args, want) {
					t.Errorf("expected arg %q in SSH invocation %q", want, args)
				}
			}

			// Verify forbidden privileged commands are absent.
			for _, forbidden := range []string{"sudo", "osascript", "pfctl", "launchctl"} {
				if containsFold(args, forbidden) {
					t.Errorf("forbidden command %q found in SSH invocation %q", forbidden, args)
				}
			}
		})
	}
}

// TestLBHelperInjectionPayloadIsDistinctToken verifies that a shell-metacharacter
// payload in appName is passed as a separate argv token to SSH rather than
// being concatenated into a shell string. The fake SSH script captures "$@"
// (space-joined args) so we can confirm the payload appears verbatim — not
// interpreted — and that the helper binary path is separate from the payload.
func TestLBHelperInjectionPayloadIsDistinctToken(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()

	keyFile := filepath.Join(dir, "id_ed25519")
	if err := os.WriteFile(keyFile, []byte("fake key"), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	t.Setenv("NSELF_SSH_KEY_INJECT", keyFile)

	// Fake SSH: write each positional arg on its own line so we can check
	// that the payload was NOT shell-expanded.
	argsFile := filepath.Join(dir, "args.txt")
	script := `#!/bin/sh
for a in "$@"; do echo "$a"; done > ` + argsFile + `
exit 0
`
	fakeSSH := filepath.Join(dir, "ssh")
	if err := os.WriteFile(fakeSSH, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake ssh: %v", err)
	}
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+":"+origPath)
	stubHostKeyPin(t, dir)

	srv := Server{
		Name:       "lb-inject",
		Host:       "ubuntu@lb-inject.example.com",
		SSHKeyRef:  "NSELF_SSH_KEY_INJECT",
		RemotePath: "/opt/nself",
	}

	// A classic injection payload — if concatenated into a shell string,
	// this would execute "touch /tmp/pwned" on the remote host.
	injectionPayload := "x; touch /tmp/pwned"
	if _, err := Drain(context.Background(), srv, injectionPayload); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	argsRaw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read args file: %v", err)
	}

	// The payload must appear verbatim as one of the per-line tokens.
	argsStr := string(argsRaw)
	if !containsFold(argsStr, injectionPayload) {
		t.Errorf("injection payload %q not found verbatim in SSH args:\n%s", injectionPayload, argsStr)
	}

	// The helper binary path must be a separate token from the payload.
	// Verify that the last line is the payload (not "helperBin drain x; touch...").
	lines := nonEmptyLines(argsStr)
	if len(lines) == 0 {
		t.Fatal("no args captured")
	}
	lastToken := lines[len(lines)-1]
	if lastToken != injectionPayload {
		t.Errorf("last token: got %q, want the injection payload %q (should be separate from action token)", lastToken, injectionPayload)
	}
}
