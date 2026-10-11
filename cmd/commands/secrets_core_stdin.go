package commands

// Purpose: read the VALUE of `nself secrets set` without it touching argv.
// Inputs: the command's input stream and the secret key name.
// Outputs: the value text, or an error when none is given.
// Constraints: a terminal gets a no-echo prompt on stderr; any other input is
// read to EOF, capped at maxSecretStdinBytes, with one trailing "\n" or "\r\n"
// trimmed. An empty value is refused. The prompt never goes to stdout, which
// may carry the --json envelope.

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// maxSecretStdinBytes caps a secret value read from stdin.
const maxSecretStdinBytes = 1 << 20

// readSecretValue returns the secret value for key from cmd's input.
func readSecretValue(cmd *cobra.Command, key string) (string, error) {
	in := cmd.InOrStdin()
	if f, ok := in.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		fmt.Fprintf(cmd.ErrOrStderr(), "Enter value for %s: ", key)
		raw, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(cmd.ErrOrStderr())
		if err != nil {
			return "", fmt.Errorf("reading value: %w", err)
		}
		return nonEmptySecret(string(raw))
	}
	raw, err := io.ReadAll(io.LimitReader(in, maxSecretStdinBytes+1))
	if err != nil {
		return "", fmt.Errorf("reading value from stdin: %w", err)
	}
	if len(raw) > maxSecretStdinBytes {
		return "", fmt.Errorf("the value on stdin is larger than %d bytes", maxSecretStdinBytes)
	}
	text := strings.TrimSuffix(string(raw), "\n")
	if len(text) < len(raw) {
		text = strings.TrimSuffix(text, "\r")
	}
	return nonEmptySecret(text)
}

func nonEmptySecret(v string) (string, error) {
	if v == "" {
		return "", errors.New("the secret value is empty: pass it on stdin or at the prompt")
	}
	return v, nil
}
