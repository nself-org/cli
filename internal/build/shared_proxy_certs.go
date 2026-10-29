package build

// shared_proxy_certs.go — the certificate pairs a fronted project copies into
// the owner's served ssl dir (EPIC ADOPT D2, review F4/R2/R5).
//
// Purpose: the owner's nginx reads /etc/nginx/ssl/certificates/<dir>/ from the
// owner's own ssl/, so a fronted project's pair must live there.
// Inputs: the certificate directory names the site confs reference, this
// project's ssl/certificates/, and the owner's ssl/ tree.
// Outputs: planCert decides per directory (skip, copy, replace, keep, reuse) or
// refuses with E503; copyCert installs a pair (dir 0700, key 0600).
// Constraints: nothing is written before every directory is planned. A path
// component of the target that is a symlink is refused, except a pair P7-LIVE-15
// manages (a symlink to a sibling generation dir, target recorded in
// <ssl>/.acme/lineages.json): that pair is never copied over, listed or removed.
// A pair already in the owner's ssl with a later NotAfter is never overwritten.

import (
	"bytes"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/sharedproxy"
)

type certAction int

const (
	certSkip    certAction = iota // nothing to do (no source, or managed by ACME)
	certCopy                      // target absent: install the pair
	certReplace                   // target is this project's own: replace it
	certKeep                      // own target already newer: keep, stay listed
	certReuse                     // identical pair held by someone else: use, do not list
)

var certFiles = []string{"fullchain.pem", "privkey.pem"}

func certRefusal(format string, args ...any) error {
	return errs.New("E503", fmt.Sprintf(format, args...)).
		WithFix("Do not replace anything in the owner's ssl/ by hand; give this project another certificate directory, or fix the owner's ssl/ permissions. Nothing was written.")
}

// certNotAfter reads the NotAfter of the first certificate in a PEM file.
func certNotAfter(path string) (time.Time, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}, false
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return time.Time{}, false
	}
	c, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}, false
	}
	return c.NotAfter, true
}

// acmeManaged reports whether target is a pair P7-LIVE-15 manages: recorded in
// lineages.json and a symlink that stays inside the owner's ssl tree.
func (sp *sharedProxy) acmeManaged(dir, target string) bool {
	data, err := os.ReadFile(filepath.Join(sp.sslDir, ".acme", "lineages.json"))
	if err != nil {
		return false
	}
	var doc struct {
		Lineages []struct {
			Targets []string `json:"targets"`
		} `json:"lineages"`
	}
	if json.Unmarshal(data, &doc) != nil {
		return false
	}
	recorded := false
	for _, l := range doc.Lineages {
		for _, t := range l.Targets {
			recorded = recorded || t == "certificates/"+dir
		}
	}
	resolved, err := filepath.EvalSymlinks(target)
	root, rerr := filepath.EvalSymlinks(sp.sslDir)
	return recorded && err == nil && rerr == nil && strings.HasPrefix(resolved, root+string(filepath.Separator))
}

// planCert decides what to do with certificate directory dir. haveSource says
// whether this project has the pair locally (it may not yet, before the cert
// step of a first build).
func (sp *sharedProxy) planCert(dir string, haveSource bool) (certAction, error) {
	for _, c := range []string{sp.sslDir, filepath.Join(sp.sslDir, "certificates")} {
		if fi, err := os.Lstat(c); err == nil && (fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir()) {
			return certSkip, certRefusal("%s in the owner's ssl is a symlink or not a directory", c)
		}
	}
	target := filepath.Join(sp.sslDir, "certificates", dir)
	fi, err := os.Lstat(target)
	if err != nil {
		return certCopy, nil // absent: written once the pair exists (the cert step may not have run yet)
	}
	paths := []string{target}
	for _, f := range certFiles {
		paths = append(paths, filepath.Join(target, f))
	}
	for _, p := range paths {
		if li, err := os.Lstat(p); err == nil && li.Mode()&os.ModeSymlink != 0 {
			if sp.acmeManaged(dir, target) {
				return certSkip, nil
			}
			return certSkip, certRefusal("%s is a symlink in the owner's ssl that no ACME lineage records", p)
		}
	}
	if !fi.IsDir() {
		return certSkip, certRefusal("%s exists and is not a directory", target)
	}
	if !haveSource {
		// No local pair yet (a first build, or ssl/ was cleaned): this project's own
		// directory stays as it is and stays listed; anyone else's cannot be proven
		// identical, so it is refused before any file is written.
		if sp.ownsCert(dir) {
			return certKeep, nil
		}
		return certSkip, certRefusal("certificate directory %s already exists in the owner's ssl and is not this project's (project %q)", target, sp.project)
	}
	src := filepath.Join(sp.workdir, "ssl", "certificates", dir)
	if sp.ownsCert(dir) {
		have, ok1 := certNotAfter(filepath.Join(target, "fullchain.pem"))
		want, ok2 := certNotAfter(filepath.Join(src, "fullchain.pem"))
		if ok1 && ok2 && have.After(want) {
			return certKeep, nil
		}
		return certReplace, nil
	}
	if sameFiles(src, target) {
		return certReuse, nil
	}
	return certSkip, certRefusal("certificate directory %s already exists in the owner's ssl and is not this project's (project %q)", target, sp.project)
}

// ownsCert reports whether dir is listed in this project's previous registration.
func (sp *sharedProxy) ownsCert(dir string) bool {
	return sp.prev != nil && inSorted(sp.prev.Files, "ssl/certificates/"+dir+"/fullchain.pem")
}

// sameFiles reports whether both certificate files are byte-identical.
func sameFiles(a, b string) bool {
	for _, f := range certFiles {
		x, err1 := os.ReadFile(filepath.Join(a, f))
		y, err2 := os.ReadFile(filepath.Join(b, f))
		if err1 != nil || err2 != nil || !bytes.Equal(x, y) {
			return false
		}
	}
	return true
}

// probeWritable fails when the nearest existing directory above target cannot
// take a new entry (a permission error must surface before any write).
func probeWritable(target string) error {
	dir := target
	for {
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			break
		}
		next := filepath.Dir(dir)
		if next == dir {
			return nil
		}
		dir = next
	}
	f, err := os.CreateTemp(dir, ".nself-probe-*")
	if err != nil {
		return certRefusal("the owner's directory %s is not writable: %v", dir, err)
	}
	name := f.Name()
	_ = f.Close()
	return os.Remove(name)
}

// installCert writes the pair for dir into the owner's ssl. In write mode a
// temp directory is renamed into place so a pair never half exists (also under a
// plan-bound apply, where the pair is a runtime effect); in plan mode the files
// go through the sink.
func (st *buildState) installCert(sp *sharedProxy, dir string, replace bool) error {
	src := filepath.Join(sp.workdir, "ssl", "certificates", dir)
	if st.fx != nil && st.certPlanned(dir, sp) {
		return nil // regenerated during apply; old bytes cannot bind the plan
	}
	target := filepath.Join(sp.sslDir, "certificates", dir)
	pair := map[string][]byte{}
	for _, f := range certFiles {
		data, err := os.ReadFile(filepath.Join(src, f))
		if err != nil {
			return fmt.Errorf("reading %s for the shared proxy: %w", filepath.Join(src, f), err)
		}
		pair[f] = data
	}
	modes := map[string]os.FileMode{"fullchain.pem": 0o644, "privkey.pem": 0o600}
	marker := []byte(sp.project + "\n")
	// Cert bytes are a runtime effect: plan mode records the pair in its sink; a plan-bound
	// apply (expectSink) stages a pair the plan lists and installs any other pair directly.
	_, disk := st.sink.(*diskSink)
	held := false
	if es, ok := st.sink.(*expectSink); ok {
		_, planned := es.exp.Files[es.keyr.key(filepath.Join(target, sharedproxy.CertMarker))]
		held = !planned
	}
	if !disk && !held {
		if err := st.sink.MkdirAll(target, 0o700); err != nil {
			return err
		}
		for _, f := range certFiles {
			if err := st.sink.WriteFile(filepath.Join(target, f), pair[f], modes[f]); err != nil {
				return err
			}
		}
		return st.sink.WriteFile(filepath.Join(target, sharedproxy.CertMarker), marker, 0o600)
	}
	tmp, err := os.MkdirTemp(filepath.Join(sp.sslDir, "certificates"), "."+dir+".tmp-")
	if err != nil {
		if mkErr := os.MkdirAll(filepath.Join(sp.sslDir, "certificates"), 0o755); mkErr != nil {
			return certRefusal("cannot create %s: %v", filepath.Join(sp.sslDir, "certificates"), mkErr)
		}
		if tmp, err = os.MkdirTemp(filepath.Join(sp.sslDir, "certificates"), "."+dir+".tmp-"); err != nil {
			return err
		}
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if err := os.Chmod(tmp, 0o700); err != nil {
		return err
	}
	for _, f := range certFiles {
		if err := os.WriteFile(filepath.Join(tmp, f), pair[f], modes[f]); err != nil {
			return err
		}
		if err := os.Chmod(filepath.Join(tmp, f), modes[f]); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(tmp, sharedproxy.CertMarker), marker, 0o600); err != nil {
		return err
	}
	if !replace {
		return os.Rename(tmp, target)
	}
	old := tmp + ".old"
	if err := os.Rename(target, old); err != nil {
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Rename(old, target) // put the working pair back
		return err
	}
	_ = os.RemoveAll(old) // the new pair is live; a leftover .old dir is harmless
	return nil
}

// checkCertTargets refuses, before any write, a certificate directory of the
// owner's ssl that cannot be used (a symlink, another project's directory, no
// permission). Whether this project already holds the pair is not known yet at
// this point of a first build, so only the target side is checked.
func (st *buildState) checkCertTargets(sp *sharedProxy) error {
	if sp.role != sharedFronted {
		return nil
	}
	for _, dir := range sp.facts.certDirs {
		src := filepath.Join(sp.workdir, "ssl", "certificates", dir)
		_, e1 := os.Stat(filepath.Join(src, certFiles[0]))
		_, e2 := os.Stat(filepath.Join(src, certFiles[1]))
		action, err := sp.planCert(dir, e1 == nil && e2 == nil)
		if err != nil {
			return err
		}
		if (action == certCopy || action == certReplace) && !st.planning() {
			if err := probeWritable(filepath.Join(sp.sslDir, "certificates")); err != nil {
				return err
			}
		}
	}
	return nil
}
