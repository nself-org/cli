package verify

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/controlplane/promote"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/sdk/go/v2/signing"
	"github.com/nself-org/cli/sdk/go/v2/signing/signingtest"
)

func codeOf(err error) string {
	var c *errs.CLIError
	if errors.As(err, &c) {
		return c.Code
	}
	return ""
}

// TestPromoteReleaseCodes drives every refusal of the contract and the happy
// path, and pins the exit class of each code.
func TestPromoteReleaseCodes(t *testing.T) {
	cases := []struct {
		name string
		want string
		mut  func(r *release) (payloadType string, s signing.Signer)
		post func(r *release)
	}{
		{name: "valid", want: ""},
		{name: "wrong payloadType before any decode", want: "E493", mut: func(r *release) (string, signing.Signer) {
			r.manifest = map[string]any{"schema": 7}
			return "application/json", r.signer
		}},
		{name: "unknown key id", want: "E493", mut: func(r *release) (string, signing.Signer) {
			_, other := signingtest.NewKey(t, signing.PurposeCIRelease)
			return PayloadType, other
		}},
		{name: "key of purpose plugins", want: "E493", mut: func(r *release) (string, signing.Signer) {
			_, other := signingtest.NewKey(t, signing.PurposePlugins)
			return PayloadType, other
		}},
		{name: "revoked key", want: "E494", post: func(r *release) {
			r.git.files[trustRel+revokedName] = []byte(r.key.ID + "\n")
		}},
		{name: "manifest names another key", want: "E493", mut: func(r *release) (string, signing.Signer) {
			r.manifest["signing"] = map[string]any{"purpose": "ci-release", "key_id": "ci-release-0000000000000000"}
			return PayloadType, r.signer
		}},
		{name: "unknown major", want: "E492", mut: func(r *release) (string, signing.Signer) {
			r.manifest["schema"] = "ci.release-manifest/v2"
			return PayloadType, r.signer
		}},
		{name: "not a manifest schema", want: "E492", mut: func(r *release) (string, signing.Signer) {
			r.manifest["schema"] = "other/v1"
			return PayloadType, r.signer
		}},
		{name: "short revision", want: "E492", mut: func(r *release) (string, signing.Signer) {
			r.manifest["revision"] = "abc"
			return PayloadType, r.signer
		}},
		{name: "revision mismatch", want: "E496", mut: func(r *release) (string, signing.Signer) {
			r.evidence = []byte(`{"revision":"` + otherRev + `"}`)
			return PayloadType, r.signer
		}},
		{name: "evidence is not the pinned file", want: "E497", post: func(r *release) {
			_ = os.WriteFile(filepath.Join(r.dir, EvidenceFile), []byte(`{"revision":"`+testRev+`","x":1}`), 0o600)
		}},
		{name: "stale evidence", want: "E497", post: func(r *release) { r.exit = 10 }},
		{name: "unbound evidence", want: "E497", post: func(r *release) { r.exit = 12 }},
		{name: "invalid evidence", want: "E497", post: func(r *release) { r.exit = 11 }},
		{name: "not release class", want: "E497", post: func(r *release) {
			r.report = `{"data":{"class":"default","result":"pass","quarantined":false}}`
		}},
		{name: "quarantined", want: "E498", post: func(r *release) {
			r.report = `{"data":{"class":"release","result":"pass","quarantined":true}}`
		}},
		{name: "quarantine verdict absent", want: "E497", post: func(r *release) {
			r.report = `{"data":{"class":"release","result":"pass"}}`
		}},
		{name: "plugin cannot start", want: "E499", post: func(r *release) { r.runErr = errors.New("no such file") }},
		{name: "unreadable report", want: "E497", post: func(r *release) { r.report = "not json" }},
		{name: "evidence is not JSON", want: "E497", mut: func(r *release) (string, signing.Signer) {
			r.evidence = []byte("not json")
			return PayloadType, r.signer
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRelease(t)
			pt, signer := PayloadType, r.signer
			if tc.mut != nil {
				pt, signer = tc.mut(r)
			}
			r.write(pt, signer)
			if tc.post != nil {
				tc.post(r)
			}
			m, err := Verify(context.Background(), r.input())
			if got := codeOf(err); got != tc.want {
				t.Fatalf("code = %q (%v), want %q", got, err, tc.want)
			}
			if tc.want == "" && (m == nil || m.Revision != testRev) {
				t.Fatalf("manifest = %+v", m)
			}
			if tc.want == "" && (len(r.runs) != 1 || r.runs[0] != "evidence.json@"+testRev) {
				t.Fatalf("evidence ran as %v, want once at the manifest revision", r.runs)
			}
			if tc.want != "" && tc.want != "E497" && tc.want != "E498" && len(r.runs) != 0 && tc.want != "E499" {
				t.Fatalf("evidence ran before %s: %v", tc.want, r.runs)
			}
		})
	}
}

func TestPromoteReleaseDigestsAndExitClass(t *testing.T) {
	r := newRelease(t)
	r.write(PayloadType, r.signer)
	m, err := Verify(context.Background(), r.input())
	if err != nil {
		t.Fatal(err)
	}
	ok := []promote.DigestRow{{Service: "web", Source: imgAMD64, Target: imgAMD64, Status: "equal"}, {Service: "db", Status: "unverifiable"}}
	if err := CheckDigests(m, ok); err != nil {
		t.Fatalf("listed digest refused: %v", err)
	}
	bad := []promote.DigestRow{{Service: "web", Source: unlistedID, Target: unlistedID, Status: "equal"}}
	if got := codeOf(CheckDigests(m, bad)); got != "E495" {
		t.Fatalf("tampered digest code = %q, want E495", got)
	}
	if got := codeOf(CheckDigests(nil, ok)); got != "E499" {
		t.Fatalf("no manifest code = %q, want E499", got)
	}
	for code, exit := range map[string]int{"E492": 1, "E493": 3, "E494": 3, "E495": 1, "E496": 1, "E497": 1, "E498": 1, "E499": 1} {
		if e, found := errs.Registry[code]; !found || e.Exit != exit {
			t.Errorf("%s: registered %+v, want exit %d", code, e, exit)
		}
	}
}

// TestPromoteReleaseTrustSources proves only committed and operator keys are
// trusted: a key that exists only in the working tree refuses with E493.
func TestPromoteReleaseTrustSources(t *testing.T) {
	r := newRelease(t)
	r.git.files = map[string][]byte{}
	if err := os.MkdirAll(filepath.Join(r.repo, ".nself", "trust"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.repo, trustRel+keysName), keysLine(r.key), 0o600); err != nil {
		t.Fatal(err)
	}
	r.write(PayloadType, r.signer)
	var warned []string
	in := r.input()
	in.Warn = func(s string) { warned = append(warned, s) }
	if _, err := Verify(context.Background(), in); codeOf(err) != "E493" {
		t.Fatalf("working-tree-only key: %v, want E493", err)
	}
	if len(warned) != 1 || !strings.Contains(warned[0], "differs") || !strings.Contains(warned[0], "cccccccccccc") {
		t.Fatalf("warnings = %v, want one differing-copy warning", warned)
	}
	// The same key in the operator trust directory is trusted.
	dir := filepath.Join(r.home, ".nself", "ci", "trust")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, keysName), keysLine(r.key), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(context.Background(), r.input()); err != nil {
		t.Fatalf("operator-trusted key refused: %v", err)
	}
	// A failed fetch is fatal when origin exists.
	r.git.pinErr = errors.New("network down")
	if _, err := Verify(context.Background(), r.input()); codeOf(err) != "E493" {
		t.Fatalf("fetch failure: %v, want E493", err)
	}
	// No origin: the fetch is not attempted and operator keys still work.
	r.git.pinErr, r.git.noOrigin, r.git.pinned = nil, true, 0
	var noOriginWarns []string
	in2 := r.input()
	in2.Warn = func(s string) { noOriginWarns = append(noOriginWarns, s) }
	if _, err := Verify(context.Background(), in2); err != nil || r.git.pinned != 0 {
		t.Fatalf("no-origin repo: err %v pinned %d", err, r.git.pinned)
	}
	if len(noOriginWarns) != 1 || !strings.Contains(noOriginWarns[0], "no origin") {
		t.Fatalf("no-origin warnings = %v, want one saying revocations were not consulted", noOriginWarns)
	}
}

func TestPromoteReleaseMissingFiles(t *testing.T) {
	r := newRelease(t)
	if _, err := Verify(context.Background(), r.input()); codeOf(err) != "E492" {
		t.Fatalf("missing manifest: %v, want E492", err)
	}
	if err := os.WriteFile(filepath.Join(r.dir, ManifestFile), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(context.Background(), r.input()); codeOf(err) != "E492" {
		t.Fatalf("garbage manifest: %v, want E492", err)
	}
	r.write(PayloadType, r.signer)
	if err := os.Remove(filepath.Join(r.dir, EvidenceFile)); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(context.Background(), r.input()); codeOf(err) != "E497" {
		t.Fatalf("missing evidence: %v, want E497", err)
	}
}

// TestPromoteReleaseHomeMustBeAbsolute: an empty or relative home would make a
// working-tree file a trust root, so it refuses before anything is read.
func TestPromoteReleaseHomeMustBeAbsolute(t *testing.T) {
	for _, home := range []string{"", "rel/home", "."} {
		r := newRelease(t)
		r.write(PayloadType, r.signer)
		in := r.input()
		in.Home = home
		if _, err := Verify(context.Background(), in); codeOf(err) != "E493" {
			t.Errorf("home %q: %v, want E493", home, err)
		}
		if len(r.runs) != 0 {
			t.Errorf("home %q: evidence ran: %v", home, r.runs)
		}
	}
}

// TestPromoteReleaseTrustFileFailures: unreadable, malformed and unfetchable
// trust material refuses with E493 and never reaches the evidence.
func TestPromoteReleaseTrustFileFailures(t *testing.T) {
	cases := []struct {
		name string
		set  func(r *release)
	}{
		{"operator keys path is a directory", func(r *release) {
			if err := os.MkdirAll(filepath.Join(r.home, ".nself", "ci", "trust", keysName), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{"malformed committed keys", func(r *release) { r.git.files[trustRel+keysName] = []byte("garbage line\n") }},
		{"malformed committed revoked list", func(r *release) { r.git.files[trustRel+revokedName] = []byte("not-a-key-id\n") }},
		{"malformed operator revoked list", func(r *release) {
			dir := filepath.Join(r.home, ".nself", "ci", "trust")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, revokedName), []byte("x y z\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"git show fails", func(r *release) { r.git.showErr = errors.New("object missing") }},
		{"git cannot tell about origin", func(r *release) { r.git.originErr = errors.New("git missing") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRelease(t)
			r.write(PayloadType, r.signer)
			tc.set(r)
			if _, err := Verify(context.Background(), r.input()); codeOf(err) != "E493" {
				t.Fatalf("err = %v, want E493", err)
			}
			if len(r.runs) != 0 {
				t.Fatalf("evidence ran: %v", r.runs)
			}
		})
	}
}

// TestPromoteReleaseNoWarningWhenWorkingCopyMatches: identical copies are silent.
func TestPromoteReleaseNoWarningWhenWorkingCopyMatches(t *testing.T) {
	r := newRelease(t)
	dir := filepath.Join(r.repo, ".nself", "trust")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, keysName), r.git.files[trustRel+keysName], 0o600); err != nil {
		t.Fatal(err)
	}
	trust, err := LoadTrust(context.Background(), r.git, r.home, r.repo)
	if err != nil || len(trust.Warnings) != 0 || len(trust.Keys) != 1 {
		t.Fatalf("trust = %+v err = %v, want one key and no warnings", trust, err)
	}
	// Both files differing still gives exactly one combined warning.
	r.git.files[trustRel+revokedName] = []byte(r.key.ID + "\n")
	if err := os.WriteFile(filepath.Join(dir, keysName), []byte("# other\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, revokedName), []byte("# other\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	trust, err = LoadTrust(context.Background(), r.git, r.home, r.repo)
	if err != nil || len(trust.Warnings) != 1 || !strings.Contains(trust.Warnings[0], keysName) || !strings.Contains(trust.Warnings[0], revokedName) {
		t.Fatalf("warnings = %v err = %v, want one warning naming both files", trust.Warnings, err)
	}
}

func TestReadBoundedLimit(t *testing.T) {
	dir := t.TempDir()
	at := filepath.Join(dir, "at")
	if err := os.WriteFile(at, make([]byte, maxFile), 0o600); err != nil {
		t.Fatal(err)
	}
	if data, err := readBounded(at); err != nil || len(data) != maxFile {
		t.Fatalf("a file of exactly %d bytes: len %d err %v, want it read whole", maxFile, len(data), err)
	}
	over := filepath.Join(dir, "over")
	if err := os.WriteFile(over, make([]byte, maxFile+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readBounded(over); err == nil || !strings.Contains(err.Error(), "larger") {
		t.Fatalf("a file one byte over the limit: err %v, want a size refusal", err)
	}
}

// TestPromoteReleaseOversizeTrustFile: an oversize file at a trust path cannot
// exhaust the verifier.
func TestPromoteReleaseOversizeTrustFile(t *testing.T) {
	r := newRelease(t)
	r.write(PayloadType, r.signer)
	dir := filepath.Join(r.home, ".nself", "ci", "trust")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, keysName), make([]byte, trustFileLimit+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(context.Background(), r.input()); codeOf(err) != "E493" {
		t.Fatalf("oversize trust file: %v, want E493", err)
	}
}

func TestPromoteReleaseTrailingPayloadContent(t *testing.T) {
	r := newRelease(t)
	r.write(PayloadType, r.signer)
	payload, _ := json.Marshal(r.manifest)
	if _, err := decodeManifest(append(payload, []byte(` {"x":1}`)...)); codeOf(err) != "E492" {
		t.Fatalf("trailing JSON value: %v, want E492", err)
	}
	if _, err := decodeManifest(append(payload, []byte("garbage")...)); codeOf(err) != "E492" {
		t.Fatalf("trailing garbage: %v, want E492", err)
	}
}

func TestReadTrustFileLimit(t *testing.T) {
	dir := t.TempDir()
	at := filepath.Join(dir, "at")
	if err := os.WriteFile(at, make([]byte, trustFileLimit), 0o600); err != nil {
		t.Fatal(err)
	}
	if data, err := readTrustFile(at); err != nil || len(data) != trustFileLimit {
		t.Fatalf("file of exactly the limit: len %d err %v", len(data), err)
	}
	over := filepath.Join(dir, "over")
	if err := os.WriteFile(over, make([]byte, trustFileLimit+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readTrustFile(over); err == nil || !strings.Contains(err.Error(), "larger") {
		t.Fatalf("file one byte over: %v, want a size refusal", err)
	}
	if _, err := readTrustFile(dir); err == nil || !strings.Contains(err.Error(), "regular") {
		t.Fatalf("a directory: %v, want a regular-file refusal", err)
	}
}

func TestSameRegularFile(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	for _, p := range []string{a, b} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	infoA, err := os.Lstat(a)
	if err != nil {
		t.Fatal(err)
	}
	fa, _ := os.Open(a)
	fb, _ := os.Open(b)
	fd, _ := os.Open(dir)
	defer func() { _ = fa.Close(); _ = fb.Close(); _ = fd.Close() }()
	if err := sameRegularFile(infoA, fa); err != nil {
		t.Fatalf("the same file: %v", err)
	}
	if err := sameRegularFile(infoA, fb); err == nil {
		t.Fatal("a different file passed as the same file")
	}
	infoD, _ := os.Lstat(dir)
	if err := sameRegularFile(infoD, fd); err == nil {
		t.Fatal("a directory passed as a regular file")
	}
}

// TestPromoteReleaseDirectoryFIFOIsRefused: a FIFO or symlink planted as a
// release file cannot hang or redirect the verifier (posix tests cover FIFO).
func TestPromoteReleaseDirectorySymlinkIsRefused(t *testing.T) {
	r := newRelease(t)
	r.write(PayloadType, r.signer)
	real := filepath.Join(r.dir, "real.json")
	if err := os.Rename(filepath.Join(r.dir, ManifestFile), real); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(r.dir, ManifestFile)); err != nil {
		t.Skipf("no symlink support: %v", err)
	}
	if _, err := Verify(context.Background(), r.input()); codeOf(err) != "E492" {
		t.Fatalf("symlinked manifest: %v, want E492", err)
	}
}

// TestPromoteReleaseOriginPin: an operator-pinned origin URL must match the
// project's effective origin; absent, empty or unreadable pins behave as set.
func TestPromoteReleaseOriginPin(t *testing.T) {
	cases := []struct {
		name string
		pin  string // "" means no file
		url  string
		want string
	}{
		{"no pin file", "", "https://evil.example/x.git", ""},
		{"match", "https://github.com/o/r.git\n", "https://github.com/o/r.git", ""},
		{"match with comment and padding", "# where trust lives\n\n  https://github.com/o/r.git  \n", " https://github.com/o/r.git\n", ""},
		{"redirected origin", "https://github.com/o/r.git\n", "https://evil.example/o/r.git", "E493"},
		{"only comments", "# nothing\n", "https://github.com/o/r.git", "E493"},
		{"empty file", "\n", "https://github.com/o/r.git", "E493"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRelease(t)
			r.write(PayloadType, r.signer)
			r.git.originURL = tc.url
			if tc.pin != "" {
				dir := filepath.Join(r.home, ".nself", "ci", "trust")
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, originPinName), []byte(tc.pin), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			_, err := Verify(context.Background(), r.input())
			if codeOf(err) != tc.want {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			if tc.want != "" && (r.git.pinned != 0 || len(r.runs) != 0) {
				t.Fatalf("trust was read (pinned %d) or evidence ran (%v) despite the origin mismatch", r.git.pinned, r.runs)
			}
		})
	}
}
