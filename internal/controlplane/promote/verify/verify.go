package verify

// Purpose: Verify is the promote engine's Verify hook body: it proves a
//          release directory is a signed, trusted, current release before any
//          snapshot or remote change.
// Inputs:  Input (release dir, project root, operator home, Git, evidence runner).
// Outputs: the verified Manifest, or a coded error E492-E499.
// Constraints: order is payloadType, signature, schema major, signer match,
//              evidence pin, revision, evidence (digests are checked later by
//              CheckDigests, once promote has captured them). No bypass.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/sdk/go/v2/signing"
)

const (
	// ManifestFile is the DSSE envelope inside a release directory.
	ManifestFile = "release-manifest.dsse.json"
	maxFile      = 16 << 20
)

// Input is everything Verify needs, injected so tests need no process.
type Input struct {
	Dir      string
	RepoRoot string
	Home     string
	Git      Git
	Evidence EvidenceRunner
	Warn     func(string)
}

// Verify checks the release directory in the contract order.
func Verify(ctx context.Context, in Input) (*Manifest, error) {
	raw, err := readBounded(filepath.Join(in.Dir, ManifestFile))
	if err != nil {
		return nil, errs.Wrap("E492", "cannot read "+ManifestFile+" in the release directory", err)
	}
	var env signing.Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, errs.Wrap("E492", ManifestFile+" is not a DSSE envelope", err)
	}
	if env.PayloadType != PayloadType {
		return nil, errs.Newf("E493", "release manifest payloadType %q is not %s", clip(env.PayloadType), PayloadType)
	}
	trust, err := LoadTrust(ctx, in.Git, in.Home, in.RepoRoot)
	if err != nil {
		return nil, err
	}
	if in.Warn != nil {
		for _, w := range trust.Warnings {
			in.Warn(w)
		}
	}
	payload, signers, err := verifyEnvelope(env, trust)
	if err != nil {
		return nil, err
	}
	m, err := decodeManifest(payload)
	if err != nil {
		return nil, err
	}
	if !contains(signers, m.Signing.KeyID) || m.Signing.Purpose != string(signing.PurposeCIRelease) {
		return nil, errs.Newf("E493", "manifest names signing key %q, which did not sign it", clip(m.Signing.KeyID))
	}
	doc, err := checkEvidencePin(filepath.Join(in.Dir, EvidenceFile), m)
	if err != nil {
		return nil, err
	}
	// The plugin reads a private copy of the bytes that were just pinned, so
	// the release directory cannot change between the pin and the judgement.
	scratch, err := os.MkdirTemp("", "nself-release-")
	if err != nil {
		return nil, errs.Wrap("E497", "cannot stage the evidence for verification", err)
	}
	defer func() { _ = os.RemoveAll(scratch) }()
	staged := filepath.Join(scratch, EvidenceFile)
	if err := os.WriteFile(staged, doc, 0o600); err != nil {
		return nil, errs.Wrap("E497", "cannot stage the evidence for verification", err)
	}
	return m, checkEvidence(ctx, in.Evidence, staged, m.Revision)
}

func verifyEnvelope(env signing.Envelope, t Trust) ([]byte, []string, error) {
	v, err := signing.NewVerifier(signing.PurposeCIRelease, t.Keys, t.Revoked)
	if err != nil {
		return nil, nil, errs.Wrap("E493", "ci-release trust roots are unusable", err)
	}
	_, payload, signers, err := signing.VerifyEnvelope(v, env)
	switch {
	case err == nil:
		return payload, signers, nil
	case errors.Is(err, signing.ErrRevoked):
		return nil, nil, errs.Wrap("E494", "the key that signed the release manifest is revoked", err)
	default:
		return nil, nil, errs.Wrap("E493", "release manifest signature rejected: "+err.Error(), err)
	}
}

// checkEvidencePin proves the evidence beside the manifest is the document the
// manifest pins and that both name one revision.
func checkEvidencePin(file string, m *Manifest) ([]byte, error) {
	doc, err := readBounded(file)
	if err != nil {
		return nil, errs.Wrap("E497", "cannot read "+EvidenceFile+" in the release directory", err)
	}
	sum := sha256.Sum256(doc)
	if hex.EncodeToString(sum[:]) != m.Evidence.SHA256 {
		return nil, errs.New("E497", EvidenceFile+" is not the document the release manifest pins")
	}
	var head struct {
		Revision string `json:"revision"`
	}
	if err := json.Unmarshal(doc, &head); err != nil {
		return nil, errs.Wrap("E497", EvidenceFile+" is not valid JSON", err)
	}
	if head.Revision != m.Revision {
		return nil, errs.Newf("E496", "manifest revision %.12s differs from evidence revision %.12s", m.Revision, head.Revision)
	}
	return doc, nil
}

func readBounded(path string) ([]byte, error) {
	f, err := os.Open(path) //nolint:gosec // operator-supplied release directory
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxFile+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxFile {
		return nil, errors.New("file is larger than the 16 MiB limit")
	}
	return data, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
