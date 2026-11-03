package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
)

func TestAuthEncryptionKeyFresh(t *testing.T) {
	workdir := t.TempDir()
	cfg := &config.Config{}

	err := persistGeneratedSecrets(workdir, cfg)
	if err != nil {
		t.Fatalf("persistGeneratedSecrets: %v", err)
	}

	secretsPath := filepath.Join(workdir, ".env.secrets")
	data, err := os.ReadFile(secretsPath)
	if err != nil {
		t.Fatalf("reading .env.secrets: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, compose.AuthEncryptionKeyEnv+"=") {
		t.Fatalf("AUTH_ENCRYPTION_KEY not written to .env.secrets, got:\n%s", content)
	}

	lines := strings.Split(content, "\n")
	var value string
	for _, line := range lines {
		if strings.HasPrefix(line, compose.AuthEncryptionKeyEnv+"=") {
			value = strings.TrimPrefix(line, compose.AuthEncryptionKeyEnv+"=")
			break
		}
	}

	if len(value) != 64 {
		t.Errorf("expected 64 hex chars, got %d: %q", len(value), value)
	}

	// Verify it's lowercase hex
	for _, c := range value {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("invalid hex char %q in value %q", c, value)
			break
		}
	}

	// Check file mode
	info, err := os.Stat(secretsPath)
	if err != nil {
		t.Fatalf("stat .env.secrets: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected mode 0600, got %v", info.Mode().Perm())
	}
}

func TestAuthEncryptionKeySecondRun(t *testing.T) {
	workdir := t.TempDir()
	cfg := &config.Config{}

	// First run
	err := persistGeneratedSecrets(workdir, cfg)
	if err != nil {
		t.Fatalf("first persistGeneratedSecrets: %v", err)
	}

	secretsPath := filepath.Join(workdir, ".env.secrets")
	data1, err := os.ReadFile(secretsPath)
	if err != nil {
		t.Fatalf("reading .env.secrets after first run: %v", err)
	}

	// Second run
	err = persistGeneratedSecrets(workdir, cfg)
	if err != nil {
		t.Fatalf("second persistGeneratedSecrets: %v", err)
	}

	data2, err := os.ReadFile(secretsPath)
	if err != nil {
		t.Fatalf("reading .env.secrets after second run: %v", err)
	}

	if string(data1) != string(data2) {
		t.Errorf("second run changed .env.secrets:\nbefore:\n%s\nafter:\n%s", data1, data2)
	}
}

func TestAuthEncryptionKeyAlreadyInEnv(t *testing.T) {
	workdir := t.TempDir()
	cfg := &config.Config{Env: "dev"}

	// Create .env.dev with the key already set
	existingKey := strings.Repeat("a", 64)
	envPath := filepath.Join(workdir, ".env.dev")
	if err := os.WriteFile(envPath, []byte(compose.AuthEncryptionKeyEnv+"="+existingKey+"\n"), 0644); err != nil {
		t.Fatalf("writing .env.dev: %v", err)
	}

	err := persistGeneratedSecrets(workdir, cfg)
	if err != nil {
		t.Fatalf("persistGeneratedSecrets: %v", err)
	}

	// other generated secrets may still be written; the key must not be
	secretsPath := filepath.Join(workdir, ".env.secrets")
	data, err := os.ReadFile(secretsPath)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(string(data), "AUTH_ENCRYPTION_KEY=") {
		t.Errorf(".env.secrets must not get AUTH_ENCRYPTION_KEY when an env file holds it, got:\n%s", data)
	}
}

func TestAuthEncryptionKeyInProcessEnv(t *testing.T) {
	workdir := t.TempDir()
	cfg := &config.Config{}

	// Set key in process environment
	t.Setenv(compose.AuthEncryptionKeyEnv, strings.Repeat("b", 64))

	err := persistGeneratedSecrets(workdir, cfg)
	if err != nil {
		t.Fatalf("persistGeneratedSecrets: %v", err)
	}

	// .env.secrets should not contain AUTH_ENCRYPTION_KEY
	secretsPath := filepath.Join(workdir, ".env.secrets")
	if _, err := os.Stat(secretsPath); err == nil {
		data, _ := os.ReadFile(secretsPath)
		content := string(data)
		if strings.Contains(content, compose.AuthEncryptionKeyEnv) {
			t.Errorf(".env.secrets should not contain AUTH_ENCRYPTION_KEY when set in process env, got:\n%s", content)
		}
	} else if !os.IsNotExist(err) {
		t.Fatalf("unexpected error: %v", err)
	}
}
