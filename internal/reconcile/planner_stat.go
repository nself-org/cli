package reconcile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Purpose: the stat step of the plan's "before" read (artifactSets), with the
// D-1201 legacy-file rule.
// Inputs: the project directory, the on-disk path of one artifact, its stat error.
// Outputs: statFile (a test seam over os.Stat) and legacyPluginsFile.
// Constraints: only ENOTDIR on a path under <project>/.nself/plugins, while that
// path is itself a regular file (the 22-byte legacy plugin list some old
// projects still carry), counts as absent. Every other stat error stays fatal.

// statFile is os.Stat; tests replace it to inject errors.
var statFile = os.Stat

// legacyPluginsFile reports whether err is the ENOTDIR a path under
// <projectDir>/.nself/plugins gets because .nself/plugins is a regular file.
func legacyPluginsFile(projectDir, disk string, err error) bool {
	root := filepath.Join(projectDir, ".nself", "plugins")
	if !errors.Is(err, syscall.ENOTDIR) || !strings.HasPrefix(disk, root+string(filepath.Separator)) {
		return false
	}
	fi, lerr := os.Lstat(root)
	return lerr == nil && fi.Mode().IsRegular()
}
