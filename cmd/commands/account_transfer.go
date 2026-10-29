package commands

// Purpose: the "nself account transfer" subcommand plus the small shared
// helpers (cmdCtx, requireLogin, handleAuthError, promptConfirm) used across
// account subcommands. Inputs are the cobra command/args; outputs are transfer
// results, a context, an *auth.AuthFile, or a bool prompt answer.
// Constraints: split out of account.go (CLI-R12) as a pure move, no behavior change.

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/auth"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/ui"
	"github.com/spf13/cobra"
)

var accountTransferCmd = &cobra.Command{
	Use:   "transfer <key-id> <email>",
	Short: "Transfer a license to another account",
	Long: `Transfer a license from your account to another email address.

The recipient will receive an email to accept the transfer.`,
	SilenceUsage: true,
	Args:         cobra.ExactArgs(2),
	RunE:         runAccountTransfer,
}

func runAccountTransfer(cmd *cobra.Command, args []string) error {
	keyID := args[0]
	toEmail := args[1]

	af, err := requireLogin()
	if err != nil {
		return err
	}

	prefix := keyID
	if len(prefix) > 12 {
		prefix = prefix[:12] + "..."
	}
	msg := fmt.Sprintf("Transfer license %s to %s? This removes it from your account. [y/N]: ", prefix, toEmail)
	if !promptConfirm(msg) {
		fmt.Println("Canceled.")
		return nil
	}

	if err := auth.TransferLicense(cmdCtx(cmd), af.AccessToken, keyID, toEmail); err != nil {
		return handleAuthError(err)
	}

	ui.Success(fmt.Sprintf("License transferred. %s will receive an email to accept.", toEmail))
	return nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// cmdCtx returns cmd.Context() or context.Background() if the command was
// invoked outside the normal cobra Execute() path (e.g. unit tests).
func cmdCtx(cmd *cobra.Command) context.Context {
	if c := cmd.Context(); c != nil {
		return c
	}
	return context.Background()
}

// requireLogin reads the auth file and returns it, or prints a friendly error.
func requireLogin() (*auth.AuthFile, error) {
	af, err := auth.ReadAuthFile()
	if err != nil {
		if err == auth.ErrNotLoggedIn {
			return nil, fmt.Errorf("not logged in — run: nself account login")
		}
		return nil, fmt.Errorf("reading credentials: %w", err)
	}
	return af, nil
}

// handleAuthError converts auth API errors to user-friendly messages.
func handleAuthError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, auth.ErrNotLoggedIn) {
		return errAccountRelogin()
	}
	var lacks *auth.ServerLacksPathError
	if errors.As(err, &lacks) {
		return errs.Newf("E225", "this server does not provide %s", lacks.Path)
	}
	if apiErr, ok := err.(*auth.AuthAPIError); ok {
		switch apiErr.Status {
		case 403:
			return fmt.Errorf("permission denied — your tier does not include this feature")
		case 429:
			return fmt.Errorf("rate limited — try again in a moment")
		default:
			if apiErr.Status >= 500 {
				return fmt.Errorf("nself.org returned an error — try again later")
			}
		}
	}
	msg := err.Error()
	if strings.Contains(msg, "connection refused") || strings.Contains(msg, "no such host") {
		return fmt.Errorf("cannot reach nself.org — check your connection")
	}
	return err
}

// promptConfirm prints msg and reads y/Y from stdin. Returns false on any other input.
func promptConfirm(msg string) bool {
	fmt.Print(msg)
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes"
}
