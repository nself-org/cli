package errs

import "errors"

// sentinelCode pairs one exported sentinel error with the registry code it
// stands for.
type sentinelCode struct {
	Err  error
	Code string
}

// sentinelTable is the one sentinel-to-code mapping (contract:cli.error-codes
// v1). Every exported Err* variable in this package has exactly one row; an
// AST test fails when one is added without a code. Order matters only for
// CodeForSentinel, which returns the first errors.Is match.
var sentinelTable = []sentinelCode{
	// Docker
	{ErrDockerNotInstalled, "E001"},
	{ErrDockerNotRunning, "E002"},
	{ErrComposeNotFound, "E004"},
	{ErrPortConflict, "E005"},
	// Config
	{ErrWeakPassword, "E053"},
	{ErrInvalidProjectName, "E054"},
	{ErrDuplicateRoute, "E055"},
	{ErrInsecurePassword, "E057"},
	{ErrInvalidCORS, "E058"},
	{ErrPlaceholderSecret, "E059"},
	// Plugin / license
	{ErrPluginNotFound, "E100"},
	{ErrInvalidLicenseKey, "E101"},
	{ErrLicenseTierTooLow, "E102"},
	{ErrLicenseExpired, "E103"},
	{ErrLicenseNetworkUnavailable, "E104"},
	{ErrCircularDependency, "E105"},
	{ErrPluginManifest, "E106"},
	{ErrPluginUnsigned, "E107"},
	{ErrPluginMissingChecksum, "E108"},
	{ErrDuplicatePluginSlug, "E109"},
	{ErrTierNotEntitled, "E110"},
	// SSL
	{ErrMkcertNotFound, "E150"},
	{ErrSSLGenerationFailed, "E151"},
	// Database, backup, disaster recovery
	{ErrDatabaseNotRunning, "E200"},
	{ErrMigrationFailed, "E201"},
	{ErrBackupFailed, "E202"},
	{ErrMigrationValidationFailed, "E203"},
	{ErrMigrationPrerequisiteMissing, "E204"},
	{ErrBackupNotFound, "E205"},
	{ErrBackupVerifyFailed, "E206"},
	{ErrBackupRestoreFailed, "E207"},
	{ErrBackupEncryptFailed, "E208"},
	{ErrBackupDecryptFailed, "E209"},
	{ErrBackupRemoteFailed, "E210"},
	{ErrBackupPruneFailed, "E211"},
	{ErrWALArchiveFailed, "E212"},
	{ErrDRDrillFailed, "E213"},
	{ErrDRPromoteFailed, "E214"},
	{ErrDRRollbackFailed, "E215"},
	{ErrDRFenceFailed, "E216"},
	// Health
	{ErrServiceUnhealthy, "E250"},
	{ErrHealthTimeout, "E251"},
	{ErrServiceNotFound, "E252"},
	// Domain / port
	{ErrInvalidDomain, "E350"},
	{ErrInvalidPort, "E351"},
	// CLI safety gate
	{ErrDestructiveBlocked, "E403"},
}

// CodeForSentinel returns the registry code of the first sentinel in the
// table that err matches through errors.Is, and whether there was a match.
//
// Purpose: the single place that turns a wrapped sentinel into an error code,
// used by ExitCodeFor and Describe.
//
// Inputs: any error, possibly wrapped with %w or joined.
//
// Outputs: (code, true) on a match; ("", false) for nil or an unmapped error.
func CodeForSentinel(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	for _, s := range sentinelTable {
		if errors.Is(err, s.Err) {
			return s.Code, true
		}
	}
	return "", false
}
