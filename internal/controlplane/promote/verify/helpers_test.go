package verify

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/signing"
	"github.com/nself-org/cli/sdk/go/v2/signing/signingtest"
)

const (
	testRev    = "0123456789abcdef0123456789abcdef01234567"
	otherRev   = "fedcba9876543210fedcba9876543210fedcba98"
	imgIndex   = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	imgAMD64   = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	imgARM64   = "sha256:3333333333333333333333333333333333333333333333333333333333333333"
	unlistedID = "sha256:9999999999999999999999999999999999999999999999999999999999999999"
)

// fakeGit serves committed trust files from memory.
type fakeGit struct {
	noOrigin bool
	fetchErr error
	files    map[string][]byte
	fetched  int
}

func (g *fakeGit) HasOrigin(context.Context) bool { return !g.noOrigin }
func (g *fakeGit) Fetch(context.Context) error    { g.fetched++; return g.fetchErr }
func (g *fakeGit) DefaultRef(context.Context) (string, error) {
	return "origin/main", nil
}
func (g *fakeGit) Show(_ context.Context, _ string, path string) ([]byte, bool, error) {
	d, ok := g.files[path]
	return d, ok, nil
}

// release is a signed release directory under test.
type release struct {
	t        *testing.T
	dir      string
	home     string
	repo     string
	key      signing.Key
	signer   signing.Signer
	git      *fakeGit
	manifest map[string]any
	evidence []byte
	runs     []string
	exit     int
	report   string
	runErr   error
}

func keysLine(k signing.Key) []byte {
	return []byte(fmt.Sprintf("# trust\n%s %s\n", k.ID, base64.StdEncoding.EncodeToString(k.Public)))
}

// newRelease writes a valid release and trusts its key on the default branch.
func newRelease(t *testing.T) *release {
	t.Helper()
	key, signer := signingtest.NewKey(t, signing.PurposeCIRelease)
	r := &release{t: t, dir: t.TempDir(), home: t.TempDir(), repo: t.TempDir(), key: key, signer: signer,
		git:    &fakeGit{files: map[string][]byte{trustRel + keysName: keysLine(key)}},
		report: `{"v":1,"ok":true,"data":{"class":"release","result":"pass","quarantined":false}}`}
	r.evidence = []byte(`{"revision":"` + testRev + `","result":"pass"}`)
	r.manifest = map[string]any{
		"schema": "ci.release-manifest/v1", "revision": testRev, "future_field": map[string]any{"a": 1},
		"signing": map[string]any{"purpose": "ci-release", "key_id": key.ID},
		"artifacts": []any{map[string]any{"name": "web", "sha256": strings.Repeat("ab", 32),
			"image": map[string]any{"index_digest": imgIndex, "manifests": []any{
				map[string]any{"platform": "linux/amd64", "digest": imgAMD64},
				map[string]any{"platform": "linux/arm64", "digest": imgARM64}}}}},
	}
	return r
}

// write signs and writes the envelope and the evidence file.
func (r *release) write(payloadType string, s signing.Signer) {
	r.t.Helper()
	sum := sha256.Sum256(r.evidence)
	r.manifest["evidence"] = map[string]any{"path": EvidenceFile, "sha256": hex.EncodeToString(sum[:])}
	payload, err := json.Marshal(r.manifest)
	if err != nil {
		r.t.Fatal(err)
	}
	env, err := signing.SignEnvelope(s, payloadType, payload)
	if err != nil {
		r.t.Fatal(err)
	}
	raw, _ := json.Marshal(env)
	for name, data := range map[string][]byte{ManifestFile: raw, EvidenceFile: r.evidence} {
		if err := os.WriteFile(filepath.Join(r.dir, name), data, 0o600); err != nil {
			r.t.Fatal(err)
		}
	}
}

func (r *release) input() Input {
	return Input{Dir: r.dir, RepoRoot: r.repo, Home: r.home, Git: r.git,
		Evidence: func(_ context.Context, file, rev string) ([]byte, int, error) {
			r.runs = append(r.runs, filepath.Base(file)+"@"+rev)
			return []byte(r.report), r.exit, r.runErr
		}}
}
