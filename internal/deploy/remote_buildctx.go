package deploy

// Purpose: refuse a deploy whose compose has a service with a `build:` context
//          the deploy cannot provide (F8, review M4). The artifact set ships
//          manifests, fragments, env files, nginx/ and hasura/; it does not ship
//          build contexts, so `up` would fail on the host after files were synced.
// Inputs:  the staged ArtifactSet and a Remote for read-only existence checks.
// Outputs: nil, or E485 (detail `build context not shippable`) naming the services.
// Constraints: runs before the first remote write. A context passes when it
//              resolves (relative to its compose file, with ${VAR} from the staged
//              env files) to a directory that already exists on the host. Anything
//              else, including an unresolved variable, is refused.

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/errs"
	"gopkg.in/yaml.v3"
)

// CheckBuildContexts checks every service with a build context in the set.
func CheckBuildContexts(ctx context.Context, r Remote, set ArtifactSet) error {
	env := stagedEnv(set)
	type need struct{ svc, dir string }
	var needs []need
	for _, remote := range set.Compose {
		rel := strings.TrimPrefix(remote, set.RemotePath+"/")
		data, err := os.ReadFile(filepath.Join(set.StageDir, filepath.FromSlash(rel)))
		if err != nil {
			return err
		}
		var doc struct {
			Services map[string]struct {
				Build yaml.Node `yaml:"build"`
			} `yaml:"services"`
		}
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("parsing %s: %w", rel, err)
		}
		for name, s := range doc.Services {
			dir, has := buildContext(s.Build)
			if !has {
				continue
			}
			dir = expandVars(dir, env)
			if !path.IsAbs(dir) {
				dir = path.Join(path.Dir(remote), dir)
			}
			needs = append(needs, need{name, path.Clean(dir)})
		}
	}
	var bad []string
	for _, n := range needs {
		if strings.ContainsAny(n.dir, "$") || strings.ContainsAny(n.dir, "\n'") {
			bad = append(bad, n.svc)
			continue
		}
		out, err := r.Run(ctx, shScript(fmt.Sprintf(`if [ -d %s ]; then echo yes; else echo no; fi`, q(n.dir))))
		if err != nil {
			return fmt.Errorf("checking build context %s on %s: %w", n.dir, r.Label(), err)
		}
		if strings.TrimSpace(string(out)) != "yes" {
			bad = append(bad, n.svc)
		}
	}
	if len(bad) == 0 {
		return nil
	}
	sort.Strings(bad)
	return errs.New("E485", "build context not shippable: service(s) "+strings.Join(uniqueSorted(bad), ", ")+
		" build from a directory that is not on the host and the deploy does not ship build contexts; nothing was sent").
		WithFix("publish an image for the service, or place its build context on the host")
}

// buildContext returns the context of a build node: a string, or a map's context
// key (default "."). has is false when there is no build node.
func buildContext(n yaml.Node) (string, bool) {
	switch n.Kind {
	case yaml.ScalarNode:
		return n.Value, true
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			if n.Content[i].Value == "context" {
				return n.Content[i+1].Value, true
			}
		}
		return ".", true
	}
	return "", false
}

// stagedEnv reads KEY=VALUE pairs from the staged env files, later files winning.
func stagedEnv(set ArtifactSet) map[string]string {
	env := map[string]string{}
	for _, rel := range set.EnvFiles {
		data, err := os.ReadFile(filepath.Join(set.StageDir, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok && !strings.HasPrefix(k, "#") {
				env[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
			}
		}
	}
	return env
}

// expandVars replaces ${VAR} and $VAR from env; an unknown variable stays, so the
// caller sees the leftover $.
func expandVars(s string, env map[string]string) string {
	return os.Expand(s, func(k string) string {
		if v, ok := env[k]; ok {
			return v
		}
		return "$" + "{" + k + "}"
	})
}
