package entitlement

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

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
	if key != "" {
		return publishAfterValidation(projectDir)
	}
	return Refresh(ctx, projectDir)
}

// RefreshForStart reports entitlement failures while allowing free services up.
// Purpose: isolate licensed availability. Inputs: context/project/warnings.
// Outputs: warning line. Constraints: runtime Guard fails closed. SPORT: start.
func RefreshForStart(ctx context.Context, projectDir string, warnings io.Writer) {
	_ = StartMayContinue(RefreshInstalled(ctx, projectDir), warnings)
}

// StartMayContinue reports E125 without preventing free services from starting.
// Purpose: share refresh and auto-build failure policy. Inputs: error/warnings.
// Outputs: continuation decision. Constraints: other errors remain fatal.
// SPORT: licensed-plugin isolation during start.
func StartMayContinue(err error, warnings io.Writer) bool {
	if err == nil {
		return true
	}
	var coded *errs.CLIError
	if errors.As(err, &coded) && coded.Code == "E125" {
		_, _ = fmt.Fprintln(warnings, err)
		return true
	}
	return false
}

// publishAfterValidation avoids validating a new key twice in one command.
// Purpose: publish validated cache. Inputs: context/root. Outputs: error.
// Constraints: same coverage and grace checks. SPORT: license set.
func publishAfterValidation(projectDir string) error {
	names, err := installedLicensedNames("")
	if err != nil {
		return errs.Wrap("E125", "could not inspect installed plugin entitlements", err)
	}
	return publishToken(projectDir, false, names, "")
}

// validateNewKey requires an online result before replacing runtime entitlement.
// Purpose: preserve valid files offline. Inputs: context/project/key. Outputs: error.
// Constraints: only explicit server refusal clears the file. SPORT: key validation.
func validateNewKey(ctx context.Context, projectDir, key string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
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
	rejected := explicitRejection(remote.status, result)
	if result.CanProceed && !rejected {
		return nil
	}
	// FromCache alone cannot distinguish an absent cache from an auth rejection.
	if !rejected {
		return errs.New("E125", "could not validate the new licence online; existing entitlement retained")
	}
	if projectDir != "" {
		if writeErr := writeAtomic(Path(projectDir), nil); writeErr != nil {
			return errs.Wrap("E125", "could not clear the plugin entitlement file", writeErr)
		}
	}
	return errs.New("E125", "the new licence was rejected and could not obtain a verified entitlement")
}

// ValidateKeyOutsideProject captures the token now for the next project build.
// Purpose: avoid a stale-cache dead end. Inputs: context/key. Outputs: E125.
// Constraints: V15 only; offline saves the key for a later build retry; no file.
// SPORT: global license set.
func ValidateKeyOutsideProject(ctx context.Context, key string) error {
	// compat.V15(P7-PLUG-24): outside-project set saves only format -> validates online for the next project build
	if !compat.V15() || DevKeyMode() {
		return nil
	}
	return validateNewKey(ctx, "", key)
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
