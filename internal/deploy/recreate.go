package deploy

// Purpose: read the resolved compose model on the host and preview what a
//          deploy would recreate (D24 steps 6-7): `docker compose ... config
//          --format json` for the services, their health/restart facts, their
//          mounts and image; `up -d --dry-run` for the services docker says it
//          will Recreate or Create; the stateful subset (a named volume, or a
//          bind under the project data dir); the confirm rule for prod-class
//          targets.
// Inputs:  a Remote, the remote project path and the shipped manifests.
// Outputs: ComposeModel, RecreatePreview, an E403 error when a stateful
//          recreate is not confirmed on a prod-class target.
// Constraints: verified against compose 5.6 output: dry-run prints
//              `Container <name> Recreate|Creating` lines (stdout and stderr
//              mixed, so the command merges them). Service order, the
//              healthcheck and restart facts come from the same JSON document
//              the local rolling restart reads (docker.ServiceSpec), parsed
//              here only because internal/docker exports no parser for it.

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/errs"
)

// DataDirs are the project dirs whose bind mounts hold service data
// (internal/setup writes .volumes/ to .gitignore for the same reason).
var DataDirs = []string{".volumes"}

// Mount is one service mount from the resolved model.
type Mount struct {
	Type     string `json:"type"`
	Source   string `json:"source"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"read_only"`
}

// ModelService is one service of the resolved model.
type ModelService struct {
	Spec          docker.ServiceSpec
	Image         string
	ContainerName string
	Mounts        []Mount
}

// ComposeModel is the resolved model of the shipped manifests.
type ComposeModel struct {
	Project  string
	Services map[string]ModelService
	Order    []string // `config --services` order
}

// Specs returns the docker.ServiceSpec of a service, ok false if unknown.
func (m ComposeModel) Specs(name string) (docker.ServiceSpec, bool) {
	s, ok := m.Services[name]
	return s.Spec, ok
}

// ReadComposeModel resolves the shipped manifests on the host.
func ReadComposeModel(ctx context.Context, r Remote, remotePath string, files, envFiles []string) (ComposeModel, error) {
	base := fmt.Sprintf("cd %s && %s", q(remotePath), composeBase(files, envFiles))
	raw, err := r.Run(ctx, shScript(base+" config --format json"))
	if err != nil {
		return ComposeModel{}, fmt.Errorf("resolving the compose model on %s: %w", r.Label(), err)
	}
	m, err := parseComposeModel(raw)
	if err != nil {
		return m, err
	}
	names, err := r.Run(ctx, shScript(base+" config --services"))
	if err != nil {
		return m, fmt.Errorf("listing services on %s: %w", r.Label(), err)
	}
	m.Order = strings.Fields(string(names))
	for _, n := range m.Order {
		if _, ok := m.Services[n]; !ok {
			return m, fmt.Errorf("service %s is listed by compose but missing from its resolved config on %s", n, r.Label())
		}
	}
	return m, nil
}

func parseComposeModel(raw []byte) (ComposeModel, error) {
	var doc struct {
		Name     string `json:"name"`
		Services map[string]struct {
			Image         string `json:"image"`
			ContainerName string `json:"container_name"`
			Restart       string `json:"restart"`
			Healthcheck   *struct {
				Test    json.RawMessage `json:"test"`
				Disable bool            `json:"disable"`
			} `json:"healthcheck"`
			Volumes []Mount `json:"volumes"`
		} `json:"services"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return ComposeModel{}, fmt.Errorf("parsing docker compose config JSON: %w", err)
	}
	m := ComposeModel{Project: doc.Name, Services: map[string]ModelService{}}
	for name, s := range doc.Services {
		has := false
		if hc := s.Healthcheck; hc != nil && !hc.Disable {
			var t []string
			has = json.Unmarshal(hc.Test, &t) != nil || len(t) == 0 || t[0] != "NONE"
		}
		m.Services[name] = ModelService{
			Spec:  docker.ServiceSpec{Name: name, HasHealthcheck: has, Restart: s.Restart},
			Image: s.Image, ContainerName: s.ContainerName, Mounts: s.Volumes,
		}
	}
	return m, nil
}

// Stateful reports whether a service keeps data on the host: a volume mount
// (named or anonymous), or a bind mount under <remotePath>/<data dir>.
func (m ComposeModel) Stateful(name, remotePath string) bool {
	for _, mt := range m.Services[name].Mounts {
		switch mt.Type {
		case "volume":
			return true
		case "bind":
			for _, d := range DataDirs {
				if p := strings.TrimSuffix(remotePath, "/") + "/" + d; mt.Source == p || strings.HasPrefix(mt.Source, p+"/") {
					return true
				}
			}
		}
	}
	return false
}

// RecreatePreview is what `up -d` would do.
type RecreatePreview struct {
	Recreate []string // services that exist and would be recreated
	Create   []string // services that would be created new
	Stateful []string // the Recreate subset that keeps data
}

var dryRunLine = regexp.MustCompile(`^\s*Container\s+(\S+)\s+(Recreate|Creating)\s*$`)

// PreviewRecreate runs `up -d --dry-run` over the shipped manifests.
func PreviewRecreate(ctx context.Context, r Remote, remotePath string, files, envFiles []string, model ComposeModel) (RecreatePreview, error) {
	cmd := fmt.Sprintf("cd %s && %s up -d --dry-run 2>&1", q(remotePath), composeBase(files, envFiles))
	out, err := r.Run(ctx, shScript(cmd))
	if err != nil {
		return RecreatePreview{}, fmt.Errorf("previewing the deploy on %s: %w", r.Label(), err)
	}
	return parseDryRun(string(out), model, remotePath), nil
}

func parseDryRun(out string, model ComposeModel, remotePath string) RecreatePreview {
	var p RecreatePreview
	seen := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		mm := dryRunLine.FindStringSubmatch(line)
		if mm == nil {
			continue
		}
		svc := serviceOfContainer(mm[1], model)
		if svc == "" || seen[svc+mm[2]] {
			continue
		}
		seen[svc+mm[2]] = true
		if mm[2] == "Recreate" {
			p.Recreate = append(p.Recreate, svc)
			if model.Stateful(svc, remotePath) {
				p.Stateful = append(p.Stateful, svc)
			}
		} else {
			p.Create = append(p.Create, svc)
		}
	}
	sort.Strings(p.Recreate)
	sort.Strings(p.Create)
	sort.Strings(p.Stateful)
	return p
}

// serviceOfContainer maps a container name (<project>-<service>-<n>, the
// older <project>_<service>_<n>, or a container_name) to its service; the
// longest matching service name wins.
func serviceOfContainer(container string, model ComposeModel) string {
	best := ""
	for name, s := range model.Services {
		hit := s.ContainerName != "" && s.ContainerName == container
		for _, sep := range []string{"-", "_"} {
			if i := strings.LastIndex(container, sep+name+sep); i >= 0 && isDigits(container[i+len(name)+2:]) {
				hit = true
			}
		}
		if hit && len(name) > len(best) {
			best = name
		}
	}
	return best
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// ConfirmStateful applies the D10/D24 rule: on a prod-class target a stateful
// service in the recreate set needs confirmation (--yes, or a TTY answer from
// confirm). Anything else returns nil. The refusal is E403 naming the services;
// nothing has been applied when it is returned.
func ConfirmStateful(p RecreatePreview, prodClass, yes bool, confirm func(msg string) bool) error {
	if !prodClass || len(p.Stateful) == 0 || yes {
		return nil
	}
	msg := fmt.Sprintf("this deploy recreates stateful service(s) %s on a production target", strings.Join(p.Stateful, ", "))
	if confirm != nil && confirm(msg) {
		return nil
	}
	return errs.New("E403", msg+" and was not confirmed; nothing was applied").
		WithFix("review the preview, then re-run with --yes")
}
