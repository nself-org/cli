package entitlement

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/httptimeout"
	"github.com/nself-org/cli/internal/license"
)

// Purpose: renew configured keys once each without blocking offline starts.
// Inputs: context/keys. Outputs: rejected subjects/renewed tokens/rejection persistence error. Constraints: five seconds
// overall, existing HTTP timeouts, no cache changes on transient failures.
// SPORT: online entitlement renewal.
func revalidateKeys(ctx context.Context) (map[string]bool, []string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rejected := make(map[string]bool)
	var renewed []string
	var recordErr error
	for _, key := range license.CollectLicenseKeys() {
		if ctx.Err() != nil {
			break
		}
		remote := &validationStatus{client: httptimeout.Default}
		result, err := license.Validate(ctx, key, &license.ValidatorOptions{HTTPClient: remote})
		if err == nil && explicitRejection(remote, result) {
			rejected[license.HashKey(key)] = true
			if err := recordVerdict(license.HashKey(key), true); err != nil {
				recordErr = err
			}
		}
		if err == nil && result != nil && result.CanProceed && !result.FromCache && result.Status == license.StatusValid {
			_ = recordVerdict(license.HashKey(key), false) // a stale rejection only fails closed
			if token, err := cachedToken(); err == nil {
				renewed = append(renewed, token)
			}
		}
	}
	return rejected, renewed, recordErr
}

// Purpose: distinguish revocation from transport and unrelated HTTP failures.
// Inputs: response/result. Outputs: refusal. Constraints: signed rejection,
// 401, or ping-shaped 403; captive portals cannot revoke.
// SPORT: fail-closed runtime revocation.
func explicitRejection(remote *validationStatus, result *license.ValidatorResult) bool {
	return remote.status == http.StatusUnauthorized || (remote.status == http.StatusForbidden && remote.pingError) ||
		(remote.status == http.StatusOK && result != nil && !result.FromCache &&
			(result.Status == license.StatusRevoked || result.Status == license.StatusExpired))
}

// Purpose: retain a verified runtime token if the cache is temporarily absent.
// Inputs: file path. Outputs: token. Constraints: configured subject and signed
// grace deadline, bounded regular-file read. SPORT: reboot/offline continuity.
func retainedToken(path string) string {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return ""
	}
	f, err := os.OpenFile(path, os.O_RDONLY|noFollowFlag, 0)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, 1<<20+1))
	if err != nil || len(data) > 1<<20 {
		return ""
	}
	claims, err := license.ParseLicenseJWT(string(data))
	if err != nil || claims.Exp <= 0 || time.Now().After(time.Unix(claims.Exp, 0).Add(license.PostExpiryGraceWindow)) {
		return ""
	}
	for _, key := range license.CollectLicenseKeys() {
		if claims.Sub == license.HashKey(key) {
			return string(data)
		}
	}
	return ""
}

// Purpose: report expired grace and uncovered installed manifest slugs.
// Inputs: verified token/slugs. Outputs: policy error. Constraints: signed exp.
// SPORT: honest build diagnostics under the single-token v1 contract.
func tokenPolicy(token string, names []string) error {
	claims, err := license.ParseLicenseJWT(token)
	if err != nil {
		return err
	}
	if time.Now().After(time.Unix(claims.Exp, 0).Add(license.PostExpiryGraceWindow)) {
		return fmt.Errorf("entitlement grace ended")
	}
	allowed := make(map[string]bool)
	for _, name := range claims.Plugins {
		allowed[name] = true
	}
	var missing []string
	for _, name := range names {
		if !allowed["*"] && !allowed[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("entitlement does not cover installed plugins: %s", strings.Join(missing, ", "))
	}
	return nil
}

// Purpose: discover enabled licensed manifest names from the exact installation.
// Inputs: directory or default. Outputs: names/error. Constraints: shared policy.
// SPORT: single-token coverage validation.
func installedLicensedNames(dir string) ([]string, error) {
	dir, err := installationDirectory(dir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		p := filepath.Join(dir, entry.Name())
		if _, err := os.Stat(filepath.Join(p, ".disabled")); err == nil {
			continue
		}
		licensed, err := IsLicensedPlugin(p)
		if err != nil {
			return nil, err
		}
		if !licensed {
			continue
		}
		name, err := manifestName(p, entry.Name())
		if err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, nil
}

// Purpose: resolve an explicit or configured plugin installation directory.
// Inputs: optional directory. Outputs: path/error. Constraints: native defaults.
// SPORT: licensed installation discovery.
func installationDirectory(dir string) (string, error) {
	if dir != "" {
		return dir, nil
	}
	if dir = strings.TrimSpace(os.Getenv("NSELF_PLUGIN_DIR")); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".nself", "plugins"), nil
}

// Purpose: use the canonical manifest name rather than installation folder.
// Inputs: plugin directory/fallback. Outputs: slug/error. Constraints: legacy
// nameless manifests retain folder identity. SPORT: runtime slug coverage.
func manifestName(dir, fallback string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, "plugin.json"))
	if err != nil {
		return "", err
	}
	var manifest struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", err
	}
	if manifest.Name == "" {
		return fallback, nil
	}
	return manifest.Name, nil
}
