package config

// Purpose: isolate Compose interpolation from the config loader's exported cascade.
// Inputs: a synchronous Docker Compose operation.
// Outputs: the operation's error, with the caller's environment restored.
// Constraints: retain Docker transport and executable discovery; project values
// come from --env-file. Callbacks must not spawn background work or load config.

import (
	"os"
	"strings"
	"sync"
)

var composeEnvironmentMu sync.Mutex

// WithComposeEnvironment runs one operation with only runtime/transport env.
// The process environment is restored on every exit, including a panic. The
// mutex serializes these short lifecycle operations; config loads stay outside.
func WithComposeEnvironment(operation func() error) error {
	composeEnvironmentMu.Lock()
	defer composeEnvironmentMu.Unlock()
	before := os.Environ()
	defer func() {
		os.Clearenv()
		for _, entry := range before {
			key, value, _ := strings.Cut(entry, "=")
			_ = os.Setenv(key, value)
		}
	}()
	os.Clearenv()
	for _, entry := range before {
		key, value, _ := strings.Cut(entry, "=")
		if composeRuntimeVariable(key) {
			_ = os.Setenv(key, value)
		}
	}
	return operation()
}

// composeRuntimeVariable keeps executable discovery, Docker context/auth,
// daemon transport, proxy and SSH-agent settings. COMPOSE_* and project
// configuration deliberately do not enter Compose's interpolation environment.
func composeRuntimeVariable(key string) bool {
	switch key {
	case "PATH", "HOME", "USERPROFILE", "SYSTEMROOT", "WINDIR", "PATHEXT",
		"TMPDIR", "TMP", "TEMP", "XDG_CONFIG_HOME", "XDG_RUNTIME_DIR",
		"DOCKER_HOST", "DOCKER_CONTEXT", "DOCKER_CONFIG", "DOCKER_CERT_PATH",
		"DOCKER_TLS", "DOCKER_TLS_VERIFY", "DOCKER_API_VERSION",
		"SSH_AUTH_SOCK", "SSH_AGENT_PID", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY",
		"ALL_PROXY", "http_proxy", "https_proxy", "no_proxy", "all_proxy",
		"SSL_CERT_FILE", "SSL_CERT_DIR":
		return true
	}
	return false
}
