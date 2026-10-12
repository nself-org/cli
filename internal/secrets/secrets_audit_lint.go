package secrets

// secrets_audit_lint.go — secret auditing, deploy decryption, rekeying and linting.
//
// Purpose: audit stored secrets for staleness/weak values, decrypt them for a deploy, rekey a store to new recipients, and lint an env file for secrets that should be managed instead, split out of secrets.go for file size.
// Inputs: an environment name and, for LintSecrets, a plain .env file.
// Outputs: AuditFinding/LintFinding lists, or a decrypted/rekeyed store.
// Constraints: pure move from secrets.go (CLI-R12 Batch E); no behaviour change.

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Audit checks for secrets that haven't been rotated in over 90 days.
func Audit(projectRoot, env string) ([]AuditFinding, error) {
	store, err := loadStore(projectRoot, env)
	if err != nil {
		return nil, err
	}

	var findings []AuditFinding
	for key, entry := range store.Secrets {
		lastRotated := entry.RotatedAt
		if lastRotated == "" {
			lastRotated = entry.CreatedAt
		}
		if lastRotated == "" {
			findings = append(findings, AuditFinding{
				Key:      key,
				Issue:    "no creation or rotation timestamp",
				Severity: "high",
			})
			continue
		}
		t, err := time.Parse(time.RFC3339, lastRotated)
		if err != nil {
			continue
		}
		age := time.Since(t)
		if age > 90*24*time.Hour {
			findings = append(findings, AuditFinding{
				Key:      key,
				Issue:    fmt.Sprintf("last rotated %d days ago (threshold: 90 days)", int(age.Hours()/24)),
				Severity: "warning",
			})
		}
	}
	sort.Slice(findings, func(i, j int) bool {
		return findings[i].Key < findings[j].Key
	})
	return findings, nil
}

// AuditFinding represents a single audit finding.
type AuditFinding struct {
	Key      string
	Issue    string
	Severity string
}

// DecryptForDeploy decrypts secrets and outputs them as KEY=VALUE lines
// suitable for .env.computed or CI/CD injection.
func DecryptForDeploy(projectRoot, env string) (string, error) {
	store, err := deployStore(projectRoot, env)
	if err != nil {
		return "", err
	}
	keys := sortedStoreKeys(store)
	lines := make([]string, 0, len(keys))
	for _, k := range keys {
		lines = append(lines, fmt.Sprintf("%s=%s", k, store.Secrets[k].Value))
	}
	return strings.Join(lines, "\n"), nil
}

// DeployKeys returns the sorted key names of the store DecryptForDeploy reads,
// on the same two paths (DEPLOY_AGE_KEY, else the local store). Callers that
// must name the keys without handling the values (the --json envelope) use
// this instead of parsing DecryptForDeploy's text, where a multi-line value can
// look like a KEY= line.
func DeployKeys(projectRoot, env string) ([]string, error) {
	store, err := deployStore(projectRoot, env)
	if err != nil {
		return nil, err
	}
	return sortedStoreKeys(store), nil
}

// deployStore loads the store for a deploy: with the DEPLOY_AGE_KEY env var
// (used in CI) when set, else with the local key.
func deployStore(projectRoot, env string) (*SecretStore, error) {
	if deployKey := os.Getenv("DEPLOY_AGE_KEY"); deployKey != "" {
		return decryptWithKey(projectRoot, env, deployKey)
	}
	return loadStore(projectRoot, env)
}

// sortedStoreKeys returns the store's secret names in sorted order.
func sortedStoreKeys(store *SecretStore) []string {
	keys := make([]string, 0, len(store.Secrets))
	for k := range store.Secrets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// decryptWithKey decrypts the store using a raw age private key string (for CI/CD).
func decryptWithKey(projectRoot, env, key string) (*SecretStore, error) {
	path := filepath.Join(projectRoot, secretsDir(), envFileName(env))

	// Write key to a temp file.
	tmpKey, err := os.CreateTemp("", "nself-deploy-key-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(tmpKey.Name()) }()
	if _, err := tmpKey.WriteString(key); err != nil {
		return nil, err
	}
	_ = tmpKey.Close()

	cmd := exec.Command("age", "--decrypt", "-i", tmpKey.Name(), path)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("decrypting with deploy key: %w", err)
	}

	var store SecretStore
	if err := json.Unmarshal(output, &store); err != nil {
		return nil, fmt.Errorf("parsing secrets: %w", err)
	}
	return &store, nil
}

// Rekey re-encrypts all secret files, removing the specified public key
// from the recipients list. Used when a team member leaves.
func Rekey(projectRoot, removePubKey string) error {
	sDir := filepath.Join(projectRoot, secretsDir())
	entries, err := os.ReadDir(sDir)
	if err != nil {
		return fmt.Errorf("reading secrets directory: %w", err)
	}

	rekeyCount := 0
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".age") {
			continue
		}
		env := strings.TrimSuffix(entry.Name(), ".age")
		store, err := loadStore(projectRoot, env)
		if err != nil {
			return fmt.Errorf("loading %s: %w", env, err)
		}

		// Remove the specified recipient.
		var newRecipients []string
		removed := false
		for _, r := range store.Recipients {
			if r == removePubKey {
				removed = true
				continue
			}
			newRecipients = append(newRecipients, r)
		}
		if !removed {
			continue
		}
		if len(newRecipients) == 0 {
			return fmt.Errorf("cannot remove last recipient from %s — add a new recipient first", env)
		}
		store.Recipients = newRecipients
		if err := saveStore(projectRoot, env, store); err != nil {
			return fmt.Errorf("re-encrypting %s: %w", env, err)
		}
		rekeyCount++
		slog.Info("rekeyed environment", "env", env+".age", "action", "removed recipient")
	}

	if rekeyCount == 0 {
		slog.Info("rekey: no files contained the specified recipient")
	} else {
		slog.Info("rekey complete", "environments", rekeyCount)
	}
	return nil
}

// LintSecrets checks for plaintext secrets in git-tracked files.
func LintSecrets(projectRoot string) ([]LintFinding, error) {
	// Check if gitleaks is installed.
	gitleaksPath, err := exec.LookPath("gitleaks")
	if err != nil {
		return nil, fmt.Errorf("gitleaks not found in PATH — install with: brew install gitleaks")
	}

	// Write the report to a real temp file rather than the literal path
	// "/dev/stdout". gitleaks >=8.x opens --report-path with O_CREATE, which
	// fails against the stdout character device on several platforms/
	// sandboxes ("open /dev/stdout: permission denied") — when that happens
	// gitleaks still exits 1 (findings-may-be-present) but writes no JSON at
	// all, so every call fell through to the "parse-error" placeholder finding
	// below regardless of whether the source actually contained a secret.
	// That made `nself secrets lint` unconditionally report exactly one bogus
	// finding — a false positive on clean trees and a masked true negative on
	// leaky ones — which defeats the command's purpose entirely.
	reportFile, err := os.CreateTemp("", "nself-gitleaks-report-*.json")
	if err != nil {
		return nil, fmt.Errorf("creating gitleaks report file: %w", err)
	}
	reportPath := reportFile.Name()
	_ = reportFile.Close()
	defer func() { _ = os.Remove(reportPath) }()

	cmd := exec.Command(gitleaksPath, "detect", "--source", projectRoot, "--no-git", "--report-format", "json", "--report-path", reportPath)
	runErr := cmd.Run()

	// gitleaks exits 1 when findings are present.
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			output, readErr := os.ReadFile(reportPath)
			if readErr != nil {
				return nil, fmt.Errorf("reading gitleaks report: %w", readErr)
			}
			var findings []LintFinding
			if jsonErr := json.Unmarshal(output, &findings); jsonErr != nil {
				// If JSON parse fails, just report raw output.
				return []LintFinding{{
					File:    "unknown",
					Rule:    "parse-error",
					Message: string(output),
				}}, nil
			}
			return findings, nil
		}
		return nil, fmt.Errorf("running gitleaks: %w", runErr)
	}

	return nil, nil
}

// LintFinding represents a detected secret in source code.
type LintFinding struct {
	File    string `json:"File"`
	Rule    string `json:"RuleID"`
	Message string `json:"Description"`
	Line    int    `json:"StartLine"`
}
