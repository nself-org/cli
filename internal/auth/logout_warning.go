package auth

import (
	"errors"
	"fmt"
)

// LogoutRevokeWarning describes a best-effort revoke failure without suggesting
// login when the credentials are about to be deleted.
func LogoutRevokeWarning(err error) string {
	if errors.Is(err, ErrNotLoggedIn) {
		return "Could not revoke session on server: session already invalid on the server"
	}
	return fmt.Sprintf("Could not revoke session on server: %v", err)
}
