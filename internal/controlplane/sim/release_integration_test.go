//go:build integration

package sim_test

// P7-DEPL-19 simulation: a release that fails verification must stop the
// promotion before the target host sees a single snapshot or write command.
// One sshd host stands in for the target; its commands are recorded.

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/controlplane/promote"
	"github.com/nself-org/cli/internal/controlplane/promote/verify"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/sdk/go/v2/signing"
	"github.com/nself-org/cli/sdk/go/v2/signing/signingtest"
)

const (
	relRev    = "0123456789abcdef0123456789abcdef01234567"
	relDigest = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
)

type relGit struct{ files map[string][]byte }

func (relGit) HasOrigin(context.Context) bool             { return true }
func (relGit) Fetch(context.Context) error                { return nil }
func (relGit) DefaultRef(context.Context) (string, error) { return "origin/main", nil }
func (g relGit) Show(_ context.Context, _, p string) ([]byte, bool, error) {
	d, ok := g.files[p]
	return d, ok, nil
}

type relFixture struct {
	dir    string
	git    relGit
	exit   int
	report string
	runs   int
	// noPlugin models a machine without the ci plugin: no evidence runner.
	noPlugin bool
}

// signedRelease writes a release signed by a trusted ci-release key; tweak may
// alter the manifest fields, the trust files or the signer before signing.
func signedRelease(t *testing.T, tweak func(m map[string]any, trust map[string][]byte, key *signing.Key, s *signing.Signer)) *relFixture {
	t.Helper()
	key, signer := signingtest.NewKey(t, signing.PurposeCIRelease)
	evidence := []byte(`{"revision":"` + relRev + `"}`)
	sum := sha256.Sum256(evidence)
	m := map[string]any{
		"schema": "ci.release-manifest/v1", "revision": relRev,
		"evidence": map[string]any{"path": "evidence.json", "sha256": hex.EncodeToString(sum[:])},
		"artifacts": []any{map[string]any{"name": "web", "sha256": strings.Repeat("a", 64),
			"image": map[string]any{"index_digest": relDigest, "manifests": []any{map[string]any{"platform": "linux/amd64", "digest": relDigest}}}}},
	}
	trust := map[string][]byte{".nself/trust/ci-release.keys": []byte(fmt.Sprintf("%s %s\n", key.ID, base64.StdEncoding.EncodeToString(key.Public)))}
	if tweak != nil {
		tweak(m, trust, &key, &signer)
	}
	if m["signing"] == nil {
		m["signing"] = map[string]any{"purpose": "ci-release", "key_id": key.ID}
	}
	payload, _ := json.Marshal(m)
	env, err := signing.SignEnvelope(signer, verify.PayloadType, payload)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(env)
	dir := t.TempDir()
	for name, data := range map[string][]byte{verify.ManifestFile: raw, verify.EvidenceFile: evidence} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return &relFixture{dir: dir, git: relGit{files: trust}, report: `{"data":{"class":"release","result":"pass","quarantined":false}}`}
}

func (f *relFixture) input() verify.Input {
	in := verify.Input{Dir: f.dir, RepoRoot: f.dir, Home: f.dir, Git: f.git}
	if !f.noPlugin {
		in.Evidence = func(context.Context, string, string) ([]byte, int, error) {
			f.runs++
			return []byte(f.report), f.exit, nil
		}
	}
	return in
}

func TestSimPromoteReleaseVerify(t *testing.T) {
	target := startHost(t)
	target.put("/usr/local/bin/nself", promoteNselfStub)
	target.sh("chmod 755 /usr/local/bin/nself; printf original > /tmp/sim-db-fp; chown nself /tmp/sim-db-fp")
	target.own()
	targetRemote := target.remote()
	const tag = "pre-promote-1791720000-feedface"

	// run drives the canonical step list with verification as the Verify hook
	// and the digest check at the compare step; later steps touch the host.
	run := func(f *relFixture, comparedDigest string) error {
		var manifest *verify.Manifest
		state := &promote.State{Source: "ls", Target: "prodsim", TargetTier: controlplane.TierProd, Values: map[string]any{}}
		steps := make([]promote.Step, 0, len(promote.StepNames))
		for _, name := range promote.StepNames {
			steps = append(steps, promote.Step{Name: name})
		}
		steps[7].Run = func(_ context.Context, st *promote.State) error {
			st.Digests = []promote.DigestRow{{Service: "web", Source: comparedDigest, Target: comparedDigest, Status: "equal"}}
			return verify.CheckDigests(manifest, st.Digests)
		}
		steps[8] = promote.SnapshotStep(targetRemote, target.rp, tag)
		eng := promote.Engine{Steps: steps, Verify: func(ctx context.Context, _ *promote.State) error {
			m, err := verify.Verify(ctx, f.input())
			manifest = m
			return err
		}}
		_, err := eng.Run(context.Background(), state)
		return err
	}

	cases := []struct {
		name   string
		want   string
		tweak  func(m map[string]any, trust map[string][]byte, key *signing.Key, s *signing.Signer)
		exit   int
		report string
		digest string
		noPlug bool
	}{
		{name: "missing ci plugin", want: "E499", noPlug: true},
		{name: "tampered digest", want: "E495", digest: "sha256:" + strings.Repeat("9", 64)},
		{name: "wrong key id", want: "E493", tweak: func(m map[string]any, _ map[string][]byte, _ *signing.Key, s *signing.Signer) {
			_, other := signingtest.NewKey(t, signing.PurposeCIRelease)
			*s = other
		}},
		{name: "plugins purpose key", want: "E493", tweak: func(m map[string]any, _ map[string][]byte, _ *signing.Key, s *signing.Signer) {
			_, other := signingtest.NewKey(t, signing.PurposePlugins)
			*s = other
		}},
		{name: "revoked key", want: "E494", tweak: func(m map[string]any, trust map[string][]byte, key *signing.Key, _ *signing.Signer) {
			trust[".nself/trust/ci-release.revoked"] = []byte(key.ID + "\n")
		}},
		{name: "stale evidence", want: "E497", exit: 10},
		{name: "unbound evidence", want: "E497", exit: 12},
		{name: "revision mismatch", want: "E496", tweak: func(m map[string]any, _ map[string][]byte, _ *signing.Key, _ *signing.Signer) {
			m["revision"] = "fedcba9876543210fedcba9876543210fedcba98"
		}},
		{name: "quarantined", want: "E498", report: `{"data":{"class":"release","result":"pass","quarantined":true}}`},
		{name: "unknown major", want: "E492", tweak: func(m map[string]any, _ map[string][]byte, _ *signing.Key, _ *signing.Signer) {
			m["schema"] = "ci.release-manifest/v2"
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := signedRelease(t, tc.tweak)
			f.exit, f.noPlugin = tc.exit, tc.noPlug
			if tc.report != "" {
				f.report = tc.report
			}
			digest := relDigest
			if tc.digest != "" {
				digest = tc.digest
			}
			targetRemote.log = nil
			err := run(f, digest)
			var coded *errs.CLIError
			if !errors.As(err, &coded) || coded.Code != tc.want {
				t.Fatalf("err = %v, want %s", err, tc.want)
			}
			if len(targetRemote.log) != 0 {
				t.Fatalf("%s reached the target host: %v", tc.want, targetRemote.log)
			}
		})
	}

	t.Run("valid release reaches the snapshot", func(t *testing.T) {
		f := signedRelease(t, nil)
		targetRemote.log = nil
		if err := run(f, relDigest); err != nil {
			t.Fatal(err)
		}
		if f.runs != 1 || !strings.Contains(strings.Join(targetRemote.log, "\n"), "backup create") {
			t.Fatalf("evidence runs %d, target commands %v", f.runs, targetRemote.log)
		}
	})
}
