package commands

// Purpose: the v1 envelope wiring of the licence commands (account canon
// fragment, P7-SURF-16). It wraps every RunE that hands its facts over with
// setData in dataEnv (the shared data-fragment emitter, db_envelope.go) and
// holds the helpers the bodies share.
// Inputs: the cobra commands of license.go and its split files, wrapped from
// init(). `license show` and `license health` print pre-contract JSON, so they
// emit themselves through emitEnv (legacy shape) and are not wrapped.
// Outputs: one envelope on stdout in v1.5 mode with --json; v1.4 output and
// every human output are unchanged.
// Constraints: a licence key reaches a document only masked (license.MaskKey);
// error text echoed into a document is scrubbed of every configured key; a
// JWT or signature from the cache never does. SPORT: P7-SURF-16,
// contract:cli.json-envelope

import (
	"errors"
	"regexp"
	"strings"

	"github.com/nself-org/cli/internal/license"
	"github.com/spf13/cobra"
)

// errNoCacheToExport is the `license export` failure when no validation was cached.
var errNoCacheToExport = errors.New("no license cache to export — run 'nself license validate' first")

// licenseDataEnvCmds are the commands whose RunE is wrapped with dataEnv.
func licenseDataEnvCmds() []*cobra.Command {
	return []*cobra.Command{
		licenseCmd, licenseSetCmd, licenseAddCmd, licenseRemoveCmd, licenseClearCmd,
		licenseStatusCmd, licenseListCmd, licenseValidateCmd, licenseUpgradeCmd,
		licenseRevalidateCmd, licenseRevokeCmd, licenseRestoreCmd, licenseMigrateCmd,
		licenseRefreshCmd, licenseExportCmd, licenseImportCmd, licenseSimulateCmd,
	}
}

func init() {
	for _, c := range licenseDataEnvCmds() {
		c.RunE = dataEnv(c.RunE)
	}
}

// licenseProductName is the display name of a detected product, or fallback.
func licenseProductName(pp *license.ProductPrefix, fallback string) string {
	if pp == nil {
		return fallback
	}
	return pp.DisplayName
}

// newLicenseStatusData is the empty status document: no member is nil.
func newLicenseStatusData() licenseStatusData {
	return licenseStatusData{Keys: []licenseStatusKey{}, Products: []string{}, Plugins: []string{}}
}

// licenseKeyShape matches a licence key (product prefix, then the secret part);
// jwtShape matches a JWT.
var (
	licenseKeyShape = regexp.MustCompile(`nself_[a-z]+_[A-Za-z0-9_-]{8,}`)
	jwtShape        = regexp.MustCompile(`eyJ[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]{5,}(\.[A-Za-z0-9_-]*)?`)
	longTokenRun    = regexp.MustCompile(`[A-Za-z0-9_.-]{24,}`)
)

// licenseScrubMax caps the length of server text copied into a document.
const licenseScrubMax = 200

// licenseScrub masks every licence key in s and drops every JWT, so text from
// a server reply, a network failure or a file error cannot carry a secret into
// a document: the configured keys, the explicit extra keys (a --key flag),
// anything shaped like a key or JWT, and any other 24-character token-like run
// (an access or session token has no fixed shape). Output is capped.
func licenseScrub(s string, extra ...string) string {
	for _, key := range append(license.CollectLicenseKeys(), extra...) {
		if key != "" {
			s = strings.ReplaceAll(s, key, license.MaskKey(key))
		}
	}
	s = jwtShape.ReplaceAllString(s, "[REDACTED]")
	s = licenseKeyShape.ReplaceAllStringFunc(s, license.MaskKey)
	s = longTokenRun.ReplaceAllString(s, "[REDACTED]")
	if r := []rune(s); len(r) > licenseScrubMax {
		s = string(r[:licenseScrubMax]) + "..."
	}
	return s
}

// emitLicenseShow writes the `license show` document in its legacy-compatible
// form; out is nil when no key is configured.
func emitLicenseShow(cmd *cobra.Command, out *licenseShowJSON) error {
	doc := licenseShowData{Bundles: []string{}}
	if out != nil {
		doc = licenseShowData{Configured: true, KeyPrefix: out.KeyPrefix, Tier: out.Tier, Bundles: nonNilStrings(out.Bundles),
			PluginsCount: out.PluginsCount, DaysToExpire: out.DaysToExpire, CacheExpiry: out.CacheExpiry}
	}
	return emitEnv(cmd, doc, true)
}

// exportCacheSummary answers `license export` in JSON mode: the cache summary,
// never the signed blob (it holds the licence JWT). The blob stays a human-mode
// output, so the command is CLI-only on the machine surfaces.
func exportCacheSummary(cmd *cobra.Command) error {
	entry, err := license.ReadCache()
	if err != nil {
		return err
	}
	if entry == nil {
		return errNoCacheToExport
	}
	setData(cmd, licenseExportData{Tier: entry.Tier, PluginsAllowed: nonNilStrings(entry.PluginsAllowed),
		FetchedAt: entry.FetchedAt, ExpiresAt: entry.ExpiresAt, Signed: entry.VerifySignature()})
	return nil
}
