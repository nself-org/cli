package deploy

// Purpose: the artifact set a v1.5 remote deploy ships (D24): the rewritten
//          manifests and merged compose files, plugin fragments, env files,
//          nginx/ (only files this build generated or the operator allowlisted;
//          conf.d only as nginx/conf.d/<name>.conf; D-0423, D-0424) and hasura/,
//          laid out in a staging dir
//          exactly as they will sit under <remote_path>.
// Inputs:  the scratch build output, the path map, the local compose and env
//          file lists (build.ReadComposeManifest / ReadComposeEnvManifest,
//          read by the caller), the resolved env snapshot.
// Outputs: ArtifactSet (relative file lists plus the remote manifests).
// Constraints: the project .env is never shipped (see ArtifactSet.KeepHostEnv).
//              Nothing under ssl/ is ever staged (the stub certs of the
//              scratch build are never shipped). Every staged file is
//              checked for a surviving local path; one survivor fails Stage.
//              Env files are staged 0600 (they carry secrets); the staging
//              dir is the only thing a sync reads.

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// maxStagedFile bounds the files read into memory for rewriting.
const maxStagedFile = 8 << 20

// ArtifactSet is the staged tree and its manifests.
type ArtifactSet struct {
	StageDir    string
	RemotePath  string
	Files       []string // relative to StageDir and to RemotePath; env files excluded
	EnvFiles    []string // relative; shipped 0600
	Compose     []string // remote absolute compose files, in -f order
	EnvManifest []string // remote absolute env files, in --env-file order
	// KeepHostEnv: the project .env is in EnvManifest but not shipped. It is the
	// host's own file (the target snapshot reads PLUGIN_DIR from it, the host's
	// nself reads it); the operator's copy would replace it with local values.
	// A sync only creates it, empty and 0600, when the host has none.
	KeepHostEnv bool
	// NotShipped are the nginx/ files of the scratch tree that were left out
	// (relative to RemotePath, sorted): not written by this build and not on the
	// operator's NSELF_DEPLOY_NGINX_INCLUDE allowlist (D-0423).
	NotShipped []string
	// Front is the fronting stack's second artifact root (D-0425); nil for a
	// project that runs its own nginx.
	Front *FrontRoot
}

// StageInput is what Stage reads.
type StageInput struct {
	Map         PathMap
	StageDir    string   // a fresh empty dir
	Compose     []string // local compose files in order
	EnvFiles    []string // local env files in order (the compose env manifest)
	EnvSnapshot string   // local resolved env snapshot, shipped as .env.<EnvName> (optional)
	EnvName     string
	// Nginx decides which scratch nginx/ files ship; nil ships every non-conf.d
	// file (and no conf.d file). See NginxPolicy.
	Nginx *NginxPolicy
	// FrontScratch is the local scratch root of the fronting stack (the parent
	// of Map.ScratchRoot) and FrontedBy its NGINX_FRONTED_BY name; both empty
	// for a project that runs its own nginx. See artifacts_front.go.
	FrontScratch, FrontedBy string
}

// Stage builds the artifact set in in.StageDir.
func Stage(in StageInput) (ArtifactSet, error) {
	set := ArtifactSet{StageDir: in.StageDir, RemotePath: in.Map.RemotePath}
	if in.FrontScratch != "" {
		in.Map.FrontRoot = in.FrontScratch
	}
	if err := checkFrontLayout(in); err != nil {
		return set, err
	}
	put := func(rel string, data []byte, mode os.FileMode) error {
		dst := filepath.Join(in.StageDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, data, mode)
	}
	rewritten := func(local string) (rel, remote string, data []byte, err error) {
		if !filepath.IsAbs(local) { // a relative source (the build's override) is relative to the scratch root, not the cwd
			local = filepath.Join(in.Map.ScratchRoot, local)
		}
		if remote, err = in.Map.Remote(local); err != nil {
			return
		}
		if data, err = readCapped(local); err != nil {
			return
		}
		return strings.TrimPrefix(remote, in.Map.RemotePath+"/"), remote, []byte(in.Map.Rewrite(string(data))), nil
	}
	for _, f := range in.Compose {
		rel, remote, data, err := rewritten(f)
		if err != nil {
			return set, err
		}
		if err := put(rel, data, 0o644); err != nil {
			return set, err
		}
		set.Compose = append(set.Compose, remote)
		set.Files = append(set.Files, rel)
	}
	for _, f := range in.EnvFiles {
		rel, remote, data, err := rewritten(f)
		if err != nil {
			return set, err
		}
		set.EnvManifest = append(set.EnvManifest, remote)
		if rel == ".env" {
			set.KeepHostEnv = true
			continue
		}
		if err := put(rel, data, 0o600); err != nil {
			return set, err
		}
		set.EnvFiles = append(set.EnvFiles, rel)
	}
	if in.EnvSnapshot != "" {
		data, err := readCapped(in.EnvSnapshot)
		if err != nil {
			return set, err
		}
		rel := ".env." + in.EnvName
		if err := put(rel, []byte(in.Map.Rewrite(string(data))), 0o600); err != nil {
			return set, err
		}
		set.EnvFiles = append(set.EnvFiles, rel)
	}
	manifests := map[string][]string{".nself/compose-files.txt": set.Compose, ".nself/compose-env-files.txt": set.EnvManifest}
	for rel, lines := range manifests {
		if len(lines) == 0 {
			continue
		}
		if err := put(rel, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			return set, err
		}
		set.Files = append(set.Files, rel)
	}
	hasura, err := copyTree(filepath.Join(in.Map.ScratchRoot, "hasura"), in.StageDir, "hasura", in.Map, false)
	if err != nil {
		return set, err
	}
	nginxFiles, notShipped, err := stageNginx(in)
	if err != nil {
		return set, err
	}
	set.Files = append(append(set.Files, hasura...), nginxFiles...)
	secrets, err := stageSecretFiles(in.Map.ScratchRoot, in.StageDir)
	if err != nil {
		return set, err
	}
	set.Files = append(set.Files, secrets...)
	set.NotShipped = notShipped
	if set.Front, err = stageFront(in); err != nil {
		return set, err
	}
	set.Files, set.EnvFiles = uniqueSorted(set.Files), uniqueSorted(set.EnvFiles)
	return set, checkNoLocalPaths(in.Map, in.StageDir, append(append([]string{}, set.Files...), set.EnvFiles...))
}

// copyTree copies srcDir (a missing dir is fine) to <stage>/<rel> and returns
// the relative file paths. Directories named conf.d* are skipped whole. rewrite
// maps local roots in the file text (nginx confs); otherwise bytes are kept.
func copyTree(srcDir, stage, rel string, m PathMap, rewrite bool) ([]string, error) {
	var out []string
	err := filepath.WalkDir(srcDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && p == srcDir {
				return filepath.SkipAll
			}
			return err
		}
		if d.IsDir() && strings.HasPrefix(d.Name(), "conf.d") {
			return filepath.SkipDir
		}
		if !d.Type().IsRegular() {
			return nil
		}
		sub, _ := filepath.Rel(srcDir, p)
		dstRel := path.Join(rel, filepath.ToSlash(sub))
		data, err := readCapped(p)
		if err != nil {
			return err
		}
		if rewrite {
			data = []byte(m.Rewrite(string(data)))
		}
		dst := filepath.Join(stage, filepath.FromSlash(dstRel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if info, ierr := d.Info(); ierr == nil {
			mode = info.Mode().Perm()
		}
		if err := os.WriteFile(dst, data, mode); err != nil {
			return err
		}
		out = append(out, dstRel)
		return nil
	})
	return out, err
}

// checkNoLocalPaths fails when any staged file still names a local root.
func checkNoLocalPaths(m PathMap, stage string, rels []string) error {
	for _, rel := range rels {
		if isSecretFile(rel) {
			continue
		} // credential bytes are never paths
		data, err := os.ReadFile(filepath.Join(stage, filepath.FromSlash(rel)))
		if err != nil {
			return err
		}
		if left := m.Leftovers(string(data)); len(left) > 0 {
			return fmt.Errorf("shipped file %s still names a local path: %s; nothing was sent", rel, strings.Join(left, ", "))
		}
	}
	return nil
}

func readCapped(p string) ([]byte, error) {
	info, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	if info.Size() > maxStagedFile {
		return nil, fmt.Errorf("%s is %d bytes, over the %d byte limit for a shipped file", p, info.Size(), maxStagedFile)
	}
	return os.ReadFile(p)
}

func uniqueSorted(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
