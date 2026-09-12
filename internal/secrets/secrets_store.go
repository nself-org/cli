package secrets

// secrets_store.go — reading, writing and rotating stored secrets.
//
// Purpose: load and save the encrypted SecretStore, set/get/list individual secrets, and rotate a secret's value, used by the commands built on top of secrets.go, split out for file size.
// Inputs: an environment name and, for Set/Rotate, the secret key and value.
// Outputs: an updated, re-encrypted SecretStore on disk.
// Constraints: pure move from secrets.go (CLI-R12 Batch E); no behaviour change.

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// loadStore decrypts and reads the secret store for an environment.
// Returns an empty store if the file does not exist.
func loadStore(projectRoot, env string) (*SecretStore, error) {
	path := filepath.Join(projectRoot, secretsDir(), envFileName(env))
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return &SecretStore{
			Secrets: make(map[string]SecretEntry),
		}, nil
	}

	keyPath, err := ageKeyPath()
	if err != nil {
		return nil, err
	}

	cmd := exec.Command("age", "--decrypt", "-i", keyPath, path)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("decrypting secrets: %w", err)
	}

	var store SecretStore
	if err := json.Unmarshal(output, &store); err != nil {
		return nil, fmt.Errorf("parsing secrets: %w", err)
	}
	if store.Secrets == nil {
		store.Secrets = make(map[string]SecretEntry)
	}
	return &store, nil
}

// saveStore encrypts and writes the secret store for an environment.
func saveStore(projectRoot, env string, store *SecretStore) error {
	store.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling secrets: %w", err)
	}

	path := filepath.Join(projectRoot, secretsDir(), envFileName(env))
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating secrets directory: %w", err)
	}

	// Build age encrypt command with recipients.
	args := []string{"--encrypt"}
	if len(store.Recipients) > 0 {
		for _, r := range store.Recipients {
			args = append(args, "-r", r)
		}
	} else {
		// Use our own public key as the only recipient.
		keyPath, err := ageKeyPath()
		if err != nil {
			return err
		}
		pubKey, err := GetPublicKey(keyPath)
		if err != nil {
			return err
		}
		store.Recipients = []string{pubKey}
		// Re-marshal with recipients included.
		data, err = json.MarshalIndent(store, "", "  ")
		if err != nil {
			return fmt.Errorf("marshalling secrets: %w", err)
		}
		args = append(args, "-r", pubKey)
	}
	args = append(args, "-o", path)

	cmd := exec.Command("age", args...)
	cmd.Stdin = strings.NewReader(string(data))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("encrypting secrets: %w\n%s", err, output)
	}
	return nil
}

// Set adds or updates a secret. The value is validated before it ever
// touches the store — see ValidateSecretValue for the exact rules (empty
// values, unexpanded shell references, bracketed placeholder tokens, and
// implausibly short values are all rejected).
func Set(projectRoot, env, key, value string) error {
	if err := ValidateSecretValue(key, value); err != nil {
		slog.Error("secrets_set_rejected", "key_name", key, "env", env, "error", err)
		return err
	}
	store, err := loadStore(projectRoot, env)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	entry, exists := store.Secrets[key]
	if !exists {
		entry = SecretEntry{CreatedAt: now}
	}
	entry.Value = value
	entry.UpdatedAt = now
	store.Secrets[key] = entry
	if saveErr := saveStore(projectRoot, env, store); saveErr != nil {
		slog.Error("secrets_set_failed", "key_name", key, "env", env, "error", saveErr)
		return saveErr
	}
	slog.Info("secrets_set", "key_name", key, "env", env)
	return nil
}

// Get retrieves a secret value.
func Get(projectRoot, env, key string) (string, error) {
	store, err := loadStore(projectRoot, env)
	if err != nil {
		return "", err
	}
	entry, ok := store.Secrets[key]
	if !ok {
		return "", fmt.Errorf("secret %q not found in %s environment", key, env)
	}
	return entry.Value, nil
}

// List returns all secret keys with metadata for an environment.
func List(projectRoot, env string) ([]string, map[string]SecretEntry, error) {
	store, err := loadStore(projectRoot, env)
	if err != nil {
		return nil, nil, err
	}
	var keys []string
	for k := range store.Secrets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys, store.Secrets, nil
}

// Rotate generates a new value for a secret based on its type/name pattern.
func Rotate(projectRoot, env, key string) (string, error) {
	store, err := loadStore(projectRoot, env)
	if err != nil {
		return "", err
	}
	_, ok := store.Secrets[key]
	if !ok {
		return "", fmt.Errorf("secret %q not found in %s environment", key, env)
	}

	newValue, hint := generateRotationValue(key)
	if newValue == "" {
		// generateRotationValue declines to auto-generate for API keys/
		// tokens that must be rotated through a provider dashboard. Do NOT
		// persist that empty value over the existing entry — that would
		// silently blank a working credential, exactly the failure mode
		// this whole validation layer exists to prevent. Leave the stored
		// value untouched and tell the caller what to do instead.
		return "", fmt.Errorf("secret %q requires manual rotation: %s (existing value left unchanged; use 'nself secrets set %s <new-value>')", key, hint, key)
	}
	if hint != "" {
		slog.Info("rotation hint", "note", hint)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	store.Secrets[key] = SecretEntry{
		Value:     newValue,
		CreatedAt: store.Secrets[key].CreatedAt,
		UpdatedAt: now,
		RotatedAt: now,
	}
	if err := saveStore(projectRoot, env, store); err != nil {
		slog.Error("secrets_rotate_failed", "key_name", key, "env", env, "error", err)
		return "", err
	}
	slog.Info("secrets_rotated", "key_name", key, "env", env)
	return newValue, nil
}

// generateRotationValue produces a new secret value based on the key name.
func generateRotationValue(key string) (value string, hint string) {
	keyUpper := strings.ToUpper(key)
	switch {
	case strings.HasSuffix(keyUpper, "_PASSWORD") || strings.HasSuffix(keyUpper, "_PASS"):
		return generateRandomString(32), "Remember to update the corresponding database/service password."
	case strings.Contains(keyUpper, "JWT") || strings.Contains(keyUpper, "SECRET"):
		return generateRandomString(64), "Services using this secret will need to be restarted."
	case strings.Contains(keyUpper, "API_KEY") || strings.Contains(keyUpper, "TOKEN"):
		return "", "API keys and tokens must be rotated through the provider's dashboard. Update the value with 'nself secrets set'."
	default:
		return generateRandomString(32), ""
	}
}

// generateRandomString generates a cryptographically random alphanumeric string.
func generateRandomString(length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := range result {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		result[i] = chars[n.Int64()]
	}
	return string(result)
}
