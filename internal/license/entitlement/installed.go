package entitlement

import (
	"context"
	"net/http"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/httptimeout"
	"github.com/nself-org/cli/internal/license"
)

// RefreshInstalled refreshes a project only when a licensed plugin is installed.
// Purpose: keep free-only stacks independent of licences. Inputs: context/project.
// Outputs: the refresh result. Constraints: native manifests, no runtime Docker calls.
// SPORT: start/license-set entitlement publication.
func RefreshInstalled(ctx context.Context, projectDir string) error {
	return RefreshInstalledForKey(ctx, projectDir, "")
}

// RefreshInstalledForKey validates a newly set key before publishing its token.
// Purpose: prevent old-key reuse after license set. Inputs: context/project/new key.
// Outputs: private runtime file or E125. Constraints: only licensed installations
// need online validation; a rejected key never retains the old token.
// SPORT: license-set token capture.
func RefreshInstalledForKey(ctx context.Context, projectDir, key string) error {
	// compat.V15(P7-PLUG-24): start and license set ignore entitlement -> refresh licensed installed plugins
	if !compat.V15() || DevKeyMode() {
		return nil
	}
	installed, err := HasLicensedInstalled()
	if err != nil {
		return err
	}
	if !installed {
		return nil
	}
	if key != "" {
		if err := validateNewKey(ctx, projectDir, key); err != nil {
			return err
		}
	}
	return Refresh(ctx, projectDir)
}

// validateNewKey requires an online result before replacing runtime entitlement.
// Purpose: preserve valid files offline. Inputs: context/project/key. Outputs: error.
// Constraints: only explicit server refusal clears the file. SPORT: key validation.
func validateNewKey(ctx context.Context, projectDir, key string) error {
	remote := &validationStatus{client: httptimeout.Default}
	result, err := license.Validate(ctx, key, &license.ValidatorOptions{HTTPClient: remote})
	if err != nil {
		return errs.Wrap("E125", "could not validate the new licence online; existing entitlement retained", err)
	}
	if result.FromCache || result.Status == license.StatusFailOpen {
		return errs.New("E125", "could not validate the new licence online; existing entitlement retained")
	}
	// The validator treats every other 4xx as auth failure too; only 401/403
	// or a verified 200 rejection revoke this runtime entitlement.
	rejected := remote.status == http.StatusUnauthorized || remote.status == http.StatusForbidden ||
		(remote.status == http.StatusOK && (result.Status == license.StatusRevoked || result.Status == license.StatusExpired))
	if result.CanProceed && !rejected {
		return nil
	}
	// FromCache alone cannot distinguish an absent cache from an auth rejection.
	if !rejected {
		return errs.New("E125", "could not validate the new licence online; existing entitlement retained")
	}
	if writeErr := writeAtomic(Path(projectDir), nil); writeErr != nil {
		return errs.Wrap("E125", "could not clear the plugin entitlement file", writeErr)
	}
	return errs.New("E125", "the new licence was rejected and could not obtain a verified entitlement")
}

// validationStatus records the response status without changing validator policy.
// Purpose: identify explicit refusals. Inputs: HTTP request. Outputs: response/status.
// Constraints: request-local state, existing timeout client. SPORT: key validation.
type validationStatus struct {
	client license.HTTPDoer
	status int
}

func (s *validationStatus) Do(req *http.Request) (*http.Response, error) {
	resp, err := s.client.Do(req)
	if err == nil && resp != nil {
		s.status = resp.StatusCode
	}
	return resp, err
}
