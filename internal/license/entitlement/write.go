// Package entitlement writes the ping-issued token consumed by plugin runtimes.
// Purpose: publish only the verified cached licence JWT as an atomic private file.
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

// Refresh atomically publishes a verified cached licence JWT.
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
		return err
	}
	token, tokenErr := cachedToken()
	if err := writeAtomic(Path(projectDir), []byte(token)); err != nil {
		return errs.Wrap("E125", "could not write the plugin entitlement file", err)
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
	if len(keys) > 0 {
		for _, key := range keys {
			if license.HashKey(key) == claims.Sub {
				return entry.JWT, nil
			}
		}
		return "", fmt.Errorf("cached token does not belong to a configured licence")
	}
	return entry.JWT, nil
}

// writeAtomic replaces path with data through a private sibling temporary file.
// Purpose: avoid torn or world-readable tokens. Inputs: path/data. Outputs: error.
// Constraints: always 0600, including empty files. SPORT: private state publication.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".entitlement-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
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
	return os.Rename(tmp.Name(), path)
}
