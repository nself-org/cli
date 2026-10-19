package verify

// Purpose: load the project's ci-release trust roots without ever reading the
//          working tree as a source of trust.
// Inputs:  the operator's ~/.nself/ci/trust directory and the project's
//          .nself/trust/ci-release.{keys,revoked} as committed on the default
//          branch of origin (after a fetch).
// Outputs: a Trust (keys, revoked ids, warnings), or E493.
// Constraints: a differing working-tree copy only produces one warning. A
//              fetch that fails when origin exists is fatal: a stale revoked
//              list must not silently authorise a release.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/sdk/go/v2/signing"
)

const (
	keysName    = "ci-release.keys"
	revokedName = "ci-release.revoked"
	trustRel    = ".nself/trust/"
)

// Git is the read-only view of the project repository the trust loader needs.
type Git interface {
	// HasOrigin reports whether the repository has an origin remote.
	HasOrigin(ctx context.Context) bool
	// Fetch refreshes origin.
	Fetch(ctx context.Context) error
	// DefaultRef names the default branch ref, for example origin/main.
	DefaultRef(ctx context.Context) (string, error)
	// Show returns the committed content of path at ref; found is false when
	// the path does not exist there.
	Show(ctx context.Context, ref, path string) (data []byte, found bool, err error)
}

// Trust is the set of trusted ci-release keys and revoked key ids.
type Trust struct {
	Keys     []signing.Key
	Revoked  []string
	Warnings []string
}

// LoadTrust merges the operator trust directory with the default-branch files.
func LoadTrust(ctx context.Context, g Git, home, repoRoot string) (Trust, error) {
	var keyDocs, revDocs [][]byte
	var t Trust
	for _, f := range []struct {
		name string
		dst  *[][]byte
	}{{keysName, &keyDocs}, {revokedName, &revDocs}} {
		data, err := os.ReadFile(filepath.Join(home, ".nself", "ci", "trust", f.name))
		switch {
		case err == nil:
			*f.dst = append(*f.dst, data)
		case !errors.Is(err, os.ErrNotExist):
			return t, errs.Wrap("E493", "cannot read operator trust file "+f.name, err)
		}
	}
	if g != nil && g.HasOrigin(ctx) {
		if err := g.Fetch(ctx); err != nil {
			return t, errs.Wrap("E493", "cannot refresh origin to read the committed trust files", err).WithFix("Run git fetch origin, or place the keys in ~/.nself/ci/trust/.")
		}
		ref, err := g.DefaultRef(ctx)
		if err != nil {
			return t, errs.Wrap("E493", "cannot find the default branch of origin", err)
		}
		for _, f := range []struct {
			name string
			dst  *[][]byte
		}{{keysName, &keyDocs}, {revokedName, &revDocs}} {
			data, found, err := g.Show(ctx, ref, trustRel+f.name)
			if err != nil {
				return t, errs.Wrap("E493", fmt.Sprintf("cannot read %s at %s", trustRel+f.name, ref), err)
			}
			if found {
				*f.dst = append(*f.dst, data)
			}
			if w := workingCopyDiffers(filepath.Join(repoRoot, trustRel+f.name), data, found); w {
				t.Warnings = append(t.Warnings, fmt.Sprintf("working-tree %s differs from %s; only the committed copy is trusted", trustRel+f.name, ref))
			}
		}
	}
	return parseTrust(t, keyDocs, revDocs)
}

// workingCopyDiffers reports a working-tree trust file that is not the committed one.
func workingCopyDiffers(path string, committed []byte, found bool) bool {
	local, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return !found || !bytes.Equal(local, committed)
}

func parseTrust(t Trust, keyDocs, revDocs [][]byte) (Trust, error) {
	seenKey, seenRev := map[string]bool{}, map[string]bool{}
	for _, doc := range keyDocs {
		keys, err := signing.ParseKeysFile(bytes.NewReader(doc), signing.PurposeCIRelease)
		if err != nil {
			return t, errs.Wrap("E493", "ci-release trust keys are malformed", err)
		}
		for _, k := range keys {
			if !seenKey[k.ID] {
				seenKey[k.ID] = true
				t.Keys = append(t.Keys, k)
			}
		}
	}
	for _, doc := range revDocs {
		ids, err := signing.ParseRevokedFile(bytes.NewReader(doc))
		if err != nil {
			return t, errs.Wrap("E493", "ci-release revoked list is malformed", err)
		}
		for _, id := range ids {
			if !seenRev[id] {
				seenRev[id] = true
				t.Revoked = append(t.Revoked, id)
			}
		}
	}
	sort.Strings(t.Revoked)
	return t, nil
}
