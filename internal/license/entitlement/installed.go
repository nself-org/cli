package entitlement

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/httptimeout"
	"github.com/nself-org/cli/internal/license"
)

// ErrValidationUnavailable means a saved key could not be checked online.
// Callers may warn and retry at the next build without failing the key save.
var ErrValidationUnavailable = errors.New("could not validate the new licence online; existing entitlement retained")

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
// Purpose: report direct refresh failure. Inputs: error/warnings.
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
	return publishToken(projectDir, names)
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
		return fmt.Errorf("%w: %v", ErrValidationUnavailable, err)
	}
	if result.FromCache || result.Status == license.StatusFailOpen {
		return ErrValidationUnavailable
	}
	// The validator treats other 4xx as auth failure too; only 401, ping-shaped
	// 403, or a signed rejection revoke this runtime entitlement.
	rejected := explicitRejection(remote, result)
	if result.CanProceed && !rejected {
		_ = recordVerdict(license.HashKey(key), false) // a stale rejection only fails closed
		return nil
	}
	// FromCache alone cannot distinguish an absent cache from an auth rejection.
	if !rejected {
		return ErrValidationUnavailable
	}
	recordErr := recordVerdict(license.HashKey(key), true)
	if projectDir != "" {
		if writeErr := publishAfterKeyRejection(projectDir, key); writeErr != nil {
			return writeErr
		}
		if recordErr != nil {
			return errs.Wrap("E125", "the licence rejection could not be recorded and may not hold offline", recordErr)
		}
		return errs.New("E125", "the new licence was rejected and could not obtain a verified entitlement")
	}
	if recordErr != nil {
		return fmt.Errorf("the licence rejection could not be recorded and may not hold offline: %w", recordErr)
	}
	return errors.New("the new licence was rejected and could not obtain a verified entitlement")
}

// ValidateKeyOutsideProject captures the token now for the next project build.
// Purpose: avoid a stale-cache dead end. Inputs: context/key. Outputs: validation error.
// Constraints: V15 only; offline saves the key for a later build retry; no file.
// SPORT: global license set.
func ValidateKeyOutsideProject(ctx context.Context, key string) error {
	// compat.V15(P7-PLUG-24): outside-project set saves only format -> validates online for the next project build
	if !compat.V15() || DevKeyMode() {
		return nil
	}
	return validateNewKey(ctx, "", key)
}
