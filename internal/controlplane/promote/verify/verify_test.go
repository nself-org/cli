package verify

import (
	"context"
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
		{name: "plugin cannot start", want: "E499", post: func(r *release) { r.runErr = errors.New("no such file") }},
		{name: "unreadable report", want: "E497", post: func(r *release) { r.report = "not json" }},
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
	if len(warned) != 1 || !strings.Contains(warned[0], "differs") {
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
	r.git.fetchErr = errors.New("network down")
	if _, err := Verify(context.Background(), r.input()); codeOf(err) != "E493" {
		t.Fatalf("fetch failure: %v, want E493", err)
	}
	// No origin: the fetch is not attempted and operator keys still work.
	r.git.fetchErr, r.git.noOrigin, r.git.fetched = nil, true, 0
	if _, err := Verify(context.Background(), r.input()); err != nil || r.git.fetched != 0 {
		t.Fatalf("no-origin repo: err %v fetched %d", err, r.git.fetched)
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
