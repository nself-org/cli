package build

// shared_proxy_files.go — what a finished build writes for the shared proxy:
// the registration, `_owner.json`, copied certificates and the removal of
// files a project no longer serves.
//
// Purpose: EPIC ADOPT D2 ownership. Each project removes only files its own
// previous registration listed; the owner's build writes `_owner.json` and warns
// about registrations whose project directory is gone.
// Inputs: the sharedProxy plan and the files the site sink recorded.
// Outputs: <owner>/nginx/shared/<project>.json (or `_owner.json`), certificate
// pairs under <owner>/ssl/certificates/, removals.
// Constraints: every path comes from a registration and passes
// sharedproxy.ValidFile; a removal never follows a symlink; a file another
// registration lists is never removed. All writes go through st.sink.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/sharedproxy"
)

// registration builds this project's registration from the plan.
func (st *buildState) registration(sp *sharedProxy, files []string) sharedproxy.Registration {
	r := sharedproxy.New(sp.project, sp.workdir)
	if st.cfg.DockerNetwork != "" {
		r.Network = st.cfg.DockerNetwork
	}
	r.Files, r.ServerNames = files, sp.facts.names
	r.ServiceNames, r.UpstreamHosts = sp.serviceNames(), sp.facts.verbatim
	return r
}

// writeRegistration writes r as name under <owner>/nginx/shared/.
func (st *buildState) writeRegistration(sp *sharedProxy, name string, r sharedproxy.Registration) error {
	data, err := sharedproxy.Marshal(r)
	if err != nil {
		return err
	}
	if err := st.sink.MkdirAll(sp.sharedDir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", sp.sharedDir, err)
	}
	return st.sink.WriteAtomic(filepath.Join(sp.sharedDir, name), data, 0o644)
}

// finishOwner writes `_owner.json` and warns about registrations that outlived
// their project directory.
func (st *buildState) finishOwner(sp *sharedProxy) error {
	if err := st.writeRegistration(sp, sharedproxy.OwnerFile, st.registration(sp, nil)); err != nil {
		return err
	}
	for _, r := range sp.set.Projects {
		if _, err := os.Stat(r.ProjectDir); os.IsNotExist(err) {
			warnOnce(fmt.Sprintf("shared proxy: project %q is registered with %s but %s is gone; run `nself uninstall` there if it comes back, or delete: %s",
				r.Project, filepath.Join(sp.sharedDir, sharedproxy.FileName(r.Project)), r.ProjectDir, strings.Join(r.Files, " ")))
		}
	}
	return nil
}

// finishFronted installs certificates, removes stale files and writes the registration.
func (st *buildState) finishFronted(sp *sharedProxy) error {
	type step struct {
		dir        string
		action     certAction
		haveSource bool
	}
	var steps []step
	for dir := range sp.certDirs {
		_, e1 := os.Stat(filepath.Join(sp.workdir, "ssl", "certificates", dir, certFiles[0]))
		_, e2 := os.Stat(filepath.Join(sp.workdir, "ssl", "certificates", dir, certFiles[1]))
		have := e1 == nil && e2 == nil
		action, err := sp.planCert(dir, have)
		if err != nil {
			return err
		}
		if (action == certCopy || action == certReplace) && !st.planning() {
			if err := probeWritable(filepath.Join(sp.sslDir, "certificates")); err != nil {
				return err
			}
		}
		steps = append(steps, step{dir, action, have})
	}
	files := keys(sp.files)
	for _, s := range steps {
		switch s.action {
		case certCopy, certReplace:
			if !s.haveSource {
				if !st.planning() {
					warnOnce("shared proxy: " + sp.project + " has no certificate pair in ssl/certificates/" + s.dir + " to copy to the owner yet; its confs will not load until it does")
				}
				continue
			}
			if err := st.installCert(sp, s.dir, s.action == certReplace); err != nil {
				return err
			}
			fallthrough
		case certKeep:
			files = append(files, "ssl/certificates/"+s.dir+"/fullchain.pem", "ssl/certificates/"+s.dir+"/privkey.pem")
		}
	}
	reg := st.registration(sp, files)
	reg.Sort()
	if sp.prev != nil {
		st.removeListed(sp, sp.prev.Files, reg.Files)
	}
	return st.writeRegistration(sp, sharedproxy.FileName(sp.project), reg)
}

// removeListed removes each path of old that is not in keep and that
// sharedproxy.Set.Removable allows (valid, not held by another project, never
// through a symlink). Cert directories left empty go too.
func (st *buildState) removeListed(sp *sharedProxy, old, keep []string) {
	for _, rel := range old {
		if inSorted(keep, rel) || !sp.set.Removable(sp.ownerRoot, sp.project, rel) {
			continue
		}
		path := filepath.Join(sp.ownerRoot, filepath.FromSlash(rel))
		_ = st.sink.Remove(path)
		if strings.HasPrefix(rel, "ssl/certificates/") {
			_ = st.sink.Remove(filepath.Dir(path)) // fails while the dir is not empty
		}
	}
}

// dropRegistration is the path of a project with no active shared role: it
// removes the files and registration an earlier fronted build left under an
// owner (no longer fronted, or the owner dropped shared mode), and a stale
// `_owner.json` of its own.
func (st *buildState) dropRegistration(sp *sharedProxy) error {
	if sp.dropRoot != "" {
		dir := sharedproxy.DirOf(sp.dropRoot)
		if _, err := os.Stat(filepath.Join(dir, sharedproxy.FileName(sp.project))); err == nil {
			set, err := sharedproxy.Load(dir, sp.dropRoot)
			if err != nil {
				warnOnce("shared proxy: leaving " + dir + " alone, a registration there does not validate: " + err.Error())
			} else if reg := set.Find(sp.project); reg != nil && sameDir(reg.ProjectDir, sp.workdir) {
				drop := &sharedProxy{project: sp.project, ownerRoot: sp.dropRoot, set: set}
				st.removeListed(drop, reg.Files, nil)
				if err := st.sink.Remove(filepath.Join(dir, sharedproxy.FileName(sp.project))); err != nil {
					return fmt.Errorf("removing the shared proxy registration: %w", err)
				}
			}
		}
	}
	if st.cfg.Nginx.Mode == "" && st.cfg.Nginx.FrontedBy == "" {
		own := filepath.Join(sharedproxy.DirOf(sp.workdir), sharedproxy.OwnerFile)
		if _, err := os.Stat(own); err == nil {
			return st.sink.Remove(own)
		}
	}
	return nil
}
