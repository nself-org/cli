package verify

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func pinFile(home string) string {
	return filepath.Join(home, ".nself", "ci", "trust", originPinName)
}

// D-1216: the first verified release pins the origin, and the pin then holds.
func TestVerifyPinsOriginOnFirstUse(t *testing.T) {
	r := newRelease(t)
	r.write(PayloadType, r.signer)
	r.git.originURL = "git@github.com:o/r.git"
	var notes, warns []string
	in := r.input()
	in.PinOnFirstUse, in.Notice, in.Warn = true, func(s string) { notes = append(notes, s) }, func(s string) { warns = append(warns, s) }
	if _, err := Verify(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(pinFile(r.home))
	if err != nil || !strings.Contains(string(raw), "\ngit@github.com:o/r.git\n") {
		t.Fatalf("pin file = %q, %v", raw, err)
	}
	if info, _ := os.Stat(pinFile(r.home)); info.Mode().Perm() != 0o600 {
		t.Errorf("pin file mode %v, want 0600", info.Mode().Perm())
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "git@github.com:o/r.git") || len(warns) != 0 {
		t.Errorf("notes %q warns %q", notes, warns)
	}
	if left, _ := filepath.Glob(filepath.Join(r.home, ".nself", "ci", "trust", ".origin-*")); len(left) != 0 {
		t.Errorf("temp files left: %v", left)
	}
	// The pin now holds: a redirected origin refuses before trust is read.
	r.git.originURL = "https://evil.example/o/r.git"
	r.git.pinned = 0
	if _, err := Verify(context.Background(), in); codeOf(err) != "E493" || r.git.pinned != 0 {
		t.Fatalf("redirected origin after the first-use pin: %v (trust reads %d)", err, r.git.pinned)
	}
	// A second verified release with the pinned origin keeps the file as it is.
	r.git.originURL = "git@github.com:o/r.git"
	if _, err := Verify(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if again, _ := os.ReadFile(pinFile(r.home)); string(again) != string(raw) {
		t.Errorf("pin file rewritten: %q", again)
	}
}

func TestVerifyFirstUsePinIsOptIn(t *testing.T) {
	r := newRelease(t)
	r.write(PayloadType, r.signer)
	r.git.originURL = "git@github.com:o/r.git"
	if _, err := Verify(context.Background(), r.input()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(pinFile(r.home)); !os.IsNotExist(err) {
		t.Fatalf("a library caller wrote the pin: %v", err)
	}
}

func TestVerifyFailureDoesNotPin(t *testing.T) {
	r := newRelease(t)
	r.exit = 1 // evidence plugin refuses
	r.write(PayloadType, r.signer)
	r.git.originURL = "git@github.com:o/r.git"
	in := r.input()
	in.PinOnFirstUse = true
	if _, err := Verify(context.Background(), in); err == nil {
		t.Fatal("want a refusal")
	}
	if _, err := os.Stat(pinFile(r.home)); !os.IsNotExist(err) {
		t.Fatalf("pinned an origin on a failed verification: %v", err)
	}
}

func TestPinOriginOnFirstUseCases(t *testing.T) {
	cases := []struct {
		name, url string
		noOrigin  bool
		existing  string
		wantPin   bool
		wantErr   bool
	}{
		{name: "https", url: "https://github.com/o/r.git", wantPin: true},
		{name: "ssh url", url: "ssh://git@host/o/r.git", wantPin: true},
		{name: "no origin", noOrigin: true},
		{name: "existing pin untouched", url: "https://github.com/o/r.git", existing: "https://other.example/x.git\n"},
		{name: "token in https userinfo", url: "https://TOKEN@github.com/o/r.git", wantErr: true},
		{name: "password in userinfo", url: "https://u:p@github.com/o/r.git", wantErr: true},
		{name: "password in ssh userinfo", url: "ssh://u:p@host/r.git", wantErr: true},
		{name: "empty url", url: "", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			if tc.existing != "" {
				if err := os.MkdirAll(filepath.Dir(pinFile(home)), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(pinFile(home), []byte(tc.existing), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			url, pinned, err := PinOriginOnFirstUse(context.Background(), &fakeGit{noOrigin: tc.noOrigin, originURL: tc.url}, home)
			if pinned != tc.wantPin || (err != nil) != tc.wantErr || (pinned && url != tc.url) {
				t.Fatalf("url %q pinned %v err %v", url, pinned, err)
			}
			raw, rerr := os.ReadFile(pinFile(home))
			switch {
			case tc.existing != "":
				if string(raw) != tc.existing {
					t.Errorf("existing pin replaced: %q", raw)
				}
			case !tc.wantPin && rerr == nil:
				t.Errorf("pin file written: %q", raw)
			}
			if err != nil && strings.Contains(err.Error(), "TOKEN") {
				t.Errorf("credential in the error: %v", err)
			}
		})
	}
	if _, pinned, err := PinOriginOnFirstUse(context.Background(), &fakeGit{originURL: "https://github.com/o/r.git"}, "relative"); pinned || err != nil {
		t.Errorf("a relative home must be a no-op: %v %v", pinned, err)
	}
}
