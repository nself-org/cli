package config

// env_overlay.go — the env-cascade overlay of LoadOptions (P7-LIVE-05).
//
// Purpose: let a caller load the cascade as it will be after a pending change
// to one env file, without writing that file first. `nself config set` plans
// the regeneration a new value causes before it writes the line; the planner
// passes the file's would-be bytes here and the loader reads them in place of
// the disk copy.
// Inputs: the project dir and LoadOptions.EnvOverlay (cascade file name, e.g.
// ".env.secrets", to the bytes that replace it). Outputs: the ENV choice and
// the process environment exactly as godotenv.Overload would leave them.
// Constraints: an overlaid name is parsed with the same parser Overload uses
// (godotenv.UnmarshalBytes is what Overload's file read calls) and gets the
// same size and null-byte checks. A name the overlay does not hold is read
// from disk by the unchanged path in loader.go.

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// maxEnvFileSize bounds one cascade file (1 MB), on disk or overlaid.
const maxEnvFileSize = 1 << 20

// resolveEnvWith is ResolveEnv with the overlay's .env standing in for the
// file on disk when the overlay holds one.
func resolveEnvWith(projectDir string, overlay map[string][]byte) (value string, source string) {
	data, ok := overlay[".env"]
	if !ok || os.Getenv("ENV") != "" {
		return ResolveEnv(projectDir)
	}
	if m, err := godotenv.UnmarshalBytes(data); err == nil && m["ENV"] != "" {
		return m["ENV"], ".env"
	}
	return "dev", "default"
}

// loadOverlaid exports one overlaid cascade file into the process environment
// (later files override earlier ones, as godotenv.Overload does) and records
// its keys in loaded for the unknown-variable warning.
func loadOverlaid(projectDir, name string, data []byte, loaded map[string]string) error {
	f := filepath.Join(projectDir, name)
	if len(data) > maxEnvFileSize {
		return fmt.Errorf("config file too large (max 1MB): %s", f)
	}
	if pos := bytes.IndexByte(data, 0); pos >= 0 {
		return fmt.Errorf("invalid .env file: contains null byte at position %d in %s", pos, f)
	}
	return exportEnvFile(f, data, loaded)
}

// OverloadEnvFile exports one env file at path into the process environment
// the way Load exports a cascade file: a file value replaces an existing one,
// and a protected security key is skipped. A missing file is an error, as
// with godotenv.Overload, which callers outside Load used before.
func OverloadEnvFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return loadOverlaid(filepath.Dir(path), filepath.Base(path), data, map[string]string{})
}

// exportEnvFile parses one cascade file's bytes and exports every key into the
// process environment (overload semantics: a file value replaces an existing
// one), recording the keys in loaded for the unknown-variable warning.
//
// Inputs: the file path (for messages only), its bytes, and the loaded map.
// Outputs: nil, or an error. A parse failure returns "invalid dotenv syntax in
// <path>" and never carries the parser text, because godotenv quotes the
// offending line, which can hold a secret. Constraints: the size and null-byte
// checks are the caller's; nothing is exported when the parse fails; a key the
// OS refuses (an empty name) is skipped and not recorded, as godotenv.Overload
// ignores that error. A protected security key (env_protected.go) is never
// exported or recorded: only the process environment may set it.
func exportEnvFile(f string, data []byte, loaded map[string]string) error {
	m, err := godotenv.UnmarshalBytes(data)
	if err != nil {
		return fmt.Errorf("invalid dotenv syntax in %s", f)
	}
	for k, v := range m {
		if skipProtectedKey(f, k, v) {
			continue
		}
		if os.Setenv(k, v) != nil {
			continue // a key the OS rejects (empty name): skipped, as godotenv.Overload does
		}
		loaded[k] = v
	}
	return nil
}
