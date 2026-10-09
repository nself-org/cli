// Package errs is the sole home for structured, user-facing CLI errors and
// the process exit-code contract for the nSelf CLI.
//
// Purpose: give every command a single place to (a) classify a failure by a
// stable code (E001-E449, see codes.go and the codes_<topic>.go fragments), (b) attach a What/Why/Fix
// explanation via CLIError (structured.go), and (c) map that failure onto one
// of the four canonical process exit codes (exit_codes.go, exit_error.go) so
// wrappers (CI runners, schedulers, ops scripts) can branch on outcome class
// without parsing stderr.
//
// Inputs: sentinel errors declared in this file (errs.ErrDockerNotRunning
// etc.), or a Registry code passed to New/Newf/Wrap.
//
// Outputs: *CLIError (implements error, formats as "[CODE] What / Why / Fix")
// and *ExitError (implements error + ExitCode() int for main() to read).
//
// Constraints: named "errs" (not "errors") to avoid shadowing the Go standard
// library; other packages import as `nself-org/cli/internal/errs`. This
// package absorbed the sole responsibility for structured user-facing errors
// as of CLI-R14 (2026-08-23) — the parallel, unimported `internal/errors`
// catalog (ERR-INSTALL-*, ERR-LICENSE-*, etc. with a Message struct and its
// own HelpFooter) was deleted as a confirmed-dead duplicate: same shape
// (Code/What/Why/Fix), same category coverage (docker, config, plugin,
// license, ssl, database, health, init, domain), zero importers anywhere in
// the tree. Nothing from it was migrated because everything it did is already
// covered here via Registry + CLIError, and CLIError additionally carries the
// exit-code classification that the deleted package never had.
package errs

import "errors"

var (
	// Docker
	ErrDockerNotRunning   = errors.New("docker daemon is not running")
	ErrDockerNotInstalled = errors.New("docker not found in PATH")
	ErrComposeNotFound    = errors.New("docker-compose.yml not found — run 'nself build' first")
	ErrPortConflict       = errors.New("port already in use")

	// Config
	ErrWeakPassword       = errors.New("password does not meet minimum length")
	ErrInsecurePassword   = errors.New("password matches insecure pattern")
	ErrInvalidProjectName = errors.New("invalid project name format")
	ErrInvalidCORS        = errors.New("CORS wildcards not allowed in production")
	ErrPlaceholderSecret  = errors.New("secret is empty or matches a known placeholder value")

	// Health
	ErrServiceUnhealthy = errors.New("service health check failed")
	ErrHealthTimeout    = errors.New("health check timed out")
	ErrServiceNotFound  = errors.New("service not found")

	// Plugin
	ErrInvalidLicenseKey         = errors.New("invalid license key format")
	ErrLicenseTierTooLow         = errors.New("license tier does not include this plugin")
	ErrLicenseExpired            = errors.New("license key is expired")
	ErrLicenseNetworkUnavailable = errors.New("cannot validate license: network unavailable and no valid cache")
	ErrPluginNotFound            = errors.New("plugin not found in registry")
	ErrPluginManifest            = errors.New("invalid plugin manifest")
	ErrCircularDependency        = errors.New("circular plugin dependency detected")
	ErrPluginUnsigned            = errors.New("stable plugin is missing required signature — install refused")
	ErrPluginMissingChecksum     = errors.New("stable plugin is missing required checksum — install refused")
	ErrDuplicatePluginSlug       = errors.New("plugin slug is served by more than one unrelated registry entry")
	ErrTierNotEntitled           = errors.New("license does not entitle this Licensed plugin")

	// SSL
	ErrMkcertNotFound      = errors.New("mkcert not installed — falling back to OpenSSL")
	ErrSSLGenerationFailed = errors.New("SSL certificate generation failed")

	// Nginx
	ErrDuplicateRoute = errors.New("duplicate nginx route detected")

	// Domain / Port
	ErrInvalidDomain = errors.New("invalid domain name")
	ErrInvalidPort   = errors.New("invalid port number")

	// Database
	ErrDatabaseNotRunning           = errors.New("database is not running")
	ErrMigrationFailed              = errors.New("database migration failed")
	ErrMigrationValidationFailed    = errors.New("migration dry-run validation failed")
	ErrMigrationPrerequisiteMissing = errors.New("migration prerequisite object missing from live schema")
	ErrBackupFailed                 = errors.New("database backup failed")
	ErrBackupNotFound               = errors.New("backup not found")
	ErrBackupVerifyFailed           = errors.New("backup verification failed")
	ErrBackupRestoreFailed          = errors.New("backup restore failed")
	ErrBackupEncryptFailed          = errors.New("backup encryption failed")
	ErrBackupDecryptFailed          = errors.New("backup decryption failed")
	ErrBackupRemoteFailed           = errors.New("remote backup operation failed")
	ErrBackupPruneFailed            = errors.New("backup pruning failed")
	ErrWALArchiveFailed             = errors.New("WAL archive failed")

	// Disaster Recovery
	ErrDRDrillFailed    = errors.New("DR drill failed")
	ErrDRPromoteFailed  = errors.New("standby promotion failed")
	ErrDRRollbackFailed = errors.New("DR rollback failed")
	ErrDRFenceFailed    = errors.New("split-brain fence failed")
)
