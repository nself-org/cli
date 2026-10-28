package build

// Purpose: hold snapshots and retention pruning in the artifact transaction.
// Inputs: the existing nginx sites directory. Outputs: staged backup files.
// Constraints: backup roots must remain real project directories; publication
// uses os.Root so a symlink swap cannot redirect a backup outside the project.

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (e *expectSink) backupSites(sitesDir string) error {
	entries, err := e.ReadDir(sitesDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || len(entries) == 0 {
		return err
	}
	if e.allowBackup() != nil {
		return nil // the confirmed render carries no snapshot: the plan leaves nginx/sites alone (D-0757)
	}
	if err := checkBackupRoot(e.root); err != nil {
		return err
	}
	if backupBeforeWrite != nil {
		backupBeforeWrite()
	}
	if err := checkBackupRoot(e.root); err != nil {
		return err
	}
	backups := filepath.Join(e.root, ".nself", "backups")
	dest := filepath.Join(backups, "nginx-sites-"+time.Now().UTC().Format("20060102-150405"))
	if err := e.MkdirAll(dest, 0755); err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := e.ReadFile(filepath.Join(sitesDir, entry.Name()))
		if err != nil {
			return err
		}
		if err := e.WriteFile(filepath.Join(dest, entry.Name()), data, 0644); err != nil {
			return err
		}
	}
	entries, err = e.ReadDir(backups)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var snapshots []string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "nginx-sites-") {
			snapshots = append(snapshots, entry.Name())
		}
	}
	// memSink directory listings omit staged child directories.
	name := filepath.Base(dest)
	sort.Strings(snapshots)
	if !inSorted(snapshots, name) {
		snapshots = append(snapshots, name)
	}
	sort.Strings(snapshots)
	if len(snapshots) <= maxNginxSitesBackups {
		return nil
	}
	for _, name := range snapshots[:len(snapshots)-maxNginxSitesBackups] {
		if err := e.removeBackupTree(filepath.Join(backups, name)); err != nil {
			return err
		}
	}
	return nil
}

func (e *expectSink) removeBackupTree(path string) error {
	entries, err := e.ReadDir(path)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		child := filepath.Join(path, entry.Name())
		if entry.IsDir() {
			err = e.removeBackupTree(child)
		} else {
			err = e.Remove(child)
		}
		if err != nil {
			return err
		}
	}
	return e.Remove(path)
}
