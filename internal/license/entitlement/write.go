// Package entitlement writes the ping-issued token consumed by plugin runtimes.
// Purpose: publish only the verified cached licence JWT as a private file.
// Inputs: context, project directory, and the existing licence cache.
// Outputs: .nself/state/entitlement.jwt, or E125 with an empty fail-closed file.
// Constraints: v1.4 never writes; no JWT is exposed in compose or diagnostics.
// SPORT: runtime entitlement publication for build, start and license set.
package entitlement

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/license"
)

// Path resolves the private runtime token file.
// Purpose: share the runtime state location. Inputs: project root. Outputs: path.
// Constraints: project-local state only. SPORT: entitlement file location.
func Path(projectDir string) string {
	return filepath.Join(projectDir, ".nself", "state", "entitlement.jwt")
}

// Refresh publishes a verified cached licence JWT while preserving mounted inodes.
// Purpose: prepare the runtime token. Inputs: context and project root.
// Outputs: a private JWT file, or an empty file and E125 if no verified token exists.
// Constraints: trust server signatures, not cache copies; v1.4 is unchanged.
// SPORT: runtime entitlement refresh.
func Refresh(ctx context.Context, projectDir string) error {
	// compat.V15(P7-PLUG-24): no runtime entitlement file -> verified JWT publication and E125 on failure
	if !compat.V15() || DevKeyMode() {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return errs.Wrap("E125", "plugin entitlement refresh was canceled", err)
	}
	token, tokenErr := cachedToken()
	if err := writeAtomic(Path(projectDir), []byte(token)); err != nil {
		return errs.Wrap("E125", fmt.Sprintf("could not write the plugin entitlement file: %v", err), err)
	}
	if tokenErr != nil {
		return errs.Wrap("E125", "no verified plugin entitlement token is available", tokenErr)
	}
	return nil
}

// cachedToken checks both the server-signed cache body and its JWT.
// Purpose: reject altered cache data. Inputs: existing cache. Outputs: token/error.
// Constraints: no network or local timestamp anchors. SPORT: entitlement trust.
func cachedToken() (string, error) {
	entry, err := license.ReadCache()
	if err != nil {
		return "", err
	}
	if entry == nil || !entry.VerifySignature() {
		return "", fmt.Errorf("licence cache is absent or its signature is invalid")
	}
	claims, err := license.ParseLicenseJWT(entry.JWT)
	if err != nil {
		return "", err
	}
	if claims.Exp <= 0 {
		return "", fmt.Errorf("licence token has no signed expiry")
	}
	keys := license.CollectLicenseKeys()
	if len(keys) == 0 {
		return "", fmt.Errorf("no licence key is configured")
	}
	for _, key := range keys {
		if license.HashKey(key) == claims.Sub {
			return entry.JWT, nil
		}
	}
	return "", fmt.Errorf("cached token does not belong to a configured licence")
}

// writeAtomic creates new files atomically and refreshes existing mounted inodes.
// Purpose: keep single-file bind mounts current. Inputs: path/data. Outputs: error.
// Constraints: 0600; SDK retries torn reads. SPORT: private state publication.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err == nil {
		if info.IsDir() {
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("entitlement target directory is not empty or cannot be removed: %w", err)
			}
		} else {
			return writeExisting(path, data, info)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return writeNew(path, data)
}

// writeExisting updates the inode a running container has mounted.
// Purpose: preserve bind mount identity. Inputs: path/data/stat. Outputs: error.
// Constraints: no symlinks or truncation before writing. SPORT: mounted refresh.
func writeExisting(path string, data []byte, before os.FileInfo) error {
	if !before.Mode().IsRegular() {
		return fmt.Errorf("entitlement target must be a regular file, not a symlink or special file")
	}
	if before.Mode().Perm()&0200 == 0 {
		if err := normalizePermissions(path, before); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(path, os.O_WRONLY|noFollowFlag, 0)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	after, err := f.Stat()
	if err != nil {
		return err
	}
	if !after.Mode().IsRegular() || !os.SameFile(before, after) {
		return fmt.Errorf("entitlement target changed during refresh")
	}
	if err := f.Chmod(0600); err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Truncate(int64(len(data))); err != nil {
		return err
	}
	return f.Sync()
}

// normalizePermissions repairs read-only files through a verified descriptor.
// Purpose: safely restore private writes. Inputs: path/stat. Outputs: error.
// Constraints: no pathname chmod or symlink traversal. SPORT: mounted refresh.
func normalizePermissions(path string, before os.FileInfo) error {
	f, err := os.OpenFile(path, os.O_RDONLY|noFollowFlag, 0)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	after, err := f.Stat()
	if err != nil {
		return err
	}
	if !after.Mode().IsRegular() || !os.SameFile(before, after) {
		return fmt.Errorf("entitlement target changed during refresh")
	}
	return f.Chmod(0600)
}

// writeNew publishes the initial file and persists its directory entry.
// Purpose: atomic first creation. Inputs: path/data. Outputs: error.
// Constraints: private sibling file. SPORT: initial entitlement publication.
func writeNew(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".entitlement-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	defer func() { _ = tmp.Close() }()
	if err := tmp.Chmod(0600); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return err
	}
	return syncDirectory(dir)
}
