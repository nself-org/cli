//go:build windows

package invoke

// MaxSecretEnvBytes bounds the SecretsEnv value. Windows allows 32,767
// characters in one environment variable; stay below it.
const MaxSecretEnvBytes = 30 << 10
