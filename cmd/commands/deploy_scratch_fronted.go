package commands

// Purpose: the scratch build of a project fronted by another stack's nginx
//          (NGINX_FRONTED_BY, D-0425 scratch side). resolveNginxSitesDir only
//          lets such a project build when basename(parent(workdir)) equals the
//          fronting stack's name, which the flat scratch layout
//          <workdir>/.nself/deploy-build/<env> never satisfied. For a fronted
//          project the scratch becomes
//            <workdir>/.nself/deploy-build/<env>/<FrontedBy>/backend
//          so the build finds its fronting root at .../<env>/<FrontedBy> and
//          writes nginx/sites/*.conf there; Stage ships those as a second root.
// Inputs:  the operator project, the deploy target, the target snapshot.
// Outputs: the scratch project dir, ssl stubs where the build and the served
//          stack read them, and the fail-closed staging result.
// Constraints: the leaf is a fixed name, not the host's basename: the path map
//              rewrites <front>/<leaf> to remote_path and <front> to its parent
//              whatever the leaf is, and runScratchBuild has no remote path
//              (its call site is not changed). The host layout is checked in
//              deploy.Stage (E485 unless basename(dir(remote_path)) equals
//              FrontedBy). The v1.5 apply cannot ship the fronting stack's
//              files yet, so a fronted project that stages any is refused
//              (fail closed) after the report; the apply side is a follow-up.

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/nginxtopo"
)

// frontedLeaf is the project dir name inside the fronting scratch root.
const frontedLeaf = "backend"

// scratchFrontedBy returns the target's NGINX_FRONTED_BY, validated as one safe
// path element ("" when the project runs its own nginx).
func scratchFrontedBy(workdir, target string) (string, error) {
	v, err := deployCascadeValue(workdir, target, "NGINX_FRONTED_BY")
	if err != nil || v == "" {
		return "", err
	}
	if v == "." || v == ".." || strings.HasPrefix(v, ".") || strings.ContainsAny(v, `/\`) || strings.ContainsFunc(v, unicode.IsControl) {
		return "", fmt.Errorf("deploy: NGINX_FRONTED_BY=%q is not one plain directory name; nothing was sent", v)
	}
	return v, nil
}

// scratchProjectDir is the scratch project dir of env: the flat scratch dir, or
// for a fronted project <scratch>/<frontedBy>/backend.
func scratchProjectDir(workdir, env, frontedBy string) string {
	if frontedBy == "" {
		return scratchDirFor(workdir, env)
	}
	return filepath.Join(scratchDirFor(workdir, env), frontedBy, frontedLeaf)
}

// writeFrontStubSSL lays the same ssl stubs where the fronting stack reads them
// (nginxtopo.ServedSSLDir) for a fronted project. The build's own ssl step reads
// <scratch>/ssl, which runScratchBuild fills for every project.
func writeFrontStubSSL(scratch, frontedBy string, certs []deploy.CertDir) error {
	if frontedBy == "" {
		return nil
	}
	dir, err := nginxtopo.ServedSSLDir(scratch, frontedBy)
	if err != nil {
		return err
	}
	return writeStubSSLDir(dir, certs)
}

// finishScratchStage reports what the stage left out and fails closed on a
// fronting stack's files the apply cannot ship yet.
func finishScratchStage(w io.Writer, set deploy.ArtifactSet, frontedBy string, stale []string) (deploy.ArtifactSet, error) {
	reportNotShipped(w, set.NotShipped)
	reportStale(w, stale)
	if set.Front == nil || len(set.Front.Files) == 0 {
		return set, nil
	}
	for _, rel := range set.Front.Files {
		_, _ = fmt.Fprintf(w, "  [staged, not shipped] %s\n", path.Join(set.Front.RemotePath, rel))
	}
	return deploy.ArtifactSet{}, errs.New("E485", fmt.Sprintf("NGINX_FRONTED_BY=%s: the v1.5 apply cannot ship the fronting stack's nginx/sites yet (follow-up of D-0425); nothing was sent", frontedBy)).
		WithFix("deploy this project with compat v1.4 (unset NSELF_V15) until the v1.5 apply can ship the fronting stack's nginx/sites")
}

// clearScratch removes the scratch root of env (both layouts).
func clearScratch(workdir, env string) error {
	root := scratchDirFor(workdir, env)
	if err := os.RemoveAll(root); err != nil {
		return fmt.Errorf("deploy: clearing %s: %w", root, err)
	}
	return nil
}
