package commands

// Purpose: License validation/health/upgrade commands split out of
// license.go (CLI-R12 Batch B mechanical file-size split). Holds `nself
// license validate/upgrade/health/revalidate`.
// Inputs: cobra command flags (--json) and the keys returned by
// license.CollectLicenseKeys().
// Outputs: stdout/JSON validation and health reports, or opening the
// pricing page in the OS browser; errors wrap license/plugin failures.
// Constraints: pure move, no behavior change. defaultPingURL/pricingURL
// consts and the licenseCmd/init() registration remain in license.go.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/license"
	"github.com/nself-org/cli/internal/plugin"

	"github.com/spf13/cobra"
)

var licenseValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate key against ping.nself.org",
	RunE: func(cmd *cobra.Command, args []string) error {
		keys := license.CollectLicenseKeys()
		if len(keys) == 0 {
			return fmt.Errorf("no license key configured — run 'nself license add <key>' first")
		}

		pingURL := os.Getenv("NSELF_PING_API_URL")
		if pingURL == "" {
			pingURL = defaultPingURL
		}

		allValid := true
		validKeys := []string{}
		for _, key := range keys {
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			valid, err := plugin.ValidateLicenseRemote(ctx, key, pingURL)
			cancel()

			masked := license.MaskKey(key)
			if err != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s: validation failed: %v\n", masked, licenseScrub(err.Error(), key))
				allValid = false
			} else if valid {
				validKeys = append(validKeys, masked)
				fmt.Printf("%s: valid\n", masked)
			} else {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s: invalid or expired\n", masked)
				allValid = false
			}
		}

		if !allValid {
			return fmt.Errorf("one or more keys failed validation")
		}
		setData(cmd, licenseValidateData{Keys: validKeys})
		return nil
	},
}

var licenseUpgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Open pricing page in browser",
	RunE: func(cmd *cobra.Command, args []string) error {
		if jsonEnvelopeOn(cmd) { // an agent run starts no browser; the page is in the document
			setData(cmd, licenseUpgradeData{URL: pricingURL})
			return nil
		}
		var openCmd string
		switch runtime.GOOS {
		case "darwin":
			openCmd = "open"
		case "linux":
			openCmd = "xdg-open"
		default:
			return fmt.Errorf("unsupported platform %s — visit %s manually", runtime.GOOS, pricingURL)
		}

		if err := exec.Command(openCmd, pricingURL).Start(); err != nil {
			return fmt.Errorf("opening browser: %w", err)
		}
		fmt.Printf("Opening %s\n", pricingURL)
		return nil
	},
}

// licenseHealthJSON is the structured output for `nself license health --json`.
type licenseHealthJSON struct {
	FormatOK        bool   `json:"format_ok"`
	ServerReachable bool   `json:"server_reachable"`
	ServerValid     bool   `json:"server_valid"`
	CacheIntegrity  bool   `json:"cache_integrity"`
	HealthScore     int    `json:"health_score"`
	Status          string `json:"status"`
	Message         string `json:"message,omitempty"`
}

var licenseHealthCmd = &cobra.Command{
	Use:   "health",
	Short: "Validate format, ping server, and report cache integrity",
	Long: `Run a combined license health check:

  1. Format validation — key prefix and length.
  2. Server ping — reach ping.nself.org and validate the key.
  3. Cache integrity — verify Ed25519 signature on cached entry.

Exits 0 when all checks pass, 1 when any check fails.
Use --json for machine-readable output.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonMode, _ := cmd.Flags().GetBool("json")

		keys := license.CollectLicenseKeys()
		if len(keys) == 0 {
			if jsonMode {
				out := licenseHealthJSON{Status: "no_key", Message: "no license key configured"}
				if jsonEnvelopeOn(cmd) {
					return emitDoc(cmd, out, true)
				}
				return json.NewEncoder(os.Stdout).Encode(out)
			}
			fmt.Println("No license key configured.")
			return fmt.Errorf("no license key — run 'nself license add <key>'")
		}

		key := keys[0]

		// 1. Format check.
		formatOK := license.ValidateKeyFormat(key) == nil

		// 2. Server ping.
		pingURL := os.Getenv("NSELF_PING_API_URL")
		if pingURL == "" {
			pingURL = defaultPingURL
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
		serverValid, pingErr := plugin.ValidateLicenseRemote(ctx, key, pingURL)
		cancel()
		serverReachable := pingErr == nil

		// 3. Cache integrity.
		cacheOK := false
		if cache, err := license.ReadCache(); err == nil && cache != nil {
			cacheOK = cache.VerifySignature()
		}

		// Health score: 0-3 checks passing.
		score := 0
		if formatOK {
			score++
		}
		if serverReachable && serverValid {
			score++
		}
		if cacheOK {
			score++
		}

		status := "unhealthy"
		switch score {
		case 3:
			status = "healthy"
		case 2:
			status = "degraded"
		}

		var msgParts []string
		if !formatOK {
			msgParts = append(msgParts, "key format invalid")
		}
		if !serverReachable {
			msgParts = append(msgParts, "server unreachable")
		} else if !serverValid {
			msgParts = append(msgParts, "key invalid or expired on server")
		}
		if !cacheOK {
			msgParts = append(msgParts, "cache signature invalid or missing")
		}
		message := strings.Join(msgParts, "; ")

		if jsonMode {
			out := licenseHealthJSON{
				FormatOK:        formatOK,
				ServerReachable: serverReachable,
				ServerValid:     serverValid,
				CacheIntegrity:  cacheOK,
				HealthScore:     score,
				Status:          status,
				Message:         message,
			}
			if jsonEnvelopeOn(cmd) {
				return emitDoc(cmd, out, true)
			}
			return json.NewEncoder(os.Stdout).Encode(out)
		}

		check := func(ok bool) string {
			if ok {
				return "✓"
			}
			return "✗"
		}
		fmt.Printf("Format valid:      %s\n", check(formatOK))
		fmt.Printf("Server reachable:  %s\n", check(serverReachable))
		fmt.Printf("Server valid:      %s\n", check(serverReachable && serverValid))
		fmt.Printf("Cache integrity:   %s\n", check(cacheOK))
		fmt.Printf("Health score:      %d/3\n", score)
		fmt.Printf("Status:            %s\n", status)
		if message != "" {
			fmt.Printf("Issues:            %s\n", message)
		}

		if score < 3 {
			return fmt.Errorf("license health check failed (score %d/3)", score)
		}
		return nil
	},
}

var licenseRevalidateCmd = &cobra.Command{
	Use:   "revalidate",
	Short: "Force a fresh validation against ping.nself.org and update the cache",
	Long: `Revalidate the current license key against ping.nself.org and refresh
the local cache. Useful after cache expiry or when reconnecting after offline use.

Requires network connectivity.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		keys := license.CollectLicenseKeys()
		if len(keys) == 0 {
			return fmt.Errorf("no license key configured — run 'nself license add <key>' first")
		}

		ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
		defer cancel()

		result, err := license.RefreshCache(ctx, keys[0])
		if err != nil {
			return scrubErr(fmt.Errorf("revalidation failed: %w", err))
		}
		if !result.Valid {
			return fmt.Errorf("license is invalid: %s", licenseScrub(result.Message))
		}

		rev := licenseRevalidateData{Tier: licenseScrub(result.Tier)}
		if !result.ExpiresAt.IsZero() {
			rev.ExpiresAt = result.ExpiresAt.Format("2006-01-02")
		}
		setData(cmd, rev)
		fmt.Printf("License revalidated. Tier: %s\n", rev.Tier)
		if !result.ExpiresAt.IsZero() {
			fmt.Printf("Expires: %s\n", result.ExpiresAt.Format("2006-01-02"))
		}
		fmt.Println("Cache updated.")
		return nil
	},
}
