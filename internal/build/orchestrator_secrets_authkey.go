package build

// orchestrator_secrets_authkey.go — generate AUTH_ENCRYPTION_KEY once per new
// v1.5 project (D-0660).
// Purpose: nhost/auth 0.43.0 and later refuse to start without the key and
// encrypt every TOTP secret with it on first start. The v1.5 default renders the
// current auth release only when the env cascade carries a valid key
// (compose.AuthEntry), so a new project needs one before its first render.
// Inputs:  workdir, the build Sink, the loaded config and the keys already
//          defined in env files.
// Outputs: the new key (64 hex characters, 32 random bytes), which the caller
//          appends to .env.secrets; the key is also set in cfg.Passthrough so
//          this build renders the current release.
// Constraints: v1.5 only. Never generated when any env file or the process
//          environment names the key (valid or not: a malformed key keeps its
//          error), and never for a project that was built before
//          (.nself/compose.env exists): such a stack may already run
//          auth-v14 on a database that moves one way when the current release
//          starts, so it keeps its image until the operator sets the key.
//          Once written the key is never regenerated or rotated: losing or
//          changing it locks out every TOTP user. The value is never logged.

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
)

// newAuthEncryptionKey returns a fresh key for a new v1.5 project, or ok false
// when nothing must be generated.
func newAuthEncryptionKey(sink Sink, workdir string, cfg *config.Config, onDisk map[string]bool) (key string, ok bool, err error) {
	const name = compose.AuthEncryptionKeyEnv
	if !compat.V15() || onDisk[name] || os.Getenv(name) != "" {
		return "", false, nil
	}
	if _, set := cfg.Passthrough[name]; set {
		return "", false, nil
	}
	if _, readErr := sink.ReadFile(filepath.Join(workdir, composeEnvFile)); readErr == nil {
		return "", false, nil // built before: keep the image it runs
	}
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", false, fmt.Errorf("generating %s: %w", name, err)
	}
	key = hex.EncodeToString(raw[:])
	if cfg.Passthrough == nil {
		cfg.Passthrough = map[string]string{}
	}
	cfg.Passthrough[name] = key
	return key, true, nil
}
