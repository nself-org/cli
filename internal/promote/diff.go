package promote

// Purpose: the env/directory/image diffing helpers, secret redaction (config.IsSecretKey + MaskURLCredentials, D-0415), and record/backup persistence backing DryRun/Execute/Rollback in promote.go.
// Inputs: parsed .env maps and directory paths for source/target environments.
// Outputs: []DiffEntry comparisons, a saved PromotionRecord, and backup-tag discovery for rollback.
// Constraints: split out of promote.go (CLI-R12); redaction follows the shared config rule and never prints a character of a secret.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/config"
)

// loadEnvFile reads a .env file into a key-value map.
func loadEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]string), nil
		}
		return nil, err
	}

	result := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}
	return result, nil
}

// diffMaps compares two key-value maps and returns diff entries with redaction.
func diffMaps(source, target map[string]string) []DiffEntry {
	var entries []DiffEntry
	seen := make(map[string]bool)

	for k, sv := range source {
		seen[k] = true
		tv, exists := target[k]
		entry := DiffEntry{Key: k, SourceValue: redactIfSecret(k, sv)}
		if !exists {
			entry.Status = "added"
			entry.TargetValue = "(not set)"
		} else if sv == tv {
			entry.Status = "same"
			entry.TargetValue = redactIfSecret(k, tv)
		} else {
			entry.Status = "changed"
			entry.TargetValue = redactIfSecret(k, tv)
		}
		entries = append(entries, entry)
	}

	for k, tv := range target {
		if !seen[k] {
			entries = append(entries, DiffEntry{
				Key:         k,
				SourceValue: "(not set)",
				TargetValue: redactIfSecret(k, tv),
				Status:      "removed",
			})
		}
	}
	return entries
}

// redactIfSecret masks a diff value with the shared rule (D-0415): a key that
// config.IsSecretKey names prints as "****" in full, and any other value has
// its URL credentials masked by config.MaskURLCredentials. No character of a
// secret is shown; DiffEntry.Status still says whether it changed. An empty
// value stays empty.
func redactIfSecret(key, value string) string {
	if value == "" {
		return value
	}
	if config.IsSecretKey(key) {
		return "****"
	}
	return config.MaskURLCredentials(value)
}

func diffDirectories(source, target string) []DiffEntry {
	var entries []DiffEntry
	sourceFiles := listFiles(source)
	targetFiles := listFiles(target)

	seen := make(map[string]bool)
	for _, f := range sourceFiles {
		seen[f] = true
		found := false
		for _, tf := range targetFiles {
			if tf == f {
				found = true
				break
			}
		}
		if found {
			entries = append(entries, DiffEntry{Key: f, Status: "same"})
		} else {
			entries = append(entries, DiffEntry{Key: f, Status: "added"})
		}
	}
	for _, f := range targetFiles {
		if !seen[f] {
			entries = append(entries, DiffEntry{Key: f, Status: "removed"})
		}
	}
	return entries
}

func listFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, e.Name())
		}
	}
	return files
}

func diffDockerImages(_ context.Context, _ string, _, _ string) []DiffEntry {
	// Docker image diff requires reading docker-compose files for each env
	// and comparing image tags/digests. Stubbed for now with basic structure.
	return nil
}

func saveRecord(projectDir string, record *PromotionRecord) error {
	dir := filepath.Join(projectDir, ".nself", "promotions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, record.ID+".json"), data, 0o644)
}

func findLatestPromoteBackup(projectDir string) (string, error) {
	dir := filepath.Join(projectDir, ".nself", "promotions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("no promotion records found")
	}
	// Find the most recent promotion record
	var latest *PromotionRecord
	for i := len(entries) - 1; i >= 0; i-- {
		data, err := os.ReadFile(filepath.Join(dir, entries[i].Name()))
		if err != nil {
			continue
		}
		var rec PromotionRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			continue
		}
		if rec.BackupTag != "" {
			latest = &rec
			break
		}
	}
	if latest == nil {
		return "", fmt.Errorf("no promotion backup tags found")
	}
	return latest.BackupTag, nil
}
