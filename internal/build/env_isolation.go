package build

// env_isolation.go — keep a build from changing the process environment where
// that would change what a later step reads (P7-LIVE-03, review M1).
//
// Purpose: config.Load exports every .env cascade file into the process
// environment (godotenv.Overload). A plan-mode build therefore left the
// project's values in os.Environ, and the write build that followed resolved
// ENV and NSELF_V15 from that leftover state instead of from the operator's own
// environment. Plan mode now restores the environment it found, and every build
// pins the compat switch across config.Load so a project file cannot toggle
// v1.5 behaviour mid-command.
// Inputs: none. Outputs: restore functions. Constraints: process-wide, not
// goroutine safe (a build is one command's work).

import (
	"os"
	"strings"

	"github.com/nself-org/cli/internal/compat"
)

// SnapshotEnv captures the environment and returns a function that restores it
// exactly: variables added since are unset, changed or removed ones reset.
func SnapshotEnv() (restore func()) {
	saved := os.Environ()
	return func() {
		keep := make(map[string]string, len(saved))
		for _, kv := range saved {
			if k, v, ok := strings.Cut(kv, "="); ok {
				keep[k] = v
			}
		}
		for _, kv := range os.Environ() {
			if k, _, ok := strings.Cut(kv, "="); ok {
				if _, was := keep[k]; !was {
					_ = os.Unsetenv(k)
				}
			}
		}
		for k, v := range keep {
			if os.Getenv(k) != v {
				_ = os.Setenv(k, v)
			}
		}
	}
}

// pinCompatEnv records the compat switch as the operator set it and returns a
// function that puts it back, for use right after config.Load.
func pinCompatEnv() (restore func()) {
	v, had := os.LookupEnv(compat.EnvVar)
	return func() {
		if had {
			_ = os.Setenv(compat.EnvVar, v)
		} else {
			_ = os.Unsetenv(compat.EnvVar)
		}
	}
}
