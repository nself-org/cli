package compose

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
)

// Purpose: render the CS_N v2 keys (P7-ADOPT-01) into a custom service:
// depends_on, extra project networks, Dockerfile path, build target, command,
// the ancestor build-context bound (E528) and the host-bind rule (E502).
// Inputs: the ServiceConfig buildCustomService already filled, the parsed
// config.CustomService, and the Generator's config and workDir.
// Outputs: svc updated in place, or an E500/E501/E502/E528 error.
// Constraints: additive. A service that sets no v2 key is not touched, so
// existing projects render byte-identically (basis.golden.yml). Dependency
// names are only syntax-checked in config; existence is checked in build
// post-validation, after the plugin step wrote .nself/compose-files.txt.

// csNetworkRestRe is what may follow the <PROJECT_NAME>_ prefix.
var csNetworkRestRe = regexp.MustCompile(`^[a-z0-9_-]+$`)

// dockerSocketWarned dedupes the v1.4 docker.sock warning per entry.
var dockerSocketWarned sync.Map

// applyCustomServiceV2 applies the v2 keys to svc. See the file comment.
func (g *Generator) applyCustomServiceV2(svc *ServiceConfig, cs config.CustomService) error {
	if cs.BuildPath != "" && cs.Image == "" {
		key := fmt.Sprintf("CS_%d_PATH", cs.Index)
		if err := config.ValidateBuildContext(key, g.workDir, cs.BuildPath); err != nil {
			return err
		}
	}
	if svc.Build != nil {
		if cs.Dockerfile != "" {
			svc.Build.Dockerfile = cs.Dockerfile
			if err := g.checkDockerfileInContext(cs, svc.Build); err != nil {
				return err
			}
		}
		svc.Build.Target = cs.BuildTarget
	}
	if len(cs.Command) > 0 {
		svc.Command = cs.Command
	}
	for _, dep := range cs.DependsOn {
		if dep.Name == cs.Name {
			return errs.Newf("E500", "CS_%d_DEPENDS_ON names the service itself (%q)", cs.Index, dep.Name)
		}
		svc.DependsOn[dep.Name] = DepOn{Condition: dep.Condition}
	}
	nets, err := customServiceNetworks(g.cfg, cs)
	if err != nil {
		return err
	}
	svc.Networks = append(svc.Networks, nets...)
	return g.checkCustomServiceVolumes(cs)
}

// customServiceNetworks returns the extra networks of cs, each
// <PROJECT_NAME>_[a-z0-9_-]+ (E501 otherwise), without the project network
// and without duplicates. A custom service never joins another project's
// network.
func customServiceNetworks(cfg *config.Config, cs config.CustomService) ([]string, error) {
	prefix := cfg.ProjectName + "_"
	seen := map[string]bool{cfg.DockerNetwork: true}
	var out []string
	for _, n := range cs.Networks {
		if !strings.HasPrefix(n, prefix) || !csNetworkRestRe.MatchString(strings.TrimPrefix(n, prefix)) {
			return nil, errs.Newf("E501", "CS_%d_NETWORKS names %q: a custom service may only join networks named %s*", cs.Index, n, prefix)
		}
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out, nil
}

// checkDockerfileInContext refuses a CS_N_DOCKERFILE that is a symlink leading
// outside the build context. The path text was already checked for "..".
// A Dockerfile that does not exist yet is left to the builder to report.
func (g *Generator) checkDockerfileInContext(cs config.CustomService, b *BuildConfig) error {
	ctx := filepath.Join(g.workDir, b.Context)
	file := filepath.Join(ctx, b.Dockerfile)
	realFile, err := filepath.EvalSymlinks(file)
	if err != nil {
		return nil
	}
	realCtx, err := filepath.EvalSymlinks(ctx)
	if err != nil {
		return nil
	}
	if !pathWithin(realFile, realCtx) {
		return fmt.Errorf("CS_%d_DOCKERFILE %q resolves outside the build context", cs.Index, cs.Dockerfile)
	}
	return nil
}

// pathWithin reports whether p is dir or lies under it (both cleaned).
func pathWithin(p, dir string) bool {
	p, dir = filepath.Clean(p), filepath.Clean(dir)
	return p == dir || strings.HasPrefix(p, dir+string(filepath.Separator))
}

// checkCustomServiceVolumes applies the host-bind rule to CS_N_VOLUMES.
// v1.5 mode refuses (E502): the Docker socket and the host root in any mode
// suffix, and a writable absolute host path outside the project. v1.4 mode
// keeps today's behaviour and warns once about the socket and the host root.
// Named volumes, project-relative binds and read-only outside binds pass.
func (g *Generator) checkCustomServiceVolumes(cs config.CustomService) error {
	// compat.V15(P7-ADOPT-01): writable absolute bind allowed -> E502
	strict := compat.V15()
	proj, err := filepath.Abs(g.workDir)
	if err != nil {
		proj = g.workDir
	}
	if real, err := filepath.EvalSymlinks(proj); err == nil {
		proj = real
	}
	for _, entry := range parseCustomServiceVolumes(cs.Volumes) {
		parts := strings.Split(entry, ":")
		host := parts[0]
		if !strings.HasPrefix(host, "/") {
			continue
		}
		clean := filepath.Clean(host)
		switch {
		case clean == "/" || clean == "/var/run/docker.sock" || clean == "/run/docker.sock":
			if strict {
				return errs.Newf("E502", "CS_%d_VOLUMES binds %q: the Docker socket and the host root are never allowed", cs.Index, entry)
			}
			if _, dup := dockerSocketWarned.LoadOrStore(entry, true); !dup {
				slog.Warn("CS_N_VOLUMES binds the Docker socket or host root; this is refused in v1.5", "service", cs.Name, "volume", entry)
			}
		case strict && !(len(parts) >= 3 && parts[2] == "ro") && !pathWithin(clean, proj):
			return errs.Newf("E502", "CS_%d_VOLUMES binds %q: a writable absolute host path outside the project is not allowed (add :ro or use a named volume)", cs.Index, entry)
		}
	}
	return nil
}
