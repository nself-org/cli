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
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

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
	// HasOrigin reports whether the project is a git repository with an origin
	// remote. The error is for git itself failing to run: that must not read as
	// "no origin", because it would skip the committed revocations.
	HasOrigin(ctx context.Context) (bool, error)
	// PinDefault refreshes origin's default branch and returns the commit it
	// points to. It fails when that cannot be established.
	PinDefault(ctx context.Context) (commit string, err error)
	// Show returns the committed content of path at the commit; found is false
	// when the path does not exist there.
	Show(ctx context.Context, commit, path string) (data []byte, found bool, err error)
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
	if !filepath.IsAbs(home) {
		// An empty or relative home would resolve against the process working
		// directory, which is normally the project tree: never a trust root.
		return t, errs.New("E493", "cannot locate the operator home directory, so ~/.nself/ci/trust cannot be read").WithFix("Set HOME to an absolute path.")
	}
	for _, f := range []struct {
		name string
		dst  *[][]byte
	}{{keysName, &keyDocs}, {revokedName, &revDocs}} {
		data, err := readTrustFile(filepath.Join(home, ".nself", "ci", "trust", f.name))
		switch {
		case err == nil:
			*f.dst = append(*f.dst, data)
		case !errors.Is(err, os.ErrNotExist):
			return t, errs.Wrap("E493", "cannot read operator trust file "+f.name, err)
		}
	}
	hasOrigin := false
	if g != nil {
		var err error
		if hasOrigin, err = g.HasOrigin(ctx); err != nil {
			return t, errs.Wrap("E493", "cannot tell whether the project has an origin remote", err)
		}
	}
	if hasOrigin {
		commit, err := g.PinDefault(ctx)
		if err != nil {
			return t, errs.Wrap("E493", "cannot refresh origin's default branch to read the committed trust files", err).WithFix("Check that origin is reachable, or place the keys in ~/.nself/ci/trust/.")
		}
		var differing []string
		for _, f := range []struct {
			name string
			dst  *[][]byte
		}{{keysName, &keyDocs}, {revokedName, &revDocs}} {
			data, found, err := g.Show(ctx, commit, trustRel+f.name)
			if err != nil {
				return t, errs.Wrap("E493", fmt.Sprintf("cannot read %s at origin commit %.12s", trustRel+f.name, commit), err)
			}
			if found {
				*f.dst = append(*f.dst, data)
			}
			if workingCopyDiffers(filepath.Join(repoRoot, trustRel+f.name), data, found) {
				differing = append(differing, trustRel+f.name)
			}
		}
		if len(differing) > 0 {
			t.Warnings = append(t.Warnings, fmt.Sprintf("working-tree %s differs from origin commit %.12s; only the committed copy is trusted", strings.Join(differing, " and "), commit))
		}
	}
	return parseTrust(t, keyDocs, revDocs)
}

// workingCopyDiffers reports a working-tree trust file that is not the committed one.
func workingCopyDiffers(path string, committed []byte, found bool) bool {
	local, err := readTrustFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	// A copy that cannot be read as a small regular file also differs.
	return err != nil || !found || !bytes.Equal(local, committed)
}

// readTrustFile reads a small regular file. A FIFO, device, symlink swap or
// oversize file is an error, so a hostile path can neither block nor exhaust
// memory: the file is opened without blocking and the open descriptor is
// checked against the earlier Lstat before anything is read.
func readTrustFile(path string) ([]byte, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0) //nolint:gosec // trust file path built from the operator home or project root
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	if err := sameRegularFile(before, f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	data, err := io.ReadAll(io.LimitReader(f, trustFileLimit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > trustFileLimit {
		return nil, fmt.Errorf("%s is larger than %d bytes", path, trustFileLimit)
	}
	return data, nil
}

// sameRegularFile proves the open file is the regular file Lstat saw.
func sameRegularFile(before os.FileInfo, f *os.File) error {
	after, err := f.Stat()
	if err != nil {
		return err
	}
	if !after.Mode().IsRegular() || !os.SameFile(before, after) {
		return fmt.Errorf("the path changed while it was being opened")
	}
	return nil
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
