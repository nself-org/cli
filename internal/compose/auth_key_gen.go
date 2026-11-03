package compose

// auth_key_gen.go — generates the AUTH_ENCRYPTION_KEY a fresh project needs (D-0660).
//
// Purpose: nhost/auth 0.43+ refuses to start without a 32-byte key, so the build
// synthesizes one when no env file and no process export supplies it.
// Inputs: the config (its passthrough map), whether an env file already defines
// the key; the byte source is config.SecretSource (seeded in a plan).
// Outputs: the new key as 64 lowercase hex characters, or "" when none is needed.
// Constraints: the key is also stored in the passthrough so this build renders
// what the next build (which loads it from .env.secrets) will; never logged.

import (
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"github.com/nself-org/cli/internal/config"
)

// NewAuthEncryptionKey returns a fresh key, or "" when onDisk is true or the real
// process environment already supplies one (an env file would shadow a later
// rotated export, so a process-owned value is never persisted).
func NewAuthEncryptionKey(cfg *config.Config, onDisk bool) (string, error) {
	if onDisk || os.Getenv(AuthEncryptionKeyEnv) != "" {
		return "", nil
	}
	b := make([]byte, 32)
	if _, err := io.ReadFull(config.SecretSource(), b); err != nil {
		return "", fmt.Errorf("generating auth encryption key: %w", err)
	}
	if cfg.Passthrough == nil {
		cfg.Passthrough = map[string]string{}
	}
	cfg.Passthrough[AuthEncryptionKeyEnv] = hex.EncodeToString(b)
	return cfg.Passthrough[AuthEncryptionKeyEnv], nil
}
