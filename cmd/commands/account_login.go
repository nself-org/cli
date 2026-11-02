package commands

// Purpose: the "nself account login" subcommand and its RunE. Inputs are the
// cobra command/args (credentials/flags); outputs are a persisted auth file
// or an error.
// Constraints: split out of account.go (CLI-R12) as a pure move, no behavior change.

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/nself-org/cli/internal/auth"
	"github.com/nself-org/cli/internal/ui"
	"github.com/spf13/cobra"
)

var accountLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in to your nSelf account",
	Long: `Log in to your nSelf account using the device authorization flow.

Opens your browser to authorize this CLI session. Confirm the short code
shown here matches the one on screen, then click Authorize.

After authorization, your session is stored at ~/.nself/auth.json (0600).`,
	Deprecated:   "use nself login (nself license login in v1.5); replace --reauth with --force; --timeout is only supported by this legacy spelling",
	SilenceUsage: true,
	RunE:         runAccountLogin,
}

func init() {
	accountLoginCmd.Flags().Bool("reauth", false, "Force new auth even if already logged in")
	accountLoginCmd.Flags().Bool("no-browser", false, "Print URL and code; do not open browser")
	accountLoginCmd.Flags().Int("timeout", 300, "Device-code poll timeout in seconds")
}

func runAccountLogin(cmd *cobra.Command, _ []string) error {
	reauth, _ := cmd.Flags().GetBool("reauth")
	noBrowser, _ := cmd.Flags().GetBool("no-browser")
	timeoutSec, _ := cmd.Flags().GetInt("timeout")

	// NSELF_NO_BROWSER env var overrides flag.
	if os.Getenv("NSELF_NO_BROWSER") == "1" {
		noBrowser = true
	}

	// Guard: already logged in.
	if !reauth && auth.IsLoggedIn() {
		af, err := auth.ReadAuthFile()
		if err == nil && af != nil {
			ui.Warn(fmt.Sprintf("Already logged in as %s. Use --reauth to force.", af.Email))
			return nil
		}
	}

	// Step 1: request device code.
	ui.Info("Contacting nSelf auth server...")
	dcResp, err := auth.DeviceAuthorize(cmdCtx(cmd))
	if err != nil {
		return fmt.Errorf("cannot reach nself.org — check your connection: %w", err)
	}

	// Step 2: show user code and verification URL.
	verificationURL := dcResp.VerificationURL
	if verificationURL == "" {
		verificationURL = fmt.Sprintf("%s?code=%s", auth.CLIAuthBaseURL(), dcResp.UserCode)
	}

	fmt.Printf("\n%s Your login code: %s\n", ui.C(ui.Bold, "»"), ui.C(ui.BrightYellow, dcResp.UserCode))
	fmt.Printf("%s Open: %s\n\n", ui.C(ui.Bold, "»"), ui.C(ui.Cyan, verificationURL))

	if !noBrowser {
		openBrowser(verificationURL)
		fmt.Println("Browser opened. Waiting for authorization...")
	} else {
		fmt.Println("Open the URL above in your browser, then wait here for confirmation.")
	}

	// Step 3: poll for the token.
	// cmdCtx never returns nil (falls back to context.Background()), so no
	// nil-check is needed here.
	baseCtx := cmdCtx(cmd)
	timeout := time.Duration(timeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(baseCtx, timeout+time.Minute)
	defer cancel()

	var lastDot time.Time
	result, err := auth.PollDeviceCode(ctx, dcResp.DeviceCode, func(elapsed time.Duration) {
		if time.Since(lastDot) >= 10*time.Second {
			fmt.Print(".")
			lastDot = time.Now()
		}
	})
	fmt.Println()

	if err != nil {
		return fmt.Errorf("login: %w", err)
	}

	// Step 4: persist credentials.
	af := &auth.AuthFile{
		AccessToken:  result.Token.AccessToken,
		SessionToken: result.Token.SessionToken,
		Email:        result.Token.Email,
		Tier:         result.Token.Tier,
		DisplayName:  result.Token.DisplayName,
		Bundles:      result.Token.Bundles,
		ExpiresAt:    result.Token.ExpiresAt,
	}
	if err := auth.WriteAuthFile(af); err != nil {
		return fmt.Errorf("saving credentials: %w", err)
	}

	tier := result.Token.Tier
	if tier == "" {
		tier = "free"
	}
	ui.Success(fmt.Sprintf("Logged in as %s (%s).", result.Token.Email, tier))

	if len(result.Token.Bundles) > 0 {
		fmt.Printf("Bundles: %v\n", result.Token.Bundles)
	}

	return nil
}

// ── account logout ───────────────────────────────────────────────────────────
