//go:build !windows

package invoke

// MaxSecretEnvBytes bounds the SecretsEnv value: one environment string is
// capped near 128 KiB on Linux, and a bigger one would make exec fail.
const MaxSecretEnvBytes = 96 << 10
