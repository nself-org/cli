package doctor

// backup_hint.go: advisory backup hints for `nself doctor` (P7-PROD-08).
//
// Purpose: point projects that hand-roll backups (pg_dump piped to aws s3 cp or
// rclone copy) or migrations (psql -f migrations/...) at `nself backup stream`
// and `nself db migrate`, and remind the owner to keep the auto-created backup
// identity off this host.
// Inputs: projectDir (the nSelf working directory) and the owner's home.
// Outputs: CheckResult values whose Status is always "pass": doctor's exit code
// counts warnings (v1.4 exit 2, v1.5 exit 12), and an advisory hint must leave
// it unchanged. A hint that fires carries the "advisory:" prefix instead.
// Constraints: advisory only; never reads key contents; scans script-like files
// to a bounded depth and size. Not yet registered with the doctor runner
// (cmd/commands/doctor.go is out of scope; P7-SURF-11 wires BackupHintChecks).
// SPORT: cli/internal/doctor.

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	backupHintSection  = "backup"
	backupScriptID     = "BACKUP-HINT-01"
	backupKeyID        = "BACKUP-HINT-02"
	hintMaxDepth       = 5
	hintMaxFileBytes   = 256 << 10
	hintMaxFiles       = 4000
	hintMaxNamedInHint = 3
	// hintStatus is "pass" on purpose: see the Outputs note above.
	hintStatus = "pass"
	// HintPrefix marks a hint that fired.
	HintPrefix = "advisory: "
)

var (
	hintSkipDirs = map[string]bool{".git": true, "node_modules": true, "vendor": true, ".claude": true,
		"backups": true, ".nself": true, "dist": true, "build": true, ".next": true, "target": true}
	hintScriptExt = map[string]bool{".sh": true, ".bash": true, ".zsh": true, ".cron": true,
		".service": true, ".timer": true, ".mk": true, ".yml": true, ".yaml": true}
	pgDumpRe   = regexp.MustCompile(`\bpg_dump(all)?\b`)
	uploadRe   = regexp.MustCompile(`\baws\s+s3\s+(cp|mv|sync)\b|\brclone\s+(copy|copyto|move|moveto|sync|rcat)\b|\bs3cmd\s+put\b|\bmc\s+cp\b`)
	psqlFileRe = regexp.MustCompile(`\bpsql\b[^\n]*(\s-f\s*|\s--file[= ]|\s<\s)|\|\s*psql\b`)
	migDirRe   = regexp.MustCompile(`\bmigrations?/`)
)

// BackupHintChecks returns the advisory backup hints for projectDir.
func BackupHintChecks(projectDir string) []CheckResult {
	return []CheckResult{checkHandRolledBackup(projectDir), checkBackupIdentityBackedUp(projectDir)}
}

// checkHandRolledBackup warns when a project script uploads pg_dump output by
// hand or applies migrations with raw psql.
func checkHandRolledBackup(projectDir string) CheckResult {
	var backups, migs []string
	files := 0
	root := filepath.Clean(projectDir)
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			if path != root && (hintSkipDirs[d.Name()] || strings.Count(rel, string(filepath.Separator)) >= hintMaxDepth) {
				return filepath.SkipDir
			}
			return nil
		}
		if files++; files > hintMaxFiles {
			return filepath.SkipAll
		}
		if !d.Type().IsRegular() || !looksLikeScript(path) {
			return nil
		}
		rel = filepath.ToSlash(rel)
		dump, upload, mig := scanScript(path)
		if dump && upload {
			backups = append(backups, rel)
		}
		if mig {
			migs = append(migs, rel)
		}
		return nil
	})
	if len(backups) == 0 && len(migs) == 0 {
		return CheckResult{Section: backupHintSection, Name: backupScriptID, Status: hintStatus,
			Message: backupScriptID + ": no hand-rolled backup or migration scripts found"}
	}
	var parts, fixes []string
	if len(backups) > 0 {
		parts = append(parts, "hand-rolled backup upload in "+nameList(backups)+" (nself backup stream encrypts and uploads it)")
		fixes = append(fixes, "nself backup stream --to <destination>")
	}
	if len(migs) > 0 {
		parts = append(parts, "raw psql migrations in "+nameList(migs)+" (nself db migrate up tracks and checksums them)")
		fixes = append(fixes, "nself db migrate up")
	}
	return CheckResult{Section: backupHintSection, Name: backupScriptID, Status: hintStatus,
		Message: backupScriptID + ": " + HintPrefix + strings.Join(parts, "; "), FixCmd: strings.Join(fixes, " && ")}
}

func nameList(names []string) string {
	sort.Strings(names)
	if len(names) > hintMaxNamedInHint {
		return strings.Join(names[:hintMaxNamedInHint], ", ") + fmt.Sprintf(" and %d more", len(names)-hintMaxNamedInHint)
	}
	return strings.Join(names, ", ")
}

// looksLikeScript is true for shell and unit-style files, Makefiles and
// extensionless files that start with a shebang.
func looksLikeScript(path string) bool {
	base := filepath.Base(path)
	if hintScriptExt[strings.ToLower(filepath.Ext(base))] || base == "Makefile" || base == "makefile" || base == "crontab" {
		return true
	}
	if filepath.Ext(base) != "" {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	var b [2]byte
	n, _ := f.Read(b[:])
	return n == 2 && b[0] == '#' && b[1] == '!'
}

// scanScript reports, over non-comment lines, whether the file runs pg_dump,
// uploads with a hand-rolled tool, or feeds migrations to psql.
func scanScript(path string) (dump, upload, mig bool) {
	fi, err := os.Stat(path)
	if err != nil || fi.Size() > hintMaxFileBytes {
		return
	}
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), hintMaxFileBytes)
	var psqlFile, migDir bool
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		dump = dump || pgDumpRe.MatchString(line)
		upload = upload || uploadRe.MatchString(line)
		psqlFile = psqlFile || psqlFileRe.MatchString(line)
		migDir = migDir || migDirRe.MatchString(line)
	}
	return dump, upload, psqlFile && migDir
}

// checkBackupIdentityBackedUp warns while an auto-created identity exists
// without the owner's backed-up marker. It only stats files; the identity is
// never opened.
func checkBackupIdentityBackedUp(projectDir string) CheckResult {
	project := envKeyValue(projectDir, "PROJECT_NAME")
	home, herr := os.UserHomeDir()
	ok := CheckResult{Section: backupHintSection, Name: backupKeyID, Status: hintStatus,
		Message: backupKeyID + ": no auto-created backup identity to back up"}
	if project == "" || herr != nil || project != filepath.Base(project) {
		return ok
	}
	key := filepath.Join(home, ".config", "nself", project+"-age.key")
	if fi, err := os.Lstat(key); err != nil || !fi.Mode().IsRegular() {
		return ok
	}
	if _, err := os.Lstat(key + ".backed-up"); err == nil {
		ok.Message = backupKeyID + ": backup identity is marked as backed up off this host"
		return ok
	}
	return CheckResult{Section: backupHintSection, Name: backupKeyID, Status: hintStatus,
		Message: fmt.Sprintf("%s: "+HintPrefix+"the backup identity %s is the only way to decrypt your backups; if it is lost they are unrecoverable. Copy it off this machine, then create %s.backed-up to silence this", backupKeyID, key, key),
		FixCmd:  "touch " + key + ".backed-up"}
}
