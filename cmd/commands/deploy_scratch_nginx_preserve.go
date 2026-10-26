package commands

import (
	"os"
	"path/filepath"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/nginxtopo"
)

type preservedNginxSite struct {
	path string
	data []byte
	mode os.FileMode
}

// Purpose: preserve allowlisted marked sites across the build's stale-site prune.
// Inputs: scratch project root and resolved nginx allowlist policy.
// Outputs: a restore function holding snapshots, or a filesystem error.
// Constraints: snapshot only regular scratch files; freshly generated sites win.
// SPORT: D-1147, NSELF_DEPLOY_NGINX_INCLUDE preservation during remote deploy.
func preserveScratchNginxSites(scratch string, policy *deploy.NginxPolicy) (func() error, error) {
	var saved []preservedNginxSite
	nginxDir, err := nginxtopo.ServedNginxDir(scratch, "") // the scratch project's own tree, as scratchCopyProject lays it
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(nginxDir, "sites")
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !policy.Allows("sites/"+entry.Name()) {
			continue
		}
		p := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		if !build.IsGeneratedNginxConf(data) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		saved = append(saved, preservedNginxSite{p, data, info.Mode().Perm()})
	}
	return func() error { return restoreScratchNginxSites(saved) }, nil
}

// Purpose: restore allowlisted sites removed by the scratch build.
// Inputs: pre-build site snapshots, including contents and permissions.
// Outputs: restored missing files, or an error that stops staging.
// Constraints: exclusive creation preserves existing build outputs and symlinks.
// SPORT: D-1147, NSELF_DEPLOY_NGINX_INCLUDE preservation during remote deploy.
func restoreScratchNginxSites(saved []preservedNginxSite) error {
	for _, s := range saved {
		f, err := os.OpenFile(s.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, s.mode)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		_, err = f.Write(s.data)
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}
